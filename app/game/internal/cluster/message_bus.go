package cluster

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/zeromicro/go-zero/core/logx"
)

type MessageType string

const (
	MsgTypePlay      MessageType = "play"
	MsgTypeControl   MessageType = "control"
	MsgTypeBroadcast MessageType = "broadcast"
	MsgTypeReconnect MessageType = "reconnect"
)

type ClusterMessage struct {
	Type      MessageType     `json:"type"`
	RoomID    string          `json:"room_id"`
	SenderUID string          `json:"sender_uid,omitempty"`
	Payload   json.RawMessage `json:"payload"`
	Timestamp int64           `json:"timestamp"`
}

type MessageHandler func(msg *ClusterMessage)

type MessageBus struct {
	rdb        redis.UniversalClient
	instanceID string
	handlers   map[string]MessageHandler
	pubsub     *redis.PubSub
	ctx        context.Context
}

func NewMessageBus(rdb redis.UniversalClient, instanceID string) *MessageBus {
	return &MessageBus{
		rdb:        rdb,
		instanceID: instanceID,
		handlers:   make(map[string]MessageHandler),
	}
}

func (mb *MessageBus) Start(ctx context.Context) {
	mb.ctx = ctx
	mb.pubsub = mb.rdb.Subscribe(ctx)
	go mb.listenLoop(ctx)
}

func (mb *MessageBus) Subscribe(ctx context.Context, channel string, handler MessageHandler) error {
	mb.handlers[channel] = handler
	return mb.pubsub.Subscribe(ctx, channel)
}

func (mb *MessageBus) SubscribeRoom(ctx context.Context, roomID string) error {
	channels := []string{
		fmt.Sprintf("room:%s:broadcast", roomID),
		fmt.Sprintf("room:%s:control", roomID),
		fmt.Sprintf("room:%s:play", roomID),
	}
	for _, ch := range channels {
		if err := mb.Subscribe(ctx, ch, mb.getRoomHandler(roomID, ch)); err != nil {
			return err
		}
	}
	return nil
}

func (mb *MessageBus) Publish(ctx context.Context, channel string, msg *ClusterMessage) error {
	msg.Timestamp = time.Now().UnixMilli()
	payload, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	return mb.rdb.Publish(ctx, channel, string(payload)).Err()
}

func (mb *MessageBus) PublishToRoom(ctx context.Context, roomID string, msgType MessageType, payload interface{}) error {
	channel := getChannelForType(roomID, msgType)
	msg := &ClusterMessage{
		Type:    msgType,
		RoomID:  roomID,
		Payload: mustMarshal(payload),
	}
	return mb.Publish(ctx, channel, msg)
}

func (mb *MessageBus) listenLoop(ctx context.Context) {
	ch := mb.pubsub.Channel()
	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-ch:
			if !ok {
				return
			}
			handler, exists := mb.handlers[msg.Channel]
			if !exists {
				continue
			}
			var clusterMsg ClusterMessage
			if err := json.Unmarshal([]byte(msg.Payload), &clusterMsg); err != nil {
				logx.Errorf("failed to unmarshal cluster message: %v", err)
				continue
			}
			handler(&clusterMsg)
		}
	}
}

func (mb *MessageBus) Close() error {
	if mb.pubsub != nil {
		return mb.pubsub.Close()
	}
	return nil
}

func getChannelForType(roomID string, msgType MessageType) string {
	switch msgType {
	case MsgTypePlay:
		return fmt.Sprintf("room:%s:play", roomID)
	case MsgTypeControl:
		return fmt.Sprintf("room:%s:control", roomID)
	default:
		return fmt.Sprintf("room:%s:broadcast", roomID)
	}
}

func mustMarshal(v interface{}) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}

func (mb *MessageBus) getRoomHandler(roomID, channel string) MessageHandler {
	return func(msg *ClusterMessage) {
		switch {
		case isBroadcastChannel(channel):
			mb.handleBroadcast(roomID, msg)
		case isControlChannel(channel):
			if mb.isRoomOwner(roomID) {
				mb.handleControl(roomID, msg)
			}
		case isPlayChannel(channel):
			if mb.isRoomOwner(roomID) {
				mb.handlePlay(roomID, msg)
			}
		}
	}
}

func isBroadcastChannel(ch string) bool { return len(ch) > 9 && ch[len(ch)-9:] == "broadcast" }
func isControlChannel(ch string) bool   { return len(ch) > 7 && ch[len(ch)-7:] == "control" }
func isPlayChannel(ch string) bool      { return len(ch) > 4 && ch[len(ch)-4:] == "play" }

func (mb *MessageBus) isRoomOwner(roomID string) bool                     { return true }
func (mb *MessageBus) handleBroadcast(roomID string, msg *ClusterMessage) {}
func (mb *MessageBus) handleControl(roomID string, msg *ClusterMessage)   {}
func (mb *MessageBus) handlePlay(roomID string, msg *ClusterMessage)      {}
