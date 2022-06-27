package modules

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Logiase/MiraiGo-Template/bot"
	"github.com/Logiase/MiraiGo-Template/config"
	"github.com/Mrs4s/MiraiGo/client"
	"github.com/Mrs4s/MiraiGo/message"
	"github.com/go-co-op/gocron"
	log "github.com/sirupsen/logrus"
	"github.com/yangrq1018/botqq/utils"
	"github.com/yudeguang/ratelimit"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type userMessageCount struct {
	Uin       int64  `bson:"uin"`
	GroupCode int64  `bson:"group_code"`
	UserName  string `bson:"user_name"`
	Count     int64  `bson:"count"`
}

type manage struct {
	base
	database *mongo.Database
	ctx      context.Context
	rules    *ratelimit.Rule

	sendTime        string
	clearTime       string
	embyURL         string
	embyToken       string
	spamThreshold   float64
	muteDuration    time.Duration
	notifyGroups    []int
	spamMsgInterval int
	fileDict        map[string]string
}

var instanceManage *manage

func (s *manage) MiraiGoModule() bot.ModuleInfo {
	return bot.ModuleInfo{
		ID:       "manage",
		Instance: instanceManage,
	}
}

func (s *manage) Init() {
	s.base.Init()

	s.ctx = context.Background()
	s.database = mongoClient.Database("qq")
	s.rules = ratelimit.NewRule()
	s.fileDict = make(map[string]string)

	moduleConfig := config.GlobalConfig.Sub("modules." + s.MiraiGoModule().ID.Name())
	if moduleConfig != nil {
		s.spamMsgInterval = moduleConfig.GetInt("spam_msgs")
		s.rules.AddRule(moduleConfig.GetDuration("spam_duration"), s.spamMsgInterval)
		s.spamThreshold = moduleConfig.GetFloat64("spam_threshold")
		s.muteDuration = moduleConfig.GetDuration("mute_duration")
		s.sendTime = moduleConfig.GetString("send")
		s.clearTime = moduleConfig.GetString("clear")
		s.embyURL = moduleConfig.GetString("emby")
		s.embyToken = moduleConfig.GetString("emby_token")
		s.notifyGroups = moduleConfig.GetIntSlice("notify_groups")
		for k, v := range moduleConfig.GetStringMapString("files") {
			s.fileDict[k] = v
		}
	} else {
		logger.Warnf("module %s config not found", s.MiraiGoModule().ID.Name())
	}
}

func (s *manage) PostInit() {}

func (s *manage) Serve(bot *bot.Bot) {
	s.monitorGroups.Each(func(code int64) {
		registerMessageListener(code, s.handleCommand, &bot.GroupMessageEvent, &bot.SelfGroupMessageEvent)
		registerGroupMemberJoinListener(code, handleNewMemberJoin, &bot.GroupMemberJoinEvent)
		registerMessageListener(code, s.antiSpam, &bot.GroupMessageEvent, &bot.SelfGroupMessageEvent)
	})
}

func (s *manage) Start(bot *bot.Bot) {
	sch := gocron.NewScheduler(time.Local)
	_, err := sch.Cron(s.clearTime).Do(func() {
		logger.Info("clear stat")
		s.clearCounter(bot.QQClient)
	})
	if err != nil {
		logger.Error(err)
		return
	}
	_, err = sch.Cron(s.sendTime).Do(func() {
		for _, code := range s.notifyGroups {
			s.sendStat(bot.QQClient, int64(code), 3)
		}
	})
	if err != nil {
		logger.Error(err)
		return
	}
	sch.StartAsync()
}

func (s *manage) Stop(_ *bot.Bot, wg *sync.WaitGroup) {
	defer wg.Done()
	_ = s.database.Client().Disconnect(s.ctx)
}

func (s *manage) handleCommand(client *client.QQClient, msg *message.GroupMessage) {
	// 记录msg的发送者
	s.addCounter(msg.Sender, msg.GroupCode, 1)
	if s.isBotCommand(msg) {
		if text := textMessage(msg); text != nil {
			cmd, args := command(text)
			switch cmd {
			case "/clear":
			case "/emby":
				s.creatEmbyUser(client, msg)
			case "/top":
				s.sendStat(client, msg.GroupCode, 3)
			case "/file":
				if len(args) > 0 {
					err := s.uploadFileToGroup(client, msg.GroupCode, args[0])
					if err != nil {
						logger.Error(err)
					}
				}
			}
		}
	}
}

func (s *manage) makeStatMessage(group *client.GroupInfo, senders []userMessageCount) *message.SendingMessage {
	msg := message.NewSendingMessage()
	msg.Append(message.NewText(fmt.Sprintf("%q最活跃的前%d个成员\n",
		group.Name,
		len(senders))),
	)
	for i := range senders {
		msg.Append(
			message.NewText(fmt.Sprintf("%q水群%d次\n", senders[i].UserName, senders[i].Count)),
		)
	}
	return msg.Append(message.NewText("再接再厉!"))
}

func (s *manage) sendStat(c *client.QQClient, groupCode int64, n int64) {
	senders, err := s.top(groupCode, n)
	if err != nil {
		logger.Error(err)
		c.SendGroupMessage(groupCode, utils.TextMessage(err.Error()))
		return
	}
	if len(senders) == 0 {
		return
	}
	groupInfo := new(client.GroupInfo)
	for i := range c.GroupList {
		if c.GroupList[i].Code == groupCode {
			groupInfo = c.GroupList[i]
		}
	}
	reply := s.makeStatMessage(groupInfo, senders)
	c.SendGroupMessage(groupCode, reply)
}

func (s *manage) addCounter(sender *message.Sender, groupCode, i int64) {
	coll := s.database.Collection("stat")
	_, err := coll.UpdateOne(
		s.ctx,
		bson.D{
			{"uin", sender.Uin},
			{"group_code", groupCode},
		},
		bson.D{
			{
				"$inc", bson.D{{"count", i}},
			}, {
				"$set", bson.D{{"user_name", sender.DisplayName()}},
			},
		},
		options.Update().SetUpsert(true),
	)
	if err != nil {
		logger.Error(err)
	}
}

func (s *manage) top(groupCode, n int64) ([]userMessageCount, error) {
	// have to parse string as int
	cur, err := s.database.Collection("stat").Find(
		s.ctx,
		bson.D{
			{"group_code", groupCode},
		},
		options.Find().SetSort(bson.D{{"count", -1}}),
	)
	if err != nil {
		return nil, err
	}
	var results []userMessageCount
	if err = cur.All(s.ctx, &results); err != nil {
		return nil, err
	}
	if len(results) < int(n) {
		return results, nil
	}
	return results[:n], nil
}

func (s *manage) clearCounter(client *client.QQClient) {
	// client as parameter to keep client.GroupList updated
	for _, group := range client.GroupList {
		_, err := s.database.Collection("stat").DeleteMany(
			s.ctx,
			bson.D{
				{"group_code", group.Code},
			},
		)
		if err != nil {
			logger.Error(err)
		}
	}
}

func (s *manage) authReq(req *http.Request) {
	q := req.URL.Query()
	q.Set("api_key", s.embyToken)
	req.URL.RawQuery = q.Encode()
	req.Header.Set("Content-Type", "application/json")
}

func (s *manage) endpoint(ep endpoint) string {
	switch ep {
	case createUser:
		return s.embyURL + "/emby" + "/Users/New"
	default:
		return ""
	}
}

func (s *manage) creatEmbyUser(client *client.QQClient, msg *message.GroupMessage) {
	body := bytes.NewBuffer(nil)
	_ = json.NewEncoder(body).Encode(&struct {
		Name string `json:"Name"`
	}{strconv.FormatInt(msg.Sender.Uin, 10)})

	req, _ := http.NewRequest(http.MethodPost, s.endpoint(createUser), body)
	s.authReq(req)
	res, err := proxiedClient.Do(req)
	defer func() {
		_ = res.Body.Close()
	}()
	if err != nil {
		logger.Error(err)
		return
	}
	if res.StatusCode != http.StatusOK {
		var textError []byte
		textError, _ = ioutil.ReadAll(res.Body)
		replyToGroupMessage(client, msg, fmt.Sprintf("server response: %d %s", res.StatusCode, textError))
		return
	}
	var user UserDto
	_ = json.NewDecoder(res.Body).Decode(&user)
	replyToGroupMessage(client, msg, fmt.Sprintf("EMBY：成功创建用户，用户名为QQ号码，默认密码为空，请登录%s修改密码和观影", s.embyURL))
}

// 判断刷屏逻辑
func (s *manage) isSpam(client *client.QQClient, m *message.GroupMessage) bool {
	g, err := client.GetGroupInfo(m.GroupCode)
	if err != nil {
		return false
	}
	// parameter here
	history, err := client.GetGroupMessages(m.GroupCode, g.LastMsgSeq-int64(s.spamMsgInterval), g.LastMsgSeq)
	if err != nil {
		return false
	}
	var from int
	for _, msg := range history {
		if msg.Sender.Uin == m.Sender.Uin {
			from++
		}
	}
	// 如果超过阈值百分比的消息来自一个人，认为刷屏
	return float64(from)/float64(len(history)) > s.spamThreshold
}

func (s *manage) antiSpam(client *client.QQClient, m *message.GroupMessage) {
	if s.rules.AllowVisit(m.Sender.Uin) {
		return
	}

	// check if this user is spamming the group
	if s.isSpam(client, m) {
		logger.Infof("mute member %s: spam message %q", m.Sender.Nickname, m.ToString())
		if err := muteGroupMember(client, m, s.muteDuration); err != nil {
			logger.Error(err)
			return
		}
		replyToGroupMessage(client, m, fmt.Sprintf("您发送消息太过频繁，已被禁言%d分钟", int(s.muteDuration.Minutes())))
	}
}

func (s *manage) lookUpFileURL(keyword string) string {
	return s.fileDict[keyword]
}

// TODO: this is subject to pan.qq.come change
const remoteFolder = "/3f5cbf44-8f5c-4d2f-b559-21a100e471d5"

func (s *manage) uploadFileToGroup(c *client.QQClient, groupCode int64, keyword string) error {
	url := s.lookUpFileURL(keyword)
	if url == "" {
		logger.Infof("keyword %s does not have a URL associated", keyword)
		return nil
	}
	source := message.Source{
		SourceType: message.SourceGroup,
		PrimaryID:  groupCode,
	}
	res, err := http.Get(url)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	data := bytes.NewBuffer(nil)
	_, err = io.Copy(data, res.Body)
	if err != nil {
		return err
	}

	tokens := strings.Split(url, "/")
	file := &client.LocalFile{
		FileName:     tokens[len(tokens)-1],
		Body:         bytes.NewReader(data.Bytes()), // maybe the best way to use res.Body as io.ReadSeeker
		RemoteFolder: remoteFolder,
	}
	return c.UploadFile(source, file)
}

// 禁言群组中的该条消息发言成员
func muteGroupMember(client *client.QQClient, m *message.GroupMessage, d time.Duration) error {
	g, err := client.GetGroupInfo(m.GroupCode)
	if err != nil {
		return err
	}
	g.Members, _ = client.GetGroupMembers(g)
	member := g.FindMember(m.Sender.Uin)
	if member == nil {
		return nil
	}
	// in seconds, if less than 60, 1 minute is used
	return member.Mute(uint32(d.Seconds()))
}

func handleNewMemberJoin(client *client.QQClient, event *client.MemberJoinGroupEvent) {
	log.Infof("a new member joined group %d", event.Member.Uin)
	welcomeImage, err := readImageURI(os.Getenv("QQ_GROUP_WELCOME_URI"))
	if err != nil {
		log.Errorf("cannot welcome new user, fetch image: %v", err)
		return
	}

	msg := pictureMessage(client, event.Group.Code, welcomeImage)

	// break lines into multiple elements to avoid URL getting chunked
	msg.Append(message.NewText("\n欢迎新人" + event.Member.DisplayName())).
		Append(message.NewText("\n看置顶公告，加Steam组和完美公会：CN摆烂大队")).
		Append(message.NewText("\n下载Teamspeak(TS)，参考https://yangruoqi.site/teamspeak")).
		Append(message.NewText("\n公会跑图社区服，参考https://yangruoqi.site/csgo-server"))
	client.SendGroupMessage(event.Group.Code, msg)
}
