package handler

import (
	"database/sql"
	"encoding/json"

	"go-zero-ddz/app/game/internal/game"
	"go-zero-ddz/app/game/internal/match"
	"go-zero-ddz/app/game/internal/room"
	"go-zero-ddz/app/game/internal/websocket"
	"go-zero-ddz/pkg/types"

	"github.com/zeromicro/go-zero/core/logx"
)

// HandlerManager 消息处理器管理器
type HandlerManager struct {
	hub         *websocket.Hub
	roomMgr     *room.Manager
	coordinator *match.Coordinator
	db          *sql.DB
	gameLogic   *game.GameLogic
}

// NewHandlerManager 创建处理器管理器
func NewHandlerManager(hub *websocket.Hub, roomMgr *room.Manager, coordinator *match.Coordinator, db *sql.DB) *HandlerManager {
	gameLogic := game.NewGameLogic(hub, roomMgr, db, &game.MessageTypes{
		MsgPassNotify:         types.MsgPassNotify,
		MsgPlayCardsNotify:    types.MsgPlayCardsNotify,
		MsgTimerNotify:        types.MsgTimerNotify,
		MsgCallLandlordNotify: types.MsgCallLandlordNotify,
		MsgDealCardsNotify:    types.MsgDealCardsNotify,
		MsgGameEndNotify:      types.MsgGameEndNotify,
	})

	hm := &HandlerManager{
		hub:         hub,
		roomMgr:     roomMgr,
		coordinator: coordinator,
		db:          db,
		gameLogic:   gameLogic,
	}

	roomMgr.SetOnStartGame(func(r *room.Room) {
		logx.Infof("Room %s: starting game from callback", r.ID)
		gsm := r.InitGameState()
		logx.Infof("Room %s: game state initialized", r.ID)
		hm.startGameWithState(r, gsm)
	})

	roomMgr.SetOnBotPlayerJoined(func(roomID, uid, nickname string, isBot, isReady bool) {
		logx.Infof("Room %s: bot player joined callback triggered", roomID)
		hm.broadcastMsg(roomID, types.MsgRoomStateNotify, map[string]interface{}{
			"event":    "player_joined",
			"uid":      uid,
			"nickname": nickname,
			"is_bot":   isBot,
			"is_ready": isReady,
		})
	})

	// 设置游戏结束回调（保存数据库）
	roomMgr.SetOnGameEnd(func(r *room.Room, gsm *room.GameStateMachine) {
		logx.Infof("Room %s: game end callback triggered, saving to database", r.ID)
		hm.gameLogic.HandleGameEnd(r, gsm, hm.broadcastMsg)
	})

	roomMgr.SetOnRoomBotJoinCountdown(func(room *room.Room, seconds int) {
		logx.Infof("Room %s: bot join countdown callback: %d seconds", room.ID, seconds)
		hm.broadcastMsg(room.ID, types.MsgRoomStateNotify, map[string]interface{}{
			"event":   "bot_join_countdown",
			"seconds": seconds,
		})
	})

	return hm
}

// RegisterAll 注册所有消息处理器
func (hm *HandlerManager) RegisterAll() {
	hm.hub.RegisterHandler(types.MsgHeartbeatReq, hm.handleHeartbeat)
	hm.hub.RegisterHandler(types.MsgLoginReq, hm.handleLogin)
	hm.hub.RegisterHandler(types.MsgCreateRoomReq, hm.handleCreateRoom)
	hm.hub.RegisterHandler(types.MsgJoinRoomReq, hm.handleJoinRoom)
	hm.hub.RegisterHandler(types.MsgPlayerReadyReq, hm.handlePlayerReady)
	hm.hub.RegisterHandler(types.MsgCallLandlordReq, hm.handleCallLandlord)
	hm.hub.RegisterHandler(types.MsgPlayCardsReq, hm.handlePlayCards)
	hm.hub.RegisterHandler(types.MsgCancelAIControlReq, hm.handleCancelAIControl)
	hm.hub.RegisterHandler(types.MsgReconnectReq, hm.handleReconnect)
	hm.hub.RegisterHandler(types.MsgMatchStartReq, hm.handleMatchStart)
	hm.hub.RegisterHandler(types.MsgMatchCancelReq, hm.handleMatchCancel)

	logx.Info("All message handlers registered")
}

// sendMsg 发送 JSON 消息
func (hm *HandlerManager) sendMsg(client *websocket.Client, msgID uint16, data interface{}) {
	logx.Infof("sendMsg called: client=%s, msgID=0x%04X, data=%v", client.ID, msgID, data)
	payload, err := json.Marshal(data)
	if err != nil {
		logx.Errorf("Failed to marshal response: %v", err)
		return
	}
	logx.Infof("sendMsg: marshalled payload=%v", payload)

	if err := client.SendMsg(msgID, payload); err != nil {
		logx.Errorf("Failed to send message to %s: %v", client.ID, err)
	} else {
		logx.Infof("sendMsg: message sent successfully to %s", client.ID)
	}
}

// broadcastMsg 广播 JSON 消息到房间
func (hm *HandlerManager) broadcastMsg(roomID string, msgID uint16, data interface{}) {
	logx.Infof("broadcastMsg called: roomID=%s, msgID=0x%04X, data=%v", roomID, msgID, data)
	payload, err := json.Marshal(data)
	if err != nil {
		logx.Errorf("Failed to marshal broadcast message: %v", err)
		return
	}

	hm.hub.BroadcastToRoom(roomID, msgID, payload)
	logx.Infof("broadcastMsg: message sent to room %s", roomID)
}

// sendError 发送错误响应
func (hm *HandlerManager) sendError(client *websocket.Client, originalMsgID uint16, code int, message string) {
	hm.sendMsg(client, types.MsgErrorResponse, map[string]interface{}{
		"code":    code,
		"message": message,
		"msg_id":  originalMsgID,
	})
}
