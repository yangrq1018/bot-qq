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
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	cache "github.com/Code-Hex/go-generics-cache"
	"github.com/Logiase/MiraiGo-Template/bot"
	"github.com/Logiase/MiraiGo-Template/config"
	"github.com/Mrs4s/MiraiGo/client"
	"github.com/Mrs4s/MiraiGo/message"
	"github.com/fsnotify/fsnotify"
	"github.com/go-co-op/gocron"
	"github.com/yangrq1018/botqq/utils"
	"github.com/yudeguang/ratelimit"
	"github.com/zyedidia/generic/hashset"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// TODO: this is subject to pan.qq.come change
const remoteFolder = "/3f5cbf44-8f5c-4d2f-b559-21a100e471d5"

var instanceManage *manage
var pfRegex = regexp.MustCompile(`完美(账号)?(\d+)?$`)

type userMessageCount struct {
	Uin       int64  `bson:"uin"`
	GroupCode int64  `bson:"group_code"`
	UserName  string `bson:"user_name"`
	Count     int64  `bson:"count"`
}

type perfectWorldAccount struct {
	Account       string `bson:"account"`
	Password      string `bson:"password"`
	Email         string `bson:"email"`
	EmailPassword string `bson:"emailPassword"`
	EmailSite     string `bson:"emailSite"`
	Mobile        string `bson:"mobile"`
}

type manage struct {
	base
	database *mongo.Database
	ctx      context.Context
	rules    *ratelimit.Rule

	sendTime             string
	clearTime            string
	embyURL              string
	embyToken            string
	spamThreshold        float64
	muteDuration         time.Duration // TODO dynamic mute duration
	messageCacheTime     time.Duration
	notifyGroups         []int
	spamMsgInterval      int
	approveFriendRequest bool
	fileDict             map[string]fileSearch
	keywordReplyDict     map[string]string
	privateChatList      *hashset.Set[int64] // write once, no lock protected
	configLock           sync.Mutex
	messageCache         *cache.Cache[int32, *message.GroupMessage]
	lastRecallMessage    *message.GroupMessage
	_lastRecallMessageMu sync.Mutex
}

type fileSearch struct {
	URL string
	Msg string
}

// public methods

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
	s.fileDict = make(map[string]fileSearch)
	moduleName := s.MiraiGoModule().ID.Name()
	moduleConfig := config.GlobalConfig.Sub("modules." + moduleName)
	s.privateChatList = utils.Int64Set(moduleConfig.GetIntSlice("private_chat_list"))
	s.messageCache = cache.New[int32, *message.GroupMessage]()

	// the call must be before WatchConfig()
	config.GlobalConfig.OnConfigChange(func(in fsnotify.Event) {
		logger.Infof("the config file has changed, op=%s, name=%s", in.Op.String(), in.Name)
		s.configLock.Lock()
		s.privateChatList = utils.Int64Set(config.GlobalConfig.GetIntSlice("modules." + moduleName + ".private_chat_list"))
		s.configLock.Unlock()
		logger.Infof("# of member in private chat list: %d", s.privateChatList.Size())
	})

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
		s.messageCacheTime = moduleConfig.GetDuration("message_cache_time")
		s.keywordReplyDict = moduleConfig.GetStringMapString("keyword_reply")
		s.approveFriendRequest = moduleConfig.GetBool("approve_friend_request")
		for k, v := range moduleConfig.GetStringMap("files") {
			file := fileSearch{}
			switch x := v.(type) {
			case string:
				file.URL = x
			case map[string]interface{}:
				file.URL = x["url"].(string)
				file.Msg = x["msg"].(string)
			default:
			}
			s.fileDict[k] = file
		}
	} else {
		logger.Fatal("module %s config not loaded", s.MiraiGoModule().ID.Name())
	}
}

func (s *manage) PostInit() {}

func (s *manage) Serve(bot *bot.Bot) {
	s.monitorGroups.Each(func(code int64) {
		registerMessageListener(code, s.handleCommand, &bot.GroupMessageEvent, &bot.SelfGroupMessageEvent)
		registerMessageListener(code, s.antiSpam, &bot.GroupMessageEvent, &bot.SelfGroupMessageEvent)
		registerGroupMessageRecallListener(code, s.listenRecall, &bot.GroupMessageRecalledEvent)
		registerGroupMemberJoinListener(code, handleNewMemberJoin, &bot.GroupMemberJoinEvent)
		registerGroupMemberLeaveListener(code, handleMemberLeave, &bot.GroupMemberLeaveEvent)
	})

	registerPrivateMessageListener(s.handlePrivate, &bot.PrivateMessageEvent, &bot.SelfPrivateMessageEvent)
	// TODO: in-group non-friend chat message won't work
	registerTempMessageListener(s.handleTemp, &bot.TempMessageEvent)

	// 自动通过好友申请
	if s.approveFriendRequest {
		logger.Info("好友申请自动通过：启动")
		bot.NewFriendRequestEvent.Subscribe(func(client *client.QQClient, req *client.NewFriendRequest) {
			logger.Infof("approve friend request from %s", req.RequesterNick)
			req.Accept()
		})
	}

	bot.GroupMemberPermissionChangedEvent.Subscribe(func(client *client.QQClient, event *client.MemberPermissionChangedEvent) {
		old, new := utils.PermissionString(event.OldPermission), utils.PermissionString(event.NewPermission)
		client.SendGroupMessage(event.Group.Code, utils.NewTextMessage(fmt.Sprintf(`【%s】的权限从%s被修改为%s`,
			event.Member.DisplayName(),
			old,
			new,
		)))
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

// private methods start here

func (s *manage) handleCommand(client *client.QQClient, msg *message.GroupMessage) {
	// 记录msg的发送者
	s.addCounter(msg.Sender, msg.GroupCode, 1)
	text := textOfGroupMessage(msg)
	if text == nil {
		return
	}

	s.messageCache.Set(msg.Id, msg, cache.WithExpiration(s.messageCacheTime))

	if s.isToBot(msg) {
		if k, ok := s.containKeyWord(text); ok {
			replyToGroupMessage(client, msg, s.keywordReplyDict[k])
			return
		}
		cmd, args := command(text)
		switch cmd {
		case "/ping":
			client.SendGroupMessage(msg.GroupCode, utils.NewTextMessage("pong"))
		case "/emby":
			s.creatEmbyUser(client, msg)
		case "/top", "/活跃成员":
			s.sendStat(client, msg.GroupCode, 3)
		case "/file", "/文件":
			if len(args) > 0 {
				err := s.uploadFileToGroup(client, msg.GroupCode, args[0])
				if err != nil {
					logger.Error(err)
				}
			}
		case "/recall", "/防撤回":
			if !s.admin.Has(msg.Sender.Uin) {
				client.SendGroupMessage(msg.GroupCode, utils.NewTextMessage("你没有admin权限"))
				return
			}
			s._lastRecallMessageMu.Lock()
			if s.lastRecallMessage != nil {
				m := s.lastRecallMessage
				mTime := time.Unix(int64(m.Time), 0)
				client.SendGroupMessage(m.GroupCode,
					utils.NewTextMessage(
						fmt.Sprintf("%s前，%s撤回了消息: %q",
							time.Since(mTime),
							m.Sender.DisplayName(),
							textOfGroupMessage(m).Content)))
			} else {
				client.SendGroupMessage(msg.GroupCode, utils.NewTextMessage("没有最近记录的撤回消息"))
			}
			s._lastRecallMessageMu.Unlock()
		}
	}
}

func (s *manage) handlePrivateOrTemp(client *client.QQClient, sender *message.Sender, txt *message.TextElement) {
	if s.canPrivateChat(sender) {
		tokens := pfRegex.FindStringSubmatch(txt.Content)
		if tokens == nil {
			return
		}
		match, seq := tokens[0], tokens[2]
		accounts, err := s.getPerfectWorldAccounts()
		if err != nil {
			logger.Error(err)
			return
		}
		if match == "完美账号" {
			msg := utils.NewTextMessage("从下列账号中选择一个，发送“完美” + 【序号】，如“完美1”\n")
			for i := range accounts {
				msg.Append(message.NewText(
					fmt.Sprintf("[%d]%s\n", i+1, accounts[i].Mobile),
				))
			}
			client.SendPrivateMessage(sender.Uin, msg)
		} else if seq != "" {
			seqNum, err := strconv.Atoi(seq)
			if err != nil {
				return
			}
			seqNum--
			if seqNum >= len(accounts) {
				return
			}
			a := accounts[seqNum]
			msg := utils.NewTextMessage(fmt.Sprintf(
				`账号:%s
密码:%s
邮箱:%s
邮箱密码:%s
邮箱网址:%s
手机号:%s`,
				a.Account, a.Password, a.Email, a.EmailPassword, a.EmailSite, a.Mobile))
			client.SendPrivateMessage(sender.Uin, msg)
		}
	}
}

func (s *manage) handlePrivate(client *client.QQClient, e *message.PrivateMessage) {
	txt := textOfPrivateMessage(e)
	if txt == nil {
		return
	}
	s.handlePrivateOrTemp(client, e.Sender, txt)
}

func (s *manage) handleTemp(client *client.QQClient, e *client.TempMessageEvent) {
	txt := textOfTempMessage(e)
	if txt == nil {
		return
	}
	s.handlePrivateOrTemp(client, e.Message.Sender, txt)
}

func (s *manage) canPrivateChat(sender *message.Sender) bool {
	s.configLock.Lock()
	defer s.configLock.Unlock()
	return s.privateChatList.Has(int64(sender.Uin))
}

func (s *manage) containKeyWord(text *message.TextElement) (string, bool) {
	content := strings.ToLower(text.Content)
	for keyword := range s.keywordReplyDict {
		// probably regexp here?
		match := regexp.MustCompile(keyword).FindString(content)
		if match != "" {
			return keyword, true
		}
	}
	return "", false
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
		c.SendGroupMessage(groupCode, utils.NewTextMessage(err.Error()))
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

func (s *manage) getPerfectWorldAccounts() ([]perfectWorldAccount, error) {
	cur, err := s.database.Collection("perfectworld").
		Find(s.ctx, bson.D{}, options.Find().SetSort(bson.M{"mobile": 1}))
	if err != nil {
		return nil, err
	}
	var accounts []perfectWorldAccount
	if err = cur.All(s.ctx, &accounts); err != nil {
		return nil, err
	}
	return accounts, nil
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

func (s *manage) listenRecall(client *client.QQClient, e *client.GroupMessageRecalledEvent) {
	recallMsgId := e.MessageId
	// check in cache
	m, ok := s.messageCache.Get(recallMsgId)
	if ok {
		s._lastRecallMessageMu.Lock()
		logger.Infof("recall message set to msg id=%d", m.Id)
		s.lastRecallMessage = m
		s._lastRecallMessageMu.Unlock()
	}
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

func (s *manage) lookUpFile(keyword string) (fileSearch, bool) {
	f, ok := s.fileDict[keyword]
	return f, ok
}

func (s *manage) uploadFileToGroup(c *client.QQClient, groupCode int64, keyword string) error {
	item, ok := s.lookUpFile(keyword)
	if !ok {
		logger.Infof("keyword %s does not have a URL associated", keyword)
		return nil
	}
	source := message.Source{
		SourceType: message.SourceGroup,
		PrimaryID:  groupCode,
	}
	res, err := http.Get(item.URL)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	data := bytes.NewBuffer(nil)
	_, err = io.Copy(data, res.Body)
	if err != nil {
		return err
	}

	tokens := strings.Split(item.URL, "/")
	file := &client.LocalFile{
		FileName:     tokens[len(tokens)-1],
		Body:         bytes.NewReader(data.Bytes()), // maybe the best way to use res.Body as io.ReadSeeker
		RemoteFolder: remoteFolder,
	}
	err = c.UploadFile(source, file)
	if err != nil {
		return err
	}
	if item.Msg != "" {
		c.SendGroupMessage(groupCode, utils.NewTextMessage(item.Msg))
	}
	return nil
}

// helper functions

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
	logger.WithField("uin", event.Member.Uin).Infof("a new member joined")
	welcomeImage, err := readImageURI(os.Getenv("QQ_GROUP_WELCOME_URI"))
	if err != nil {
		logger.Errorf("cannot welcome new user, fetch image error : %v", err)
		return
	}

	msg := pictureMessage(client, event.Group.Code, welcomeImage)

	// break lines into multiple elements to avoid URL getting chunked
	msg.Append(message.NewText("\n欢迎新人" + event.Member.DisplayName())).
		Append(message.NewText("\n看置顶公告，加Steam组和完美公会：CN摆烂大队")).
		Append(message.NewText("\n下载Teamspeak(TS)，群文件有安装包和中文补丁"))
	client.SendGroupMessage(event.Group.Code, msg)
}

func handleMemberLeave(client *client.QQClient, event *client.MemberLeaveGroupEvent) {
	logger.WithField("uin", event.Member.Uin).Infof("a new member leaved")
	msg := message.NewSendingMessage()
	if event.Operator != nil {
		msg.Append(message.NewText("成员【" + event.Member.DisplayName() + "】被" + event.Operator.DisplayName() + "踢出群聊。"))
	} else {
		msg.Append(message.NewText("成员【" + event.Member.DisplayName() + "】离开了群聊。"))
	}
	client.SendGroupMessage(event.Group.Code, msg)
}
