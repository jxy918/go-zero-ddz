package handler

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"time"

	"go-zero-ddz/app/game/internal/websocket"
	"go-zero-ddz/pkg/types"

	"github.com/zeromicro/go-zero/core/logx"
)

// handleHeartbeat 处理心跳
func (hm *HandlerManager) handleHeartbeat(client *websocket.Client, msgID uint16, payload []byte) {
	client.UpdateHeartbeat()

	type HeartbeatReq struct {
		ClientTimestamp int64 `json:"client_timestamp"`
	}
	type HeartbeatResp struct {
		ServerTimestamp int64 `json:"server_timestamp"`
		Ping            int32 `json:"ping"`
	}

	var req HeartbeatReq
	json.Unmarshal(payload, &req)

	serverTime := time.Now().UnixMilli()
	ping := int32(0)
	if req.ClientTimestamp > 0 {
		ping = int32(serverTime - req.ClientTimestamp)
	}

	hm.sendMsg(client, types.MsgHeartbeatResp, HeartbeatResp{
		ServerTimestamp: serverTime,
		Ping:            ping,
	})
}

// handleLogin 处理登录
func (hm *HandlerManager) handleLogin(client *websocket.Client, msgID uint16, payload []byte) {
	type LoginReq struct {
		Token string `json:"token"`
	}

	var req LoginReq
	if err := json.Unmarshal(payload, &req); err != nil {
		hm.sendError(client, msgID, 400, "invalid request")
		return
	}

	uid := extractUIDFromJWT(req.Token)
	if uid == "" {
		uid = req.Token
	}
	client.UID = uid
	logx.Infof("Player %s logged in (conn: %s)", uid, client.ID)

	hm.sendMsg(client, types.MsgLoginResp, map[string]interface{}{
		"success":  true,
		"uid":      uid,
		"nickname": uid,
		"elo":      1000,
		"tier":     "青铜I",
		"gold":     10000,
	})
}

// extractUIDFromJWT 从 JWT token 中提取 uid 字段（不验证签名）
func extractUIDFromJWT(token string) string {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return ""
	}

	payloadBytes, err := base64.RawStdEncoding.DecodeString(parts[1])
	if err != nil {
		payloadBytes, err = base64.StdEncoding.DecodeString(parts[1])
	}
	if err != nil {
		return ""
	}

	var claims map[string]interface{}
	if err := json.Unmarshal(payloadBytes, &claims); err != nil {
		return ""
	}

	uid, ok := claims["uid"].(string)
	if !ok {
		return ""
	}
	return uid
}

// handleReconnect 处理断线重连
func (hm *HandlerManager) handleReconnect(client *websocket.Client, msgID uint16, payload []byte) {
	type ReconnectReq struct {
		SessionKey string `json:"session_key"`
		RoomID     string `json:"room_id"`
	}
	var req ReconnectReq
	if err := json.Unmarshal(payload, &req); err != nil {
		hm.sendError(client, msgID, 400, "invalid request")
		return
	}

	r, exists := hm.roomMgr.GetRoom(req.RoomID)
	if !exists {
		hm.sendError(client, msgID, 404, "room not found")
		return
	}

	r.SetOnline(client.UID, true)
	client.RoomID = req.RoomID

	player, _ := r.GetPlayer(client.UID)

	playersInfo := make([]map[string]interface{}, 0, len(r.PlayerIDs))
	for _, uid := range r.PlayerIDs {
		p, _ := r.GetPlayer(uid)
		playersInfo = append(playersInfo, map[string]interface{}{
			"uid":              uid,
			"nickname":         p.Nickname,
			"is_landlord":      p.IsLandlord,
			"is_bot":           p.IsBot,
			"is_ai_controlled": p.IsAIControlled,
		})
	}

	respData := map[string]interface{}{
		"success":      true,
		"room_id":      r.ID,
		"game_state":   r.State,
		"my_cards":     player.Cards,
		"players":      playersInfo,
		"landlord_uid": r.LandlordUID,
		"bottom_cards": r.BottomCards,
		"call_score":   r.CallScore,
		"pass_count":   r.PassCount,
	}

	if r.State == types.StatePlaying {
		respData["current_turn_uid"] = r.CurrentTurnUID
		respData["last_played_uid"] = r.LastPlayedUID
		respData["last_played_cards"] = r.LastPlayedCards
	}

	hm.sendMsg(client, types.MsgReconnectResp, respData)

	logx.Infof("Player %s reconnected to room %s, state: %d", client.UID, req.RoomID, r.State)
}
