// Package modules is the bot qq command modules definition
package modules

import (
	"bytes"
	"fmt"
	"io"
	"regexp"
	"strings"
	"sync"

	"github.com/Logiase/MiraiGo-Template/bot"
	"github.com/Logiase/MiraiGo-Template/config"
	"github.com/Mrs4s/MiraiGo/client"
	"github.com/Mrs4s/MiraiGo/message"
	log "github.com/sirupsen/logrus"
	"github.com/yangrq1018/botqq/utils"
	"github.com/zyedidia/generic"
	"github.com/zyedidia/generic/hashset"
)

// base implements bot.Module barely
type base struct {
	monitorGroups *hashset.Set[int64] // 监听群组，在Serve前初始化，目前支持从config.GlobalConfig读取
	botUin        int64
	admin         *hashset.Set[int64]
}

func (*base) MiraiGoModule() bot.ModuleInfo {
	return bot.ModuleInfo{
		ID: "implement me",
	}
}

func (b *base) Init() {
	b.monitorGroups = hashset.New(0, generic.Equals[int64], generic.HashInt64)
	for _, code := range config.GlobalConfig.GetIntSlice("group_codes") {
		b.monitorGroups.Put(int64(code))
	}
	b.botUin = config.GlobalConfig.GetInt64("bot.account")
	if b.botUin == 0 {
		log.Fatal("must specify bot qq account")
	}
	b.admin = utils.Int64Set(config.GlobalConfig.GetIntSlice("admin"))
}

func (*base) PostInit() {}

func (*base) Serve(bot *bot.Bot) {}

func (*base) Start(bot *bot.Bot) {}

func (*base) Stop(bot *bot.Bot, wg *sync.WaitGroup) {}

// 只有@机器人的消息才会认为是发给bot的命令
// 注意复制消息不会复制底层的AtElement，需要手动输入
func (b *base) isToBot(msg *message.GroupMessage) bool {
	for _, elem := range msg.Elements {
		switch e := elem.(type) {
		case *message.AtElement:
			// at对象
			if e.Target == b.botUin {
				return true
			}
		}
	}
	return false
}

func searchForTextElement(elements []message.IMessageElement) *message.TextElement {
	te := new(message.TextElement)
	var found bool
	for _, elem := range elements {
		switch e := elem.(type) {
		case *message.TextElement:
			found = true
			te.Content += e.Content
		}
	}
	if found {
		return te
	}
	return nil
}

// 选出一个文字消息元素, nil if none
// 对于有unicode表情元素的，消息会被切割成多个*message.TextElement，将其合并成一个
func textOfGroupMessage(msg *message.GroupMessage) *message.TextElement {
	return searchForTextElement(msg.Elements)
}

func textOfPrivateMessage(msg *message.PrivateMessage) *message.TextElement {
	return searchForTextElement(msg.Elements)
}

func textOfTempMessage(msg *client.TempMessageEvent) *message.TextElement {
	return searchForTextElement(msg.Message.Elements)
}

var commandRegexp = regexp.MustCompile(`/\w+`)

func command(element *message.TextElement) (string, []string) {
	var args []string
	content := strings.TrimSpace(element.Content)
	if strings.HasPrefix(content, "/") {
		m := commandRegexp.FindString(content)
		if m != "" {
			args = strings.Fields(content)
			return m, args[1:]
		}
	}
	return "", nil
}

func readImageURI(uri string) (io.ReadSeeker, error) {
	res, err := proxiedClient.Get(uri)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch image: %v", err)
	}

	imageBytes, err := io.ReadAll(res.Body)
	_ = res.Body.Close()
	if err != nil {
		return nil, fmt.Errorf("failed to read image url: %v", err)
	}
	return bytes.NewReader(imageBytes), nil
}

func pictureMessage(client *client.QQClient, groupCode int64, data io.ReadSeeker) *message.SendingMessage {
	source := message.Source{
		SourceType: message.SourceGroup,
		PrimaryID:  groupCode,
	}
	image, err := client.UploadImage(source, data)
	if err != nil {
		return utils.NewTextMessage(err.Error())
	}
	return message.NewSendingMessage().Append(image)
}

type groupMessageHandleFunc func(client *client.QQClient, e *message.GroupMessage)
type groupMemberJoinHandleFunc func(client *client.QQClient, e *client.MemberJoinGroupEvent)
type groupMemberLeaveHandleFunc func(client *client.QQClient, e *client.MemberLeaveGroupEvent)
type privateMessageHandleFunc func(client *client.QQClient, e *message.PrivateMessage)
type tempMessageHandleFunc func(client *client.QQClient, e *client.TempMessageEvent)
type groupMessageRecallHandleFunc func(client *client.QQClient, e *client.GroupMessageRecalledEvent)

func registerMessageListener(groupCode int64, callback groupMessageHandleFunc, events ...*client.EventHandle[*message.GroupMessage]) {
	for _, event := range events {
		event.Subscribe(func(client *client.QQClient, msg *message.GroupMessage) {
			if msg.GroupCode == groupCode {
				callback(client, msg)
			}
		})
	}
}

func registerGroupMemberJoinListener(groupCode int64, callback groupMemberJoinHandleFunc, events ...*client.EventHandle[*client.MemberJoinGroupEvent]) {
	for _, event := range events {
		event.Subscribe(func(client *client.QQClient, e *client.MemberJoinGroupEvent) {
			if e.Group.Code == groupCode {
				callback(client, e)
			}
		})
	}
}

func registerGroupMemberLeaveListener(groupCode int64, callback groupMemberLeaveHandleFunc, events ...*client.EventHandle[*client.MemberLeaveGroupEvent]) {
	for _, event := range events {
		event.Subscribe(func(client *client.QQClient, e *client.MemberLeaveGroupEvent) {
			if e.Group.Code == groupCode {
				callback(client, e)
			}
		})
	}
}

func registerPrivateMessageListener(callback privateMessageHandleFunc, events ...*client.EventHandle[*message.PrivateMessage]) {
	for _, event := range events {
		event.Subscribe(func(client *client.QQClient, msg *message.PrivateMessage) {
			callback(client, msg)
		})
	}
}

func registerTempMessageListener(callback tempMessageHandleFunc, events ...*client.EventHandle[*client.TempMessageEvent]) {
	for _, event := range events {
		event.Subscribe(func(client *client.QQClient, msg *client.TempMessageEvent) {
			callback(client, msg)
		})
	}
}

func registerGroupMessageRecallListener(groupCode int64, callback groupMessageRecallHandleFunc, event *client.EventHandle[*client.GroupMessageRecalledEvent]) {
	event.Subscribe(func(client *client.QQClient, e *client.GroupMessageRecalledEvent) {
		if e.GroupCode == groupCode {
			callback(client, e)
		}
	})
}
