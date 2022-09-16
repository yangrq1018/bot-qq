package model

import (
	"context"
	"encoding/hex"
	"time"

	"github.com/Logiase/MiraiGo-Template/utils"
	"github.com/Mrs4s/MiraiGo/message"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

var logger = utils.GetModuleLogger("mongo.model")

type ObjectID struct {
	ObjectID primitive.ObjectID `bson:"_id"`
}

func (o ObjectID) HexID() string {
	return hex.EncodeToString(o.ObjectID[:])
}

func (o ObjectID) ShortHexID() string {
	// 3 bytes, 6-char string
	return hex.EncodeToString(o.ObjectID[len(o.ObjectID)-3 : len(o.ObjectID)])
}

// The Mongo DB mirror of event
// the object ID is a 12 byte array, that is 24 hex digits
// the `bson:",inline"` is for letting ObjectID's fields get unmarshaled
type MongoEvent struct {
	ObjectID       `bson:",inline"`
	SenderID       int64            `bson:"sender_id" json:"qqUIN"`
	SenderNickname string           `bson:"sender_nickname"`
	SkinName       string           `bson:"skin_name" json:"skinName"`
	DrawTime       time.Time        `bson:"draw_time" json:"drawDate"`
	Source         string           `bson:"source"`
	MsgID          int32            `bson:"msg_id"`
	GroupCode      int64            `bson:"group_code"`
	GroupName      string           `bson:"group_name"`
	WinnerCount    int              `bson:"winner_count"`
	Participants   []message.Sender `bson:"participants"`
}

func (r *MongoEvent) identity() bson.M {
	return bson.M{"group_code": r.GroupCode, "msg_id": r.MsgID}
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

func (r *MongoEvent) AddWinner(ctx context.Context, c *mongo.Collection, winner message.Sender) {
	_, err := c.UpdateOne(ctx,
		r.identity(),
		bson.M{"$push": bson.D{{"winner", winner}}})
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
