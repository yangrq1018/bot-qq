package modules

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Logiase/MiraiGo-Template/bot"
	"github.com/Logiase/MiraiGo-Template/config"
	"github.com/Mrs4s/MiraiGo/client"
	"github.com/Mrs4s/MiraiGo/message"
	"github.com/julienschmidt/httprouter"
	"github.com/yangrq1018/botqq/model"
	"github.com/yangrq1018/botqq/utils"
	"github.com/yudeguang/ratelimit"
	"github.com/zyedidia/generic"
	"github.com/zyedidia/generic/hashset"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var (
	instanceRoll *roll
)

type roll struct {
	base

	groupNotice       bool
	rule              *ratelimit.Rule
	database          *mongo.Database
	ctx               context.Context
	atAll             bool
	backendServerAddr string
	ctxMgr            map[string]context.CancelFunc // all operation on ctxMgr should hold _mu
	_mu               sync.Mutex
}

func (r *roll) MiraiGoModule() bot.ModuleInfo {
	return bot.ModuleInfo{
		ID:       "roll",
		Instance: instanceRoll,
	}
}

func (r *roll) Init() {
	r.base.Init()
	r.rule = ratelimit.NewRule()
	r.ctx = context.Background()
	r.ctxMgr = make(map[string]context.CancelFunc)
	r.database = mongoClient.Database("qq")

	moduleConfig := config.GlobalConfig.Sub("modules.roll")
	r.groupNotice = moduleConfig.GetBool("group_notice")
	r.atAll = moduleConfig.GetBool("at_all")
	r.backendServerAddr = moduleConfig.GetString("addr")

	// 限制新建抽奖的频率最高为每分钟三次
	if moduleConfig.IsSet("rate") {
		duration, times := moduleConfig.GetDuration("rate.duration"), moduleConfig.GetInt("rate.times")
		logger.Infof("application (per user) rate limit set to %d per %s", times, duration)
		r.rule.AddRule(duration, times)
	}
}

func (r *roll) PostInit() {}

func (r *roll) Serve(bot *bot.Bot) {
	r.monitorGroups.Each(func(code int64) {
		registerMessageListener(code, r.dispatch, &bot.GroupMessageEvent, &bot.SelfGroupMessageEvent)
	})
	go r.startServer(bot.QQClient, r.backendServerAddr)
}

func (r *roll) Start(bot *bot.Bot) {
	// start unfinished rolls
	collection := r.collection()
	r.monitorGroups.Each(func(groupCode int64) {
		cursor, err := collection.Find(r.ctx, bson.M{
			"group_code": groupCode,
			"draw_time": bson.M{
				"$gt": time.Now(),
			},
		})
		if err != nil {
			logger.Fatal(err)
		}
		logger.Info("checking unfinished rolls...")
		for cursor.Next(r.ctx) {
			var e rollEvent
			err = cursor.Decode(&e)
			if err != nil {
				logger.Error(err)
				continue
			}
			e.DrawTime = e.DrawTime.In(time.Local)
			go r.drawLater(bot.QQClient, groupCode, &e)
		}
	})

	go r.webSourceInsert(bot)
}

func (r *roll) startServer(c *client.QQClient, addr string) {
	router := httprouter.New()
	router.GET("/members/:group", func(writer http.ResponseWriter, _ *http.Request, params httprouter.Params) {
		groupCode, _ := strconv.Atoi(params.ByName("group"))
		g, err := c.GetGroupInfo(int64(groupCode))
		if err != nil {
			logger.Error(err)
			writer.WriteHeader(http.StatusBadRequest)
			return
		}
		members, err := c.GetGroupMembers(g)
		if err != nil {
			logger.Error(err)
			writer.WriteHeader(http.StatusBadRequest)
			return
		}
		_ = json.NewEncoder(writer).Encode(&struct {
			Members []*client.GroupMemberInfo `json:"members"`
		}{
			members,
		})
	})
	router.GET("/groups", func(writer http.ResponseWriter, _ *http.Request, _ httprouter.Params) {
		err := c.ReloadGroupList()
		if err == nil {
			groups := make([]struct {
				Uin             int64
				Code            int64
				Name            string
				OwnerUin        int64
				GroupCreateTime uint32
				GroupLevel      uint32
				MemberCount     uint16
				MaxMemberCount  uint16
			}, len(c.GroupList))
			for i := range c.GroupList {
				groups[i].Uin = c.GroupList[i].Uin
				groups[i].Code = c.GroupList[i].Code
				groups[i].Name = c.GroupList[i].Name
				groups[i].OwnerUin = c.GroupList[i].OwnerUin
				groups[i].GroupCreateTime = c.GroupList[i].GroupLevel
				groups[i].GroupLevel = c.GroupList[i].GroupLevel
				groups[i].MemberCount = c.GroupList[i].MemberCount
				groups[i].MaxMemberCount = c.GroupList[i].MaxMemberCount
			}
			_ = json.NewEncoder(writer).Encode(&groups)
			return
		} else {
			logger.Error(err)
			writer.WriteHeader(http.StatusInternalServerError)
		}
	})
	go http.ListenAndServe(addr, router)
}

func (r *roll) Stop(_ *bot.Bot, wg *sync.WaitGroup) {
	defer wg.Done()
	_ = r.database.Client().Disconnect(r.ctx)
}

// 选出第一个回复元素, nil if none
func replyMessage(msg *message.GroupMessage) *message.ReplyElement {
	for _, elem := range msg.Elements {
		switch e := elem.(type) {
		case *message.ReplyElement:
			return e
		}
	}
	return nil
}

func (r *roll) persistModel(event *rollEvent) {
	model := event.Model()
	model.ObjectID.ObjectID = primitive.NewObjectID() // get a new object ID here
	model.Insert(r.ctx, r.collection())
	event.ObjectID = model.ObjectID
}

func (r *roll) getRoll(groupCode int64, msgID int32) (*rollEvent, bool) {
	var data model.MongoEvent
	err, ok := data.Find(r.ctx, r.collection(), groupCode, msgID)
	if !ok || err != nil {
		return nil, false
	}
	return newRollEventFromModel(&data), true
}

func (r *roll) collection() *mongo.Collection {
	return r.database.Collection("csgo")
}

func replyToGroupMessage(client *client.QQClient, msg *message.GroupMessage, text string) {
	client.SendGroupMessage(msg.GroupCode, utils.NewTextMessage(text))
}

func (r *roll) dispatch(client *client.QQClient, msg *message.GroupMessage) {
	if reply := replyMessage(msg); reply != nil {
		// 确认回复对象是发起roll的消息
		if re, ok := r.getRoll(msg.GroupCode, reply.ReplySeq); ok {
			if !re.participants.Has(*msg.Sender) {
				re.Model().AddParticipant(r.ctx, r.collection(), *msg.Sender)
				logger.Infof("add a participant %s, current # of participants %d", msg.Sender.DisplayName(), re.participants.Size()+1)
				replyToGroupMessage(client, msg, msg.Sender.DisplayName()+"已加入抽奖")
			} else {
				logger.Infof("%s already in roll %d", msg.Sender.DisplayName(), re.identity())
			}
		}
	}
	if r.isToBot(msg) {
		if text := textOfGroupMessage(msg); text != nil {
			cmd, args := command(text)
			switch cmd {
			case "/cancel":
				if len(args) < 1 {
					return
				}
				objectID := strings.TrimPrefix(args[0], "#")
				r._mu.Lock()
				if cancel, ok := r.ctxMgr[objectID]; ok {
					logger.Infof("called cancel func of %s", objectID)
					cancel()
					delete(r.ctxMgr, objectID)
					// don't delete object in database, for now
				}
				r._mu.Unlock()
			case "/roll":
				go func() {
					if !r.rule.AllowVisit(msg.Sender.Uin) {
						replyToGroupMessage(client, msg, "您的抽奖操作过于频繁，请稍后再试")
					} else if !isAdmin(client, msg.GroupCode, msg.Sender.Uin) {
						replyToGroupMessage(client, msg, "您不是管理员，没有抽奖权限")
					} else {
						err := r.rollCSGOSkin(client, msg)
						if err != nil {
							logger.Errorf("failed to roll: %v", err)
						}
					}
				}()
			}
		}
	}
}

// 返回该qq号是否是一个群的管理员
func isAdmin(qqc *client.QQClient, groupCode, uin int64) bool {
	admins := hashset.New(0, generic.Equals[int64], generic.HashInt64)
	for _, g := range qqc.GroupList {
		if g.Code == groupCode {
			for _, member := range g.Members {
				if member.Permission == client.Administrator || member.Permission == client.Owner {
					admins.Put(member.Uin)
				}
			}
		}
	}
	return admins.Has(uin)
}

func (r *roll) drawLater(client *client.QQClient, groupCode int64, event *rollEvent) {
	// wait until draw time
	ctx, cancel := context.WithCancel(r.ctx)
	defer cancel()
	r._mu.Lock()
	r.ctxMgr[event.ShortHexID()] = cancel
	r._mu.Unlock()

	after := time.Until(event.DrawTime)
	en := logger.
		WithField("identity", event.identity()).
		WithField("object_id", event.ShortHexID()).
		WithField("after", after)
	en.Infof("draw %q", event.SkinName)
	select {
	case <-time.After(after):
	case <-ctx.Done():
		en.Infof("cancelled")
		client.SendGroupMessage(groupCode, utils.NewTextMessage("取消#"+event.ShortHexID()))
		return
	}

	// refresh participants from database
	e, ok := r.getRoll(event.GroupCode, event.MsgId)
	if !ok {
		return
	} else {
		event.participants = e.participants
	}

	if len(event.Participants()) == 0 {
		logger.Infof("no participants in roll")
		return
	}
	winners := event.Draw()
	for i, w := range winners {
		en.Infof("draw the [%d]-th winner: %d(%s)", i, w.Uin, w.DisplayName())
		e.Model().AddWinner(ctx, r.collection(), w)
		client.SendGroupMessage(groupCode, event.noticeRollWinnerMessage(&w))
	}
}

// 启动一个抽奖事件
func (r *roll) rollCSGOSkin(client *client.QQClient, msg *message.GroupMessage) error {
	event := newRollEventFromMessage(msg)
	r.persistModel(event)
	r.notice(client, event, msg)
	// 创建群公告
	if r.groupNotice {
		err := client.AddGroupNoticeSimple(msg.GroupCode, event.GroupNotice())
		if err != nil {
			logger.Errorf("failed to add group notice: %v", err)
		}
	}
	logger.WithField("event", event).Infof("roll event created")
	r.drawLater(client, msg.GroupCode, event)
	return nil
}

// This is an example change event struct for inserts.
// It does not include all possible change event fields.
// You should consult the change event documentation for more info:
// https://docs.mongodb.com/manual/reference/change-events/
type changeEvent struct {
	OperationType string      `bson:"operationType"`
	DocumentKey   documentKey `bson:"documentKey"`
}

type documentKey struct {
	ID primitive.ObjectID `bson:"_id"`
}

// this is not a reliable method
func (r *roll) webSourceInsert(bot *bot.Bot) {
reconnect:
	collection := r.collection()
	// specify a pipeline that will only match "insert" events
	// specify the MaxAwaitTimeOption to have each attempt wait two seconds for new documents
	matchStage := bson.D{{"$match", bson.D{{"operationType", "insert"}}}}
	opts := options.ChangeStream().SetMaxAwaitTime(2 * time.Second)
	cs, err := collection.Watch(context.TODO(), mongo.Pipeline{matchStage}, opts)
	if err != nil {
		logger.Error(err)
		return
	}

	cleanup := func() {
		_ = cs.Close(r.ctx)
	}
	defer cleanup()
	logger.Infof("start listen to web source insert")
	for cs.Next(r.ctx) {
		var ce changeEvent
		var re model.MongoEvent
		_ = cs.Decode(&ce)
		err = collection.FindOne(r.ctx, ce.DocumentKey).Decode(&re)
		if err == nil {
			logger.Infof("a new document insert: %v", ce.DocumentKey.ID)
			if re.Source == "web" {
				logger.Infof("a new web source mongo db document insert: %v", ce.DocumentKey.ID)
				e := newRollEventFromModel(&re)
				go func() {
					msg2 := r.notice(bot.QQClient, e, nil)
					_, err = collection.UpdateOne(r.ctx, ce.DocumentKey, bson.M{"$set": bson.M{"msg_id": msg2.Id}})
					if err != nil {
						logger.Error(err)
						return
					}
					r.drawLater(bot.QQClient, e.GroupCode, e)
				}()
			}
		} else {
			break
		}
	}
	logger.Warn("the web source insert listen go routine stopped, err = %v", cs.Err())
	cleanup()
	logger.Info("reconnecting...")
	goto reconnect
}

func (r *roll) notice(client *client.QQClient, event *rollEvent, msg *message.GroupMessage) *message.GroupMessage {
	if msg == nil {
		text2 := fmt.Sprintf(
			`#%s
确认创建抽奖(来源Web)，回复本条任意内容以参加!
即将抽取奖品:%q 
开奖时间:%s
发起人:%s
奖品数量:%d
`, event.ObjectID.ShortHexID(), event.SkinName, event.DrawTime.In(time.Local).Format("01月02日 15:04"), event.SenderNickname, event.WinnerCount)
		msg2 := message.NewSendingMessage()
		if r.atAll {
			msg2.Append(message.NewAt(0, ""))
		}
		msg2.Append(message.NewText(text2))
		msg2Res := client.SendGroupMessage(event.GroupCode, msg2)

		// set essential
		_ = client.SetEssenceMessage(event.GroupCode, msg2Res.Id, msg2Res.InternalId)
		event.MsgId = msg2Res.Id
		return msg2Res
	} else {
		text2 := fmt.Sprintf(
			`#%s
确认创建抽奖，回复上条消息（精华消息）任意内容以参加！
即将抽取奖品:%q 
开奖时间:%s
发起人:%s
奖品数量:%d
`, event.ShortHexID(), event.SkinName, event.DrawTime.Format("01月02日 15:04"), event.SenderNickname, event.WinnerCount)
		msg2 := message.NewSendingMessage()
		if r.atAll {
			msg2.Append(message.NewAt(0, ""))
		}
		msg2.Append(message.NewText(text2))
		client.SendGroupMessage(msg.GroupCode, msg2)
		_ = client.SetEssenceMessage(msg.GroupCode, msg.Id, msg.InternalId)
		return msg
	}
}
