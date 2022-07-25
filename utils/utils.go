package utils

import (
	"github.com/Mrs4s/MiraiGo/client"
	"github.com/Mrs4s/MiraiGo/message"
	"github.com/zyedidia/generic"
	"github.com/zyedidia/generic/hashset"
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

func Int64Set[T int64 | int](xs []T) *hashset.Set[int64] {
	s := hashset.New(100, generic.Equals[int64], generic.HashInt64)
	for _, x := range xs {
		s.Put(int64(x))
	}
	return s
}
