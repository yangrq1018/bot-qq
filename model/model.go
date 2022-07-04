package model

import (
	"time"

	"github.com/Mrs4s/MiraiGo/message"
)

// mongodb digest-able type
type RollEventMongo struct {
	SenderID       int64            `bson:"sender_id" json:"qqUIN"`
	SenderNickname string           `bson:"sender_nickname"`
	SkinName       string           `bson:"skin_name" json:"skinName"`
	DrawTime       time.Time        `bson:"draw_time" json:"drawDate"`
	Source         string           `bson:"source"`
	MsgID          int32            `bson:"msg_id"`
	GroupCode      int64            `bson:"group_code"`
	GroupName      string           `bson:"group_name"`
	Participants   []message.Sender `bson:"participants"`
}
