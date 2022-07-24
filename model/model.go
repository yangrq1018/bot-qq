package model

import (
	"context"
	"time"

	"github.com/Logiase/MiraiGo-Template/utils"
	"github.com/Mrs4s/MiraiGo/message"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

var logger = utils.GetModuleLogger("mongo.model")

// The Mongo DB mirror of event
type MongoEvent struct {
	SenderID       int64            `bson:"sender_id" json:"qqUIN"`
	SenderNickname string           `bson:"sender_nickname"`
	SkinName       string           `bson:"skin_name" json:"skinName"`
	DrawTime       time.Time        `bson:"draw_time" json:"drawDate"`
	Source         string           `bson:"source"`
	MsgId          int32            `bson:"msg_id"`
	GroupCode      int64            `bson:"group_code"`
	GroupName      string           `bson:"group_name"`
	Participants   []message.Sender `bson:"participants"`
}

func (r *MongoEvent) identity() bson.M {
	return bson.M{"group_code": r.GroupCode, "msg_id": r.MsgId}
}

func (r *MongoEvent) Insert(ctx context.Context, c *mongo.Collection) {
	_, err := c.InsertOne(ctx, r)
	if err != nil {
		logger.Errorf("failed to persist roll event: %v", err)
	}
}

func (r *MongoEvent) Find(ctx context.Context, c *mongo.Collection, groupCode int64, msgID int32) (error, bool) {
	result := c.FindOne(ctx, bson.M{
		"group_code": groupCode,
		"msg_id":     msgID,
	})
	switch result.Err() {
	case nil:
	case mongo.ErrNoDocuments:
		return nil, false
	default:
		logger.Errorf("failed to get roll event: %v", result.Err())
		return nil, false
	}
	err := result.Decode(r)
	if err != nil {
		logger.Errorf("failed to decode roll event: %v", err)
		return err, false
	}
	return nil, true
}

func (r *MongoEvent) UpdateWinner(ctx context.Context, c *mongo.Collection, winner *message.Sender) {
	_, err := c.UpdateOne(ctx, r.identity(), bson.M{"$set": bson.D{{"winner", winner}}})
	if err != nil {
		logger.Error(err)
	}
}

func (r *MongoEvent) AddParticipant(ctx context.Context, c *mongo.Collection, p message.Sender) {
	_, err := c.UpdateOne(ctx,
		r.identity(),
		bson.M{"$push": bson.M{"participants": p}},
	)
	if err != nil {
		logger.Errorf("failed to append participants: %v", err)
	}
}
