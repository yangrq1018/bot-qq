package modules

import (
	"github.com/Logiase/MiraiGo-Template/bot"
	"github.com/Mrs4s/MiraiGo/client"
	"github.com/Mrs4s/MiraiGo/message"
)

type super struct {
	base
}

var instanceSuper *super

func (s *super) MiraiGoModule() bot.ModuleInfo {
	return bot.ModuleInfo{
		ID:       "super",
		Instance: instanceSuper,
	}
}

func (s *super) Serve(bot *bot.Bot) {
	s.monitorGroups.Each(func(code int64) {
		registerMessageListener(code, s.handle, &bot.GroupMessageEvent, &bot.SelfGroupMessageEvent)
	})
}

func (s *super) handle(client *client.QQClient, e *message.GroupMessage) {
	if s.admin.Has(e.Sender.Uin) {
		// enable super mode, send log messages to chat
		logger.Infof("Enter super admin mode, instructed by %s", e.Sender.DisplayName())
	}
}
