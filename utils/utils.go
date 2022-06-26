package utils

import (
	"github.com/Mrs4s/MiraiGo/message"
)

func TextMessage(text string) *message.SendingMessage {
	msg := message.NewSendingMessage()
	return msg.Append(message.NewText(text))
}
