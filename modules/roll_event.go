package modules

import (
	"bufio"
	"fmt"
	"math/rand"
	"strings"
	"sync"
	"time"

	"github.com/Mrs4s/MiraiGo/message"
	"github.com/yangrq1018/botqq/model"
	"github.com/zyedidia/generic/hashset"
	"go.mongodb.org/mongo-driver/bson"
)

type rollEvent struct {
	model.ObjectID `bson:",inline"`

	SenderID       int64     `bson:"sender_id"`
	SenderNickname string    `bson:"sender_nickname"`
	SkinName       string    `bson:"skin_name"`
	DrawTime       time.Time `bson:"draw_time"`
	MsgID          int32     `bson:"msg_id"`
	GroupCode      int64     `bson:"group_code"`
	GroupName      string    `bson:"group_name"`
	WinnerCount    int       `bson:"winner_count"`

	participants *hashset.Set[message.Sender] `bson:"-"`
	_mu          sync.Mutex                   `bson:"-"`
}

// default CTOR
func newRollEvent() *rollEvent {
	e := new(rollEvent)
	e.participants = hashset.New(0, func(a, b message.Sender) bool {
		return a.Uin == b.Uin
	}, func(t message.Sender) uint64 {
		return uint64(t.Uin)
	})
	return e
}

func newRollEventFromModel(m *model.MongoEvent) *rollEvent {
	r := newRollEvent()
	r.ObjectID = m.ObjectID
	r.SenderID = m.SenderID
	r.SenderNickname = m.SenderNickname
	r.SkinName = m.SkinName
	r.DrawTime = m.DrawTime
	r.MsgID = m.MsgID
	r.GroupCode = m.GroupCode
	r.GroupName = m.GroupName
	r.WinnerCount = m.WinnerCount
	for _, p := range m.Participants {
		r.participants.Put(p)
	}
	return r
}

// msg is assumed to contain text element(s)
func newRollEventFromMessage(msg *message.GroupMessage) *rollEvent {
	event := newRollEvent()
	event.MsgID = msg.Id
	event.SenderID = msg.Sender.Uin
	event.SenderNickname = msg.Sender.DisplayName()
	event.GroupCode = msg.GroupCode
	event.GroupName = msg.GroupName

	content := textOfGroupMessage(msg).Content
	// use scanner for consistency across platforms
	scanner := bufio.NewScanner(strings.NewReader(content))
	var i int // line number
	for scanner.Scan() {
		line := scanner.Text()
		switch i {
		case 1:
			// 物品名称
			event.SkinName = strings.TrimSpace(line)
		case 2:
			// 开奖时间
			line = strings.TrimSpace(line)
			switch line {
			case "now":
				event.DrawTime = time.Now()
			default:
				if drawTime, err := time.ParseInLocation("2006-01-02 15:04", line, time.Local); err != nil {
					logger.Errorf("failed to parse draw time: %v", line)
				} else {
					event.DrawTime = drawTime
				}
			}
		case 0:
			// ignore
		default:
			// provide an optional list of initial participants
			event.participants.Put(message.Sender{
				Uin:      -int64(i), // fake at
				Nickname: strings.TrimSpace(line),
			})
			logger.WithField("identity", event.identity()).Infof("add participant (by nickname) %q", strings.TrimSpace(line))
		}
		i++
	}
	event.WinnerCount = 1
	return event
}

func (e *rollEvent) AddParticipant(sender *message.Sender) {
	e._mu.Lock()
	e.participants.Put(*sender)
	e._mu.Unlock()
}

func (e *rollEvent) GroupNotice() string {
	return fmt.Sprintf(`老板%s即将roll一个 %q
开奖时间%s
回复上条消息（任意内容）以参加抽奖`,
		e.SenderNickname, e.SkinName, e.DrawTime.Format("2006-01-02 15:04 -0700 MST"))
}

func (e *rollEvent) Participants() []message.Sender {
	return e.participants.Values()
}

// 开奖
// 如果设置的开奖人数少于或等于当前人数，则返回全部参与者
// 否则一直抽取胜者只到达到开奖人数
// hold the lock
func (e *rollEvent) Draw() []message.Sender {
	e._mu.Lock()
	defer e._mu.Unlock()
	nParticipants := e.participants.Size()
	if nParticipants == 0 {
		return nil
	}

	nWinner := e.WinnerCount
	if nWinner == 0 {
		nWinner = 1
	}

	if nWinner >= nParticipants {
		return e.Participants()
	}

	pool := e.participants.Copy()
	var winners []message.Sender
	for len(winners) < nWinner {
		winner := pool.Values()[rand.Intn(pool.Size())]
		winners = append(winners, winner)
		pool.Remove(winner)
	}
	return winners
}

func (e *rollEvent) noticeRollWinnerMessage(winner *message.Sender) *message.SendingMessage {
	text := fmt.Sprintf(`恭喜用户%q(qq号码%d)抽中了奖品%q!`, winner.DisplayName(), winner.Uin, e.SkinName)
	msg := message.NewSendingMessage()
	// At元素必须在第一个
	if winner.Uin > 0 {
		msg.Append(message.NewAt(winner.Uin))
	}
	return msg.Append(message.NewText(text))
}

func (e *rollEvent) identity() bson.M {
	return bson.M{"group_code": e.GroupCode, "msg_id": e.MsgID}
}

func (e *rollEvent) Model() *model.MongoEvent {
	e2 := &model.MongoEvent{
		ObjectID:       e.ObjectID,
		SenderID:       e.SenderID,
		SenderNickname: e.SenderNickname,
		SkinName:       e.SkinName,
		DrawTime:       e.DrawTime,
		MsgID:          e.MsgID,
		GroupCode:      e.GroupCode,
		GroupName:      e.GroupName,
		WinnerCount:    e.WinnerCount,
		Participants:   []message.Sender{},
	}
	e.participants.Each(func(sender message.Sender) {
		e2.Participants = append(e2.Participants, sender)
	})
	return e2
}
