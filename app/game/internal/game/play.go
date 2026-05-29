package game

import (
	"database/sql"
	"log"

	"go-zero-ddz/app/game/internal/room"
	"go-zero-ddz/app/game/internal/websocket"
	"go-zero-ddz/pkg/cardutil"
)

// GameLogic 游戏逻辑管理器
type GameLogic struct {
	hub      *websocket.Hub
	roomMgr  *room.Manager
	db       *sql.DB
	msgTypes *MessageTypes
}

// MessageTypes 消息类型定义
type MessageTypes struct {
	MsgPassNotify         uint16
	MsgPlayCardsNotify    uint16
	MsgTimerNotify        uint16
	MsgCallLandlordNotify uint16
	MsgDealCardsNotify    uint16
	MsgGameEndNotify      uint16
}

// NewGameLogic 创建游戏逻辑管理器
func NewGameLogic(hub *websocket.Hub, roomMgr *room.Manager, db *sql.DB, msgTypes *MessageTypes) *GameLogic {
	return &GameLogic{
		hub:      hub,
		roomMgr:  roomMgr,
		db:       db,
		msgTypes: msgTypes,
	}
}

// HandlePlayCards 处理出牌请求
func (gl *GameLogic) HandlePlayCards(client *websocket.Client, cards []cardutil.Card, msgID uint16, sendError func(*websocket.Client, uint16, int, string), broadcastMsg func(string, uint16, interface{})) {
	log.Printf("=== GameLogic.HandlePlayCards START ===")
	log.Printf("client=%s, UID=%s, RoomID=%s, cards=%v", client.ID, client.UID, client.RoomID, cards)

	log.Printf("Looking up room: %s", client.RoomID)
	r, exists := gl.roomMgr.GetRoom(client.RoomID)
	if !exists {
		log.Printf("ERROR: Room not found: %s", client.RoomID)
		sendError(client, msgID, 404, "room not found")
		return
	}
	log.Printf("Room found: %s, player count: %d", r.ID, len(r.Players))

	log.Printf("Getting game state for room: %s", r.ID)
	gsm := r.GetGameState()
	if gsm == nil {
		log.Printf("ERROR: Game state is nil for room: %s", r.ID)
		sendError(client, msgID, 500, "game not started")
		return
	}
	log.Printf("Game state found")

	log.Printf("HandlePlayCards: client=%s, UID=%s, room=%s, cards=%v, currentTurn=%s",
		client.ID, client.UID, client.RoomID, cards, r.CurrentTurnUID)

	// 用户手动出牌，取消AI托管
	if player, exists := r.GetPlayer(client.UID); exists && !player.IsBot {
		player.IsAIControlled = false
		log.Printf("HandlePlayCards: player %s manual play, AI control disabled", client.UID)
	}

	result, gameEnded, err := gsm.PlayCards(client.UID, cards)
	if err != nil {
		log.Printf("HandlePlayCards: PlayCards failed for %s: %v", client.UID, err)
		sendError(client, msgID, 500, err.Error())
		return
	}

	log.Printf("HandlePlayCards: PlayCards success for %s: gameEnded=%v", client.UID, gameEnded)

	if result == nil {
		log.Printf("HandlePlayCards: player %s passed", client.UID)
		broadcastMsg(client.RoomID, gl.msgTypes.MsgPassNotify, map[string]interface{}{
			"uid": client.UID,
		})
	} else {
		player, _ := r.GetPlayer(client.UID)
		broadcastMsg(client.RoomID, gl.msgTypes.MsgPlayCardsNotify, map[string]interface{}{
			"uid":        client.UID,
			"cards":      cards,
			"pattern":    result.Pattern.String(),
			"card_count": len(player.Cards),
			"is_last":    len(player.Cards) == 0,
		})
	}

	if gameEnded {
		log.Printf("HandlePlayCards: Game ended after player %s played last card", client.UID)
		gl.HandleGameEnd(r, gsm, broadcastMsg)
		return
	}

	log.Printf("HandlePlayCards: Game continues, currentTurn=%s", r.CurrentTurnUID)

	nextUID := gsm.NextTurnAfterPlay()
	log.Printf("HandlePlayCards: next player is %s, starting timer", nextUID)
	if nextUID != "" {
		// 清除 IsLastRound 标记，因为有玩家出了牌，不是连续 PASS 的情况
		r.IsLastRound = false

		// 在启动定时器前，确保非机器人玩家的 IsAIControlled 被正确重置
		if player, exists := r.GetPlayer(nextUID); exists && !player.IsBot && player.IsAIControlled {
			log.Printf("HandlePlayCards: resetting IsAIControlled to false for player %s before starting timer", nextUID)
			player.IsAIControlled = false
		}

		r.StartTimer(15, nextUID)
		broadcastMsg(client.RoomID, gl.msgTypes.MsgTimerNotify, map[string]interface{}{
			"remaining_seconds": 15,
			"current_turn_uid":  nextUID,
		})
	}
}
