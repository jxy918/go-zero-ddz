package handler

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"go-zero-ddz/app/game/internal/game"
	"go-zero-ddz/app/game/internal/match"
	"go-zero-ddz/app/game/internal/room"
	"go-zero-ddz/app/game/internal/websocket"
	"go-zero-ddz/pkg/cardutil"
	"go-zero-ddz/pkg/types"
)

// parseCards 解析牌数据：支持字符串格式 ["S3", "H7"] 和对象格式 [{value: 3, suit: 1}]
func parseCards(data interface{}) ([]cardutil.Card, error) {
	if data == nil {
		return []cardutil.Card{}, nil
	}

	// 尝试作为字符串数组解析
	if strArr, ok := data.([]interface{}); ok {
		cards := make([]cardutil.Card, 0, len(strArr))
		for _, item := range strArr {
			if str, ok := item.(string); ok {
				card, err := parseCardString(str)
				if err != nil {
					return nil, err
				}
				cards = append(cards, card)
			} else if obj, ok := item.(map[string]interface{}); ok {
				// 对象格式
				card, err := parseCardObject(obj)
				if err != nil {
					return nil, err
				}
				cards = append(cards, card)
			}
		}
		return cards, nil
	}

	// 尝试作为对象数组解析
	if objArr, ok := data.([]map[string]interface{}); ok {
		cards := make([]cardutil.Card, 0, len(objArr))
		for _, obj := range objArr {
			card, err := parseCardObject(obj)
			if err != nil {
				return nil, err
			}
			cards = append(cards, card)
		}
		return cards, nil
	}

	return nil, fmt.Errorf("unsupported card format")
}

// parseCardString 解析字符串格式的牌（如 "S3", "H7", "JS", "JB"）
func parseCardString(s string) (cardutil.Card, error) {
	if len(s) < 2 {
		return cardutil.Card{}, fmt.Errorf("invalid card string: %s", s)
	}

	suitMap := map[byte]cardutil.CardSuit{
		'S': cardutil.CardSuitSpade,
		'H': cardutil.CardSuitHeart,
		'C': cardutil.CardSuitClub,
		'D': cardutil.CardSuitDiamond,
		'J': cardutil.CardSuitJoker,
	}

	valueStr := s[1:]
	var value cardutil.CardValue

	switch valueStr {
	case "3":
		value = cardutil.CardValue3
	case "4":
		value = cardutil.CardValue4
	case "5":
		value = cardutil.CardValue5
	case "6":
		value = cardutil.CardValue6
	case "7":
		value = cardutil.CardValue7
	case "8":
		value = cardutil.CardValue8
	case "9":
		value = cardutil.CardValue9
	case "10":
		value = cardutil.CardValue10
	case "J":
		value = cardutil.CardValueJ
	case "Q":
		value = cardutil.CardValueQ
	case "K":
		value = cardutil.CardValueK
	case "A":
		value = cardutil.CardValueA
	case "2":
		value = cardutil.CardValue2
	case "S":
		value = cardutil.CardValueJokerS
	case "B":
		value = cardutil.CardValueJokerB
	default:
		return cardutil.Card{}, fmt.Errorf("invalid card value: %s", valueStr)
	}

	suit, ok := suitMap[s[0]]
	if !ok {
		return cardutil.Card{}, fmt.Errorf("invalid card suit: %c", s[0])
	}

	return cardutil.Card{Value: value, Suit: suit}, nil
}

// parseCardObject 解析对象格式的牌（如 {value: 3, suit: 1}）
func parseCardObject(obj map[string]interface{}) (cardutil.Card, error) {
	value, ok := obj["value"].(float64)
	if !ok {
		return cardutil.Card{}, fmt.Errorf("invalid card value")
	}
	suit, ok := obj["suit"].(float64)
	if !ok {
		return cardutil.Card{}, fmt.Errorf("invalid card suit")
	}
	return cardutil.Card{Value: cardutil.CardValue(value), Suit: cardutil.CardSuit(suit)}, nil
}

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
	// 创建游戏逻辑管理器
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

	// 设置开始游戏回调
	roomMgr.SetOnStartGame(func(r *room.Room) {
		log.Printf("Room %s: starting game from callback", r.ID)
		gsm := r.InitGameState()
		log.Printf("Room %s: game state initialized", r.ID)
		// 直接传递 gsm，避免在 startGame 中再次获取锁
		hm.startGameWithState(r, gsm)
	})

	// 设置机器人加入回调
	roomMgr.SetOnBotPlayerJoined(func(roomID, uid, nickname string, isBot, isReady bool) {
		log.Printf("Room %s: bot player joined callback triggered", roomID)
		hm.broadcastMsg(roomID, types.MsgRoomStateNotify, map[string]interface{}{
			"event":    "player_joined",
			"uid":      uid,
			"nickname": nickname,
			"is_bot":   isBot,
			"is_ready": isReady,
		})
	})

	// 设置机器人倒计时回调
	roomMgr.SetOnRoomBotJoinCountdown(func(room *room.Room, seconds int) {
		log.Printf("Room %s: bot join countdown callback: %d seconds", room.ID, seconds)
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

	log.Println("All message handlers registered")
}

// handleHeartbeat 处理心跳
func (hm *HandlerManager) handleHeartbeat(client *websocket.Client, msgID uint16, payload []byte) {
	client.UpdateHeartbeat()

	// 回复心跳响应
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

	// 从 JWT token 中提取 UID（不验证签名，仅解析 payload）
	uid := extractUIDFromJWT(req.Token)
	if uid == "" {
		// 回退：使用完整 token 作为 UID
		uid = req.Token
	}
	client.UID = uid
	log.Printf("Player %s logged in (conn: %s)", uid, client.ID)

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
	// JWT 格式: header.payload.signature
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return ""
	}

	// base64 解码 payload（第二部分）
	payloadBytes, err := base64.RawStdEncoding.DecodeString(parts[1])
	// 尝试标准 base64（带 padding）
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

// handleCreateRoom 处理创建房间（自动填充 2 个 AI 机器人实现单人测试）
func (hm *HandlerManager) handleCreateRoom(client *websocket.Client, msgID uint16, payload []byte) {
	log.Printf("handleCreateRoom called: client=%s, UID=%s, payload=%v", client.ID, client.UID, payload)
	if client.UID == "" {
		hm.sendError(client, msgID, 401, "not logged in")
		return
	}

	// 如果玩家已经在房间中，先清理旧房间
	if client.RoomID != "" {
		if existingRoom, exists := hm.roomMgr.GetRoom(client.RoomID); exists {
			existingRoom.RemovePlayer(client.UID)
		}
		client.RoomID = ""
	}

	roomID := room.GenerateID()
	r, err := hm.roomMgr.CreateRoom(roomID)
	if err != nil {
		hm.sendError(client, msgID, 500, err.Error())
		return
	}

	// 添加人类玩家
	humanPlayer := &room.Player{
		UID:            client.UID,
		Nickname:       truncateNickname(client.UID, 8),
		IsOnline:       true,
		IsReady:        false, // 初始化为未准备状态
		IsAIControlled: false, // 确保用户初始状态不是AI托管
	}
	if err := r.AddPlayer(humanPlayer); err != nil {
		hm.sendError(client, msgID, 500, err.Error())
		return
	}
	client.RoomID = roomID

	log.Printf("Room %s created by %s (1 player)", roomID, client.UID)
	log.Printf("Room %s PlayerIDs order: %v", roomID, r.PlayerIDs)

	// 发送创建房间响应
	log.Printf("Sending CREATE_ROOM_RESP to client %s (UID: %s)", client.ID, client.UID)
	hm.sendMsg(client, types.MsgCreateRoomResp, map[string]interface{}{
		"success": true,
		"room_id": roomID,
	})

	// 房间满了且所有人都准备了，自动开始游戏
	log.Printf("Room %s: checking if all ready. Player count: %d", roomID, r.Count())
	if r.AllReady() {
		log.Printf("Room %s: all players ready (with bots), starting game", roomID)
		r.InitGameState()
		hm.startGame(r)
		// 如果第一个玩家是 Bot，触发 AI
		hm.roomMgr.TriggerBotIfNeeded(r)
	} else {
		log.Printf("Room %s: not all players ready", roomID)
	}
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

	// 检查玩家是否已经在房间中（断线重连场景）
	if existingPlayer, exists := r.GetPlayer(client.UID); exists {
		// 更新玩家状态（重新连接）
		existingPlayer.IsOnline = true
		client.RoomID = req.RoomID
		log.Printf("Player %s reconnected to room %s", client.UID, req.RoomID)
		hm.sendMsg(client, types.MsgJoinRoomResp, map[string]interface{}{
			"success": true,
		})
		return
	}

	player := &room.Player{
		UID:            client.UID,
		Nickname:       "Player_" + client.UID[:nickLen],
		IsOnline:       true,
		IsAIControlled: false, // 确保用户初始状态不是AI托管
	}
	if err := r.AddPlayer(player); err != nil {
		hm.sendError(client, msgID, 500, err.Error())
		return
	}

	client.RoomID = req.RoomID

	log.Printf("Player %s joined room %s", client.UID, req.RoomID)
	hm.sendMsg(client, types.MsgJoinRoomResp, map[string]interface{}{
		"success": true,
	})

	hm.broadcastMsg(req.RoomID, types.MsgRoomStateNotify, map[string]interface{}{
		"event": "player_joined",
		"uid":   client.UID,
		"count": r.Count(),
	})

	// 通知房间管理器玩家加入，重置计时器
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

	log.Printf("Player %s ready in room %s", client.UID, client.RoomID)

	hm.broadcastMsg(client.RoomID, types.MsgRoomStateNotify, map[string]interface{}{
		"event":    "player_ready",
		"uid":      client.UID,
		"is_ready": true,
	})

	// 通知房间管理器玩家准备（会在所有玩家准备后启动机器人加入计时器）
	hm.roomMgr.PlayerReady(r, client.UID)

	// 注意：不再直接调用 r.AllReady() 和 hm.startGame()
	// 这个逻辑现在统一放在 roomMgr.checkStartGame 中处理
	// 避免锁冲突
}

// handleCallLandlord 处理叫地主
func (hm *HandlerManager) handleCallLandlord(client *websocket.Client, msgID uint16, payload []byte) {
	log.Printf("=== handleCallLandlord START (delegated to gameLogic) ===")

	type CallReq struct {
		Action int   `json:"action"`
		Score  int32 `json:"score"`
	}
	var req CallReq
	if err := json.Unmarshal(payload, &req); err != nil {
		log.Printf("Failed to unmarshal call landlord request: %v", err)
		hm.sendError(client, msgID, 400, "invalid request")
		return
	}

	// 调用 game 模块处理叫地主逻辑
	hm.gameLogic.HandleCallLandlord(client, req.Action, req.Score, msgID, hm.sendError, hm.broadcastMsg, hm.sendMsg)
}

// handlePlayCards 处理出牌
func (hm *HandlerManager) handlePlayCards(client *websocket.Client, msgID uint16, payload []byte) {
	log.Printf("=== handlePlayCards START (delegated to gameLogic) ===")

	type PlayReq struct {
		Cards interface{} `json:"cards"` // 接受字符串数组或对象数组
	}
	var req PlayReq
	if err := json.Unmarshal(payload, &req); err != nil {
		log.Printf("ERROR: failed to unmarshal request: %v", err)
		hm.sendError(client, msgID, 400, "invalid request")
		return
	}

	// 转换牌数据：支持字符串格式 ["S3", "H7"] 或对象格式 [{value: 3, suit: 1}]
	cards, err := parseCards(req.Cards)
	if err != nil {
		log.Printf("ERROR: failed to parse cards: %v", err)
		hm.sendError(client, msgID, 400, "invalid cards format")
		return
	}

	// 调用 game 模块处理出牌逻辑
	hm.gameLogic.HandlePlayCards(client, cards, msgID, hm.sendError, hm.broadcastMsg)
}

// handleCancelAIControl 处理取消AI托管请求
func (hm *HandlerManager) handleCancelAIControl(client *websocket.Client, msgID uint16, payload []byte) {
	log.Printf("=== handleCancelAIControl START ===")
	log.Printf("client=%s, UID=%s, RoomID=%s", client.ID, client.UID, client.RoomID)

	r, exists := hm.roomMgr.GetRoom(client.RoomID)
	if !exists || r == nil {
		log.Printf("handleCancelAIControl: room not found")
		return
	}

	player, exists := r.GetPlayer(client.UID)
	if !exists {
		log.Printf("handleCancelAIControl: player not found")
		return
	}

	if player.IsBot {
		log.Printf("handleCancelAIControl: cannot cancel AI control for bot player")
		return
	}

	// 取消AI托管
	player.IsAIControlled = false
	log.Printf("handleCancelAIControl: player %s AI control disabled", client.UID)

	// 如果当前轮到该玩家，启动计时器
	if r.CurrentTurnUID == client.UID && r.State == types.StatePlaying {
		r.StartTimer(15, client.UID)
		hm.broadcastMsg(client.RoomID, types.MsgTimerNotify, map[string]interface{}{
			"remaining_seconds": 15,
			"current_turn_uid":  client.UID,
		})
		log.Printf("handleCancelAIControl: started timer for player %s", client.UID)
	}
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

	// 构建玩家信息
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

	// 构建响应数据
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

	// 如果在出牌阶段，添加更多信息
	if r.State == types.StatePlaying {
		respData["current_turn_uid"] = r.CurrentTurnUID
		respData["last_played_uid"] = r.LastPlayedUID
		respData["last_played_cards"] = r.LastPlayedCards
	}

	hm.sendMsg(client, types.MsgReconnectResp, respData)

	log.Printf("Player %s reconnected to room %s, state: %d", client.UID, req.RoomID, r.State)
}

// startGame 开始游戏
func (hm *HandlerManager) startGame(r *room.Room) {
	gsm := r.GetGameState()
	if gsm == nil {
		log.Printf("Room %s: game state not initialized", r.ID)
		return
	}
	hm.startGameWithState(r, gsm)
}

// startGameWithState 使用已初始化的游戏状态开始游戏（避免重复获取锁）
func (hm *HandlerManager) startGameWithState(r *room.Room, gsm *room.GameStateMachine) {
	log.Printf("Room %s: starting game with state", r.ID)

	// 重置所有玩家的 AI 托管状态
	for _, uid := range r.PlayerIDs {
		if player, exists := r.GetPlayer(uid); exists {
			player.IsAIControlled = false
		}
	}

	hands, _, err := gsm.DealCards()
	if err != nil {
		log.Printf("Failed to deal cards: %v", err)
		return
	}

	for i, uid := range r.PlayerIDs {
		c := hm.hub.GetClientByUID(uid)
		if c == nil {
			continue
		}

		hm.sendMsg(c, types.MsgDealCardsNotify, map[string]interface{}{
			"my_cards":           hands[i],
			"bottom_cards":       r.BottomCards,
			"first_caller_index": i,
			"players":            r.PlayerIDs,
			"landlord_uid":       "", // 叫地主阶段还没有地主
		})
	}

	firstCaller := r.PlayerIDs[0]
	log.Printf("startGameWithState: firstCaller=%s, PlayerIDs=%v", firstCaller, r.PlayerIDs)
	r.StartTimer(15, firstCaller)

	hm.broadcastMsg(r.ID, types.MsgTimerNotify, map[string]interface{}{
		"remaining_seconds": 15,
		"current_turn_uid":  firstCaller,
	})

	// 发送"轮到第一位玩家叫地主"的通知（turn=true 表示轮到叫地主）
	hm.broadcastMsg(r.ID, types.MsgCallLandlordNotify, map[string]interface{}{
		"uid":    firstCaller,
		"action": 0,
		"score":  0,
		"turn":   true, // 标识这是轮到叫地主的通知
	})
}

// sendMsg 发送 JSON 消息
func (hm *HandlerManager) sendMsg(client *websocket.Client, msgID uint16, data interface{}) {
	log.Printf("sendMsg called: client=%s, msgID=0x%04X, data=%v", client.ID, msgID, data)
	payload, err := json.Marshal(data)
	if err != nil {
		log.Printf("Failed to marshal response: %v", err)
		return
	}
	log.Printf("sendMsg: marshalled payload=%v", payload)

	if err := client.SendMsg(msgID, payload); err != nil {
		log.Printf("Failed to send message to %s: %v", client.ID, err)
	} else {
		log.Printf("sendMsg: message sent successfully to %s", client.ID)
	}
}

// broadcastMsg 广播 JSON 消息到房间
func (hm *HandlerManager) broadcastMsg(roomID string, msgID uint16, data interface{}) {
	log.Printf("broadcastMsg called: roomID=%s, msgID=0x%04X, data=%v", roomID, msgID, data)
	payload, err := json.Marshal(data)
	if err != nil {
		log.Printf("Failed to marshal broadcast message: %v", err)
		return
	}

	hm.hub.BroadcastToRoom(roomID, msgID, payload)
	log.Printf("broadcastMsg: message sent to room %s", roomID)
}

// sendError 发送错误响应
func (hm *HandlerManager) sendError(client *websocket.Client, originalMsgID uint16, code int, message string) {
	hm.sendMsg(client, types.MsgErrorResponse, map[string]interface{}{
		"code":    code,
		"message": message,
		"msg_id":  originalMsgID,
	})
}

// handleMatchStart 处理开始匹配请求
func (hm *HandlerManager) handleMatchStart(client *websocket.Client, msgID uint16, payload []byte) {
	if client.UID == "" {
		hm.sendError(client, msgID, 401, "not logged in")
		return
	}

	type MatchStartReq struct {
		MatchType int    `json:"match_type"` // 0=random, 1=ranked
		ELO       int32  `json:"elo"`
		Tier      string `json:"tier"`
	}
	var req MatchStartReq
	if err := json.Unmarshal(payload, &req); err != nil {
		hm.sendError(client, msgID, 400, "invalid request")
		return
	}

	if hm.coordinator == nil {
		hm.sendError(client, msgID, 503, "matchmaking not available")
		return
	}

	player := &match.WaitingPlayer{
		UID:       client.UID,
		ELO:       req.ELO,
		Tier:      req.Tier,
		WaitStart: time.Now(),
		MatchType: match.MatchType(req.MatchType),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := hm.coordinator.Enqueue(ctx, player); err != nil {
		hm.sendError(client, msgID, 500, "failed to join queue: "+err.Error())
		return
	}

	log.Printf("Player %s joined matchmaking (type=%d, elo=%d, tier=%s)", client.UID, req.MatchType, req.ELO, req.Tier)

	hm.sendMsg(client, types.MsgMatchStartReq, map[string]interface{}{
		"success":    true,
		"match_type": req.MatchType,
		"message":    "已加入匹配队列",
	})
}

// handleMatchCancel 处理取消匹配
func (hm *HandlerManager) handleMatchCancel(client *websocket.Client, msgID uint16, payload []byte) {
	if hm.coordinator != nil {
		hm.coordinator.Cancel(context.Background(), client.UID)
	}
	log.Printf("Player %s cancelled matchmaking", client.UID)
}

// truncateNickname 截断昵称到指定长度
func truncateNickname(s string, maxLen int) string {
	if len(s) > maxLen {
		return s[:maxLen]
	}
	return s
}
