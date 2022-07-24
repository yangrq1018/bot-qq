package utils

import (
	"github.com/Mrs4s/MiraiGo/client"
	"github.com/Mrs4s/MiraiGo/message"
)

func TextMessage(text string) *message.SendingMessage {
	msg := message.NewSendingMessage()
	return msg.Append(message.NewText(text))
}

func PermissionString(p client.MemberPermission) string {
	switch p {
	case client.Owner:
		return "群主"
	case client.Administrator:
		return "管理员"
	default:
		return "普通成员"
	}
}