package modules

import (
	"github.com/Logiase/MiraiGo-Template/bot"
	"github.com/Logiase/MiraiGo-Template/config"
	"github.com/Mrs4s/MiraiGo/client"
	"github.com/Mrs4s/MiraiGo/message"
	"github.com/stretchr/testify/assert"
	"os"
	"testing"
	"time"
)

func TestMain(t *testing.M) {
	config.Init()
	os.Exit(t.Run())
}

func TestNewJoin(t *testing.T) {
	bot.Init()
	bot.StartService()
	bot.UseProtocol(bot.AndroidPhone)
	err := bot.Login()
	assert.NoError(t, err)
	bot.RefreshList()
	_ = os.Setenv("QQ_GROUP_WELCOME_URI", "https://s1.ax1x.com/2022/05/17/O4zLAU.png")

	group := &client.GroupInfo{
		Code: 852485822,
	}
	handleNewMemberJoin(bot.Instance.QQClient, &client.MemberJoinGroupEvent{
		Group: group,
		Member: &client.GroupMemberInfo{
			Group:    group,
			Uin:      1284700603,
			Nickname: "小牛冲冲",
			CardName: "小牛冲冲",
		},
	})
}

func TestMuteMember(t *testing.T) {
	bot.Init()
	bot.StartService()
	bot.UseProtocol(bot.AndroidPhone)
	err := bot.Login()
	assert.NoError(t, err)
	bot.SaveToken()
	bot.RefreshList()

	assert.NoError(t, muteGroupMember(bot.Instance.QQClient, &message.GroupMessage{
		GroupCode: 852485822,
		Sender: &message.Sender{
			Uin: 2411690005,
		},
	}, 1*time.Minute))
}
