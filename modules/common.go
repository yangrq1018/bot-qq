package modules

import (
	"bytes"
	"fmt"
	"github.com/Logiase/MiraiGo-Template/config"
	"github.com/Mrs4s/MiraiGo/client"
	"github.com/Mrs4s/MiraiGo/message"
	log "github.com/sirupsen/logrus"
	"github.com/zyedidia/generic"
	"github.com/zyedidia/generic/hashset"
	"io"
	"regexp"
	"strings"
)

type base struct {
	monitorGroups *hashset.Set[int64] // 监听群组，在Serve前初始化，目前支持从config.GlobalConfig读取
	botUin        int64
}

func (b *base) Init() {
	b.monitorGroups = hashset.New[int64](0, generic.Equals[int64], generic.HashInt64)
	for _, code := range config.GlobalConfig.GetIntSlice("group_codes") {
		b.monitorGroups.Put(int64(code))
	}
	b.botUin = config.GlobalConfig.GetInt64("bot.account")
	if b.botUin == 0 {
		log.Fatal("must specify bot qq account")
	}
}

// 只有@机器人的消息才会认为是发给bot的命令
// 注意复制消息不会复制底层的AtElement，需要手动输入
func (b *base) isBotCommand(msg *message.GroupMessage) bool {
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

// 选出一个文字消息元素, nil if none
// 对于有unicode表情元素的，消息会被切割成多个*message.TextElement，将其合并成一个
func textMessage(msg *message.GroupMessage) *message.TextElement {
	te := new(message.TextElement)
	var found bool
	for _, elem := range msg.Elements {
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

var commandRegexp = regexp.MustCompile(`/\w+`)

func command(element *message.TextElement) string {
	contentStripped := strings.TrimSpace(element.Content)
	if strings.HasPrefix(contentStripped, "/") {
		m := commandRegexp.FindString(contentStripped)
		if m != "" {
			return m
		}
	}
	return ""
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
	image, err := client.UploadGroupImage(groupCode, data)
	if err != nil {
		return message.NewSendingMessage().
			Append(message.NewText(err.Error()))
	}
	return message.NewSendingMessage().Append(image)
}

type groupMessageHandleFunc func(client *client.QQClient, msg *message.GroupMessage)
type groupMemberJoinHandleFunc func(client *client.QQClient, e *client.MemberJoinGroupEvent)

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
