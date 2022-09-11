package modules

import (
	"fmt"
	"sync"
	"time"

	cache "github.com/Code-Hex/go-generics-cache"
	"github.com/Logiase/MiraiGo-Template/bot"
	"github.com/Logiase/MiraiGo-Template/config"
	"github.com/Mrs4s/MiraiGo/client"
	"github.com/Mrs4s/MiraiGo/message"
	"github.com/yudeguang/ratelimit"
)

var instanceSpam *antiSpam

type antiSpam struct {
	base
	spamMsgInterval int
	spamThreshold   float64
	rules           *ratelimit.Rule
	muteDuration    time.Duration // TODO dynamic mute duration
	muteMultiplier  int
	mutedCache      *cache.Cache[int64, time.Duration]
}

func (a *antiSpam) MiraiGoModule() bot.ModuleInfo {
	return bot.ModuleInfo{
		ID:       "spam",
		Instance: instanceSpam,
	}
}

func (a *antiSpam) Init() {
	a.base.Init()

	a.rules = ratelimit.NewRule()
	a.mutedCache = cache.New[int64, time.Duration]()

	moduleName := a.MiraiGoModule().ID.Name()
	moduleConfig := config.GlobalConfig.Sub("modules." + moduleName)
	if moduleConfig != nil {
		a.spamMsgInterval = moduleConfig.GetInt("spam_msgs")
		a.rules.AddRule(moduleConfig.GetDuration("spam_duration"), a.spamMsgInterval)
		a.spamThreshold = moduleConfig.GetFloat64("spam_threshold")
		a.muteDuration = moduleConfig.GetDuration("mute_duration")
		a.muteMultiplier = moduleConfig.GetInt("mute_multiplier")
	}
}

func (*antiSpam) PostInit() {}

func (a *antiSpam) Serve(bot *bot.Bot) {
	a.monitorGroups.Each(func(code int64) {
		registerMessageListener(code, a.antiSpam, &bot.GroupMessageEvent, &bot.SelfGroupMessageEvent)
	})
}

func (*antiSpam) Start(bot *bot.Bot) {}

func (*antiSpam) Stop(_ *bot.Bot, wg *sync.WaitGroup) {
	defer wg.Done()
}

func (a *antiSpam) antiSpam(client *client.QQClient, m *message.GroupMessage) {
	if a.rules.AllowVisit(m.Sender.Uin) {
		return
	}

	// check if this user is spamming the group
	if a.isSpam(client, m) {
		logger.Infof("mute member %s: spam message %q", m.Sender.Nickname, m.ToString())
		duration := a.muteDuration
		if d, ok := a.mutedCache.Get(m.Sender.Uin); ok {
			duration = d
			duration *= time.Duration(a.muteMultiplier)
		}
		// repeatedly spam the group, increase that
		if err := muteGroupMember(client, m, duration); err != nil {
			logger.Error(err)
			return
		}
		a.mutedCache.Set(m.Sender.Uin, duration, cache.WithExpiration(24*time.Hour))
		replyToGroupMessage(client, m, fmt.Sprintf("%s发送消息太过频繁，已被禁言%d分钟", m.Sender.DisplayName(), int(duration.Minutes())))
	}
}

// 判断刷屏逻辑
func (a *antiSpam) isSpam(client *client.QQClient, m *message.GroupMessage) bool {
	g, err := client.GetGroupInfo(m.GroupCode)
	if err != nil {
		return false
	}
	// parameter here
	history, err := client.GetGroupMessages(m.GroupCode, g.LastMsgSeq-int64(a.spamMsgInterval), g.LastMsgSeq)
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
	return float64(from)/float64(len(history)) > a.spamThreshold
}
