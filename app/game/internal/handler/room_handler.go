package handler

import (
	"encoding/json"

	"go-zero-ddz/app/game/internal/room"
	"go-zero-ddz/app/game/internal/websocket"
	"go-zero-ddz/pkg/types"

	"github.com/zeromicro/go-zero/core/logx"
)

// handleCreateRoom 处理创建房间（自动填充 2 个 AI 机器人实现单人测试）
func (hm *HandlerManager) handleCreateRoom(client *websocket.Client, msgID uint16, payload []byte) {
	logx.Infof("[DEBUG] handleCreateRoom called: client=%s, UID=%s, payload=%v", client.ID, client.UID, payload)
	if client.UID == "" {
		logx.Infof("[DEBUG] handleCreateRoom: UID is empty, returning error")
		hm.sendError(client, msgID, 401, "not logged in")
		return
	}

	if client.RoomID != "" {
		logx.Infof("[DEBUG] handleCreateRoom: client already in room %s", client.RoomID)
		if existingRoom, exists := hm.roomMgr.GetRoom(client.RoomID); exists {
			existingRoom.RemovePlayer(client.UID)
		}
		client.RoomID = ""
	}

	logx.Infof("[DEBUG] handleCreateRoom: generating room ID")
	roomID := room.GenerateID()
	logx.Infof("[DEBUG] handleCreateRoom: generated room ID: %s", roomID)

	logx.Infof("[DEBUG] handleCreateRoom: creating room")
	r, err := hm.roomMgr.CreateRoom(roomID)
	if err != nil {
		logx.Infof("[DEBUG] handleCreateRoom: CreateRoom failed: %v", err)
		hm.sendError(client, msgID, 500, err.Error())
		return
	}
	logx.Infof("[DEBUG] handleCreateRoom: room created successfully")

	humanPlayer := &room.Player{
		UID:            client.UID,
		Nickname:       truncateNickname(client.UID, 8),
		IsOnline:       true,
		IsReady:        false,
		IsAIControlled: false,
	}
	logx.Infof("[DEBUG] handleCreateRoom: adding player to room")
	if err := r.AddPlayer(humanPlayer); err != nil {
		logx.Infof("[DEBUG] handleCreateRoom: AddPlayer failed: %v", err)
		hm.sendError(client, msgID, 500, err.Error())
		return
	}
	logx.Infof("[DEBUG] handleCreateRoom: player added successfully")

	client.RoomID = roomID
	logx.Infof("[DEBUG] handleCreateRoom: client.RoomID set to %s", roomID)

	logx.Infof("[DEBUG] handleCreateRoom: Room %s created by %s (1 player)", roomID, client.UID)
	logx.Infof("[DEBUG] handleCreateRoom: Room %s PlayerIDs order: %v", roomID, r.PlayerIDs)

	logx.Infof("[DEBUG] handleCreateRoom: Sending CREATE_ROOM_RESP to client %s (UID: %s)", client.ID, client.UID)
	hm.sendMsg(client, types.MsgCreateRoomResp, map[string]interface{}{
		"success": true,
		"room_id": roomID,
	})
	logx.Infof("[DEBUG] handleCreateRoom: CREATE_ROOM_RESP sent")

	logx.Infof("[DEBUG] handleCreateRoom: Room %s: checking if all ready. Player count: %d", roomID, r.Count())
	if r.AllReady() {
		logx.Infof("[DEBUG] handleCreateRoom: Room %s: all players ready (with bots), starting game", roomID)
		r.InitGameState()
		hm.startGame(r)
		hm.roomMgr.TriggerBotIfNeeded(r)
	} else {
		logx.Infof("[DEBUG] handleCreateRoom: Room %s: not all players ready", roomID)
	}
	logx.Infof("[DEBUG] handleCreateRoom: completed")
}

// handleJoinRoom 处理加入房间
func (hm *HandlerManager) handleJoinRoom(client *websocket.Client, msgID uint16, payload []byte) {
	if client.UID == "" {
		hm.sendError(client, msgID, 401, "not logged in")
		return
	}

	type JoinReq struct {
		RoomID string `json:"room_id"`
	}
	var req JoinReq
	if err := json.Unmarshal(payload, &req); err != nil {
		hm.sendError(client, msgID, 400, "invalid request")
		return
	}

	r, exists := hm.roomMgr.GetRoom(req.RoomID)
	if !exists {
		hm.sendError(client, msgID, 404, "room not found")
		return
	}

	nickLen := 4
	if len(client.UID) < 4 {
		nickLen = len(client.UID)
	}

	if existingPlayer, exists := r.GetPlayer(client.UID); exists {
		existingPlayer.IsOnline = true
		client.RoomID = req.RoomID
		logx.Infof("Player %s reconnected to room %s", client.UID, req.RoomID)
		hm.sendMsg(client, types.MsgJoinRoomResp, map[string]interface{}{
			"success": true,
		})
		return
	}

	player := &room.Player{
		UID:            client.UID,
		Nickname:       "Player_" + client.UID[:nickLen],
		IsOnline:       true,
		IsAIControlled: false,
	}
	if err := r.AddPlayer(player); err != nil {
		hm.sendError(client, msgID, 500, err.Error())
		return
	}

	client.RoomID = req.RoomID

	logx.Infof("Player %s joined room %s", client.UID, req.RoomID)
	hm.sendMsg(client, types.MsgJoinRoomResp, map[string]interface{}{
		"success": true,
	})

	hm.broadcastMsg(req.RoomID, types.MsgRoomStateNotify, map[string]interface{}{
		"event": "player_joined",
		"uid":   client.UID,
		"count": r.Count(),
	})

	hm.roomMgr.PlayerJoined(r, client.UID)
}

// handlePlayerReady 处理玩家准备
func (hm *HandlerManager) handlePlayerReady(client *websocket.Client, msgID uint16, payload []byte) {
	if client.RoomID == "" {
		hm.sendError(client, msgID, 400, "not in a room")
		return
	}

	r, exists := hm.roomMgr.GetRoom(client.RoomID)
	if !exists {
		hm.sendError(client, msgID, 404, "room not found")
		return
	}

	if err := r.SetReady(client.UID, true); err != nil {
		hm.sendError(client, msgID, 500, err.Error())
		return
	}

	logx.Infof("Player %s ready in room %s", client.UID, client.RoomID)

	hm.broadcastMsg(client.RoomID, types.MsgRoomStateNotify, map[string]interface{}{
		"event":    "player_ready",
		"uid":      client.UID,
		"is_ready": true,
	})

	hm.roomMgr.PlayerReady(r, client.UID)
}

// truncateNickname 截断昵称到指定长度
func truncateNickname(s string, maxLen int) string {
	if len(s) > maxLen {
		return s[:maxLen]
	}
	return s
}
