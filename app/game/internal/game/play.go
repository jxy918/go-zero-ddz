package game

import (
	"database/sql"

	"go-zero-ddz/app/game/internal/room"
	"go-zero-ddz/app/game/internal/websocket"
	"go-zero-ddz/pkg/cardutil"

	"github.com/zeromicro/go-zero/core/logx"
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
	logx.Infof("=== GameLogic.HandlePlayCards START ===")
	logx.Infof("client=%s, UID=%s, RoomID=%s, cards=%v", client.ID, client.UID, client.RoomID, cards)

	logx.Infof("Looking up room: %s", client.RoomID)
	r, exists := gl.roomMgr.GetRoom(client.RoomID)
	if !exists {
		logx.Errorf("ERROR: Room not found: %s", client.RoomID)
		sendError(client, msgID, 404, "room not found")
		return
	}
	logx.Infof("Room found: %s, player count: %d", r.ID, len(r.Players))

	logx.Infof("Getting game state for room: %s", r.ID)
	gsm := r.GetGameState()
	if gsm == nil {
		logx.Errorf("ERROR: Game state is nil for room: %s", r.ID)
		sendError(client, msgID, 500, "game not started")
		return
	}
	logx.Infof("Game state found")

	logx.Infof("HandlePlayCards: client=%s, UID=%s, room=%s, cards=%v, currentTurn=%s",
		client.ID, client.UID, client.RoomID, cards, r.CurrentTurnUID)

	// 用户手动出牌，取消AI托管
	if player, exists := r.GetPlayer(client.UID); exists && !player.IsBot {
		player.IsAIControlled = false
		player.GraceWarningSent = false
		logx.Infof("HandlePlayCards: player %s manual play, AI control disabled", client.UID)
	}

	result, gameEnded, err := gsm.PlayCards(client.UID, cards)
	if err != nil {
		logx.Errorf("HandlePlayCards: PlayCards failed for %s: %v", client.UID, err)
		sendError(client, msgID, 500, err.Error())
		return
	}

	logx.Infof("HandlePlayCards: PlayCards success for %s: gameEnded=%v", client.UID, gameEnded)

	if result == nil {
		logx.Infof("HandlePlayCards: player %s passed", client.UID)
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
		logx.Infof("HandlePlayCards: Game ended after player %s played last card", client.UID)
		gl.HandleGameEnd(r, gsm, broadcastMsg)
		return
	}

	logx.Infof("HandlePlayCards: Game continues, currentTurn=%s", r.CurrentTurnUID)

	// 从 room 直接获取下一个玩家的 UID（PlayCards 已在内部推进回合）
	nextUID := r.CurrentTurnUID
	logx.Infof("HandlePlayCards: next player is %s, starting timer", nextUID)
	if nextUID != "" {
		// 如果是出牌成功，清除 IsLastRound 标记
		if result != nil {
			r.IsLastRound = false
		}

		// 在启动定时器前，确保非机器人玩家的 IsAIControlled 和 GraceWarningSent 被正确重置
		if player, exists := r.GetPlayer(nextUID); exists && !player.IsBot {
			if player.IsAIControlled {
				logx.Infof("HandlePlayCards: resetting IsAIControlled to false for player %s before starting timer", nextUID)
				player.IsAIControlled = false
			}
			if player.GraceWarningSent {
				player.GraceWarningSent = false
			}
		}

		r.StartTimer(15, nextUID)
		broadcastMsg(client.RoomID, gl.msgTypes.MsgTimerNotify, map[string]interface{}{
			"remaining_seconds": 15,
			"current_turn_uid":  nextUID,
		})
	}
}
