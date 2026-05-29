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

	"go-zero-ddz/app/game/internal/match"
	"go-zero-ddz/app/game/internal/room"
	"go-zero-ddz/app/game/internal/websocket"
	"go-zero-ddz/pkg/cardutil"
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

// MessageIDs 消息 ID 常量
const (
	MsgHeartbeatReq  uint16 = 0x0001
	MsgHeartbeatResp uint16 = 0x0002
	MsgErrorResponse uint16 = 0x0003

	MsgLoginReq  uint16 = 0x0101
	MsgLoginResp uint16 = 0x0102

	MsgCreateRoomReq   uint16 = 0x0201
	MsgCreateRoomResp  uint16 = 0x0202
	MsgJoinRoomReq     uint16 = 0x0203
	MsgJoinRoomResp    uint16 = 0x0204
	MsgRoomStateNotify uint16 = 0x0206
	MsgPlayerReadyReq  uint16 = 0x0207

	MsgMatchStartReq      uint16 = 0x0301
	MsgMatchCancelReq     uint16 = 0x0302
	MsgMatchSuccessNotify uint16 = 0x0303

	MsgDealCardsNotify    uint16 = 0x0401
	MsgCallLandlordReq    uint16 = 0x0402
	MsgCallLandlordNotify uint16 = 0x0403
	MsgPlayCardsReq       uint16 = 0x0404
	MsgPlayCardsNotify    uint16 = 0x0405
	MsgPassNotify         uint16 = 0x0406
	MsgGameEndNotify      uint16 = 0x0407
	MsgTimerNotify        uint16 = 0x0408
	MsgCancelAIControlReq uint16 = 0x0409

	MsgReconnectReq  uint16 = 0x0501
	MsgReconnectResp uint16 = 0x0502
)

// HandlerManager 消息处理器管理器
type HandlerManager struct {
	hub         *websocket.Hub
	roomMgr     *room.Manager
	coordinator *match.Coordinator
	db          *sql.DB
}

// NewHandlerManager 创建处理器管理器
func NewHandlerManager(hub *websocket.Hub, roomMgr *room.Manager, coordinator *match.Coordinator, db *sql.DB) *HandlerManager {
	hm := &HandlerManager{
		hub:         hub,
		roomMgr:     roomMgr,
		coordinator: coordinator,
		db:          db,
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
		hm.broadcastMsg(roomID, MsgRoomStateNotify, map[string]interface{}{
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
		hm.broadcastMsg(room.ID, MsgRoomStateNotify, map[string]interface{}{
			"event":   "bot_join_countdown",
			"seconds": seconds,
		})
	})

	return hm
}

// RegisterAll 注册所有消息处理器
func (hm *HandlerManager) RegisterAll() {
	hm.hub.RegisterHandler(MsgHeartbeatReq, hm.handleHeartbeat)
	hm.hub.RegisterHandler(MsgLoginReq, hm.handleLogin)
	hm.hub.RegisterHandler(MsgCreateRoomReq, hm.handleCreateRoom)
	hm.hub.RegisterHandler(MsgJoinRoomReq, hm.handleJoinRoom)
	hm.hub.RegisterHandler(MsgPlayerReadyReq, hm.handlePlayerReady)
	hm.hub.RegisterHandler(MsgCallLandlordReq, hm.handleCallLandlord)
	hm.hub.RegisterHandler(MsgPlayCardsReq, hm.handlePlayCards)
	hm.hub.RegisterHandler(MsgCancelAIControlReq, hm.handleCancelAIControl)
	hm.hub.RegisterHandler(MsgReconnectReq, hm.handleReconnect)
	hm.hub.RegisterHandler(MsgMatchStartReq, hm.handleMatchStart)
	hm.hub.RegisterHandler(MsgMatchCancelReq, hm.handleMatchCancel)

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

	hm.sendMsg(client, MsgHeartbeatResp, HeartbeatResp{
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

	hm.sendMsg(client, MsgLoginResp, map[string]interface{}{
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
	hm.sendMsg(client, MsgCreateRoomResp, map[string]interface{}{
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
		hm.sendMsg(client, MsgJoinRoomResp, map[string]interface{}{
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
	hm.sendMsg(client, MsgJoinRoomResp, map[string]interface{}{
		"success": true,
	})

	hm.broadcastMsg(req.RoomID, MsgRoomStateNotify, map[string]interface{}{
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

	hm.broadcastMsg(client.RoomID, MsgRoomStateNotify, map[string]interface{}{
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
	log.Printf("handleCallLandlord called: client.UID=%s, client.RoomID=%s, payload=%s", client.UID, client.RoomID, string(payload))
	
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

	log.Printf("Call landlord request: Action=%d, Score=%d", req.Action, req.Score)

	r, exists := hm.roomMgr.GetRoom(client.RoomID)
	if !exists {
		log.Printf("Room not found: %s", client.RoomID)
		hm.sendError(client, msgID, 404, "room not found")
		return
	}

	gsm := r.GetGameState()
	if gsm == nil {
		log.Printf("Game not started in room: %s", client.RoomID)
		hm.sendError(client, msgID, 500, "game not started")
		return
	}
	if err := gsm.CallLandlord(client.UID, req.Action, req.Score); err != nil {
		log.Printf("CallLandlord failed: %v", err)
		hm.sendError(client, msgID, 500, err.Error())
		return
	}

	log.Printf("Broadcasting call landlord result: uid=%s, action=%d, score=%d", client.UID, req.Action, req.Score)
	hm.broadcastMsg(client.RoomID, MsgCallLandlordNotify, map[string]interface{}{
		"uid":    client.UID,
		"action": req.Action,
		"score":  req.Score,
		"round":  gsm.CurrentCallRound(),
	})

	log.Printf("Room %s: after call, callCount=%d, round=%d, checking AllCalled()",
		client.RoomID, gsm.CallCount(), gsm.CurrentCallRound())

	// 如果还没叫完，延迟1秒后广播轮到下一个玩家叫地主
	// 这样用户可以先看到当前玩家的叫地主结果，然后再看到轮到谁叫地主
	if !gsm.AllCalled() {
		nextIdx := gsm.CurrentCallIdx()
		if nextIdx < 0 || nextIdx >= len(r.PlayerIDs) {
			nextIdx = 0
		}
		nextUID := r.PlayerIDs[nextIdx]

		roomID := client.RoomID

		time.AfterFunc(time.Second*1, func() {
			r, exists := hm.roomMgr.GetRoom(roomID)
			if !exists {
				return
			}
			r.CurrentTurnUID = nextUID
			r.StartTimer(15, nextUID)

			log.Printf("Room %s: next to call is %s", roomID, nextUID)
			hm.broadcastMsg(roomID, MsgCallLandlordNotify, map[string]interface{}{
				"uid":  nextUID,
				"turn": true,
			})
			hm.broadcastMsg(roomID, MsgTimerNotify, map[string]interface{}{
				"remaining_seconds": 15,
				"current_turn_uid":  nextUID,
			})
		})

		return
	}

	if gsm.AllCalled() {
		log.Printf("Room %s: all players have called, confirming landlord", r.ID)
		if err := gsm.ConfirmLandlord(); err != nil {
			log.Printf("Failed to confirm landlord: %v", err)
			return
		}
		log.Printf("Room %s: landlord confirmed: %s, currentTurn: %s, state: %d", r.ID, r.LandlordUID, r.CurrentTurnUID, r.State)

		// 通知所有玩家地主信息，先展示底牌
		playersInfo := make([]map[string]interface{}, 0, len(r.PlayerIDs))
		for _, uid := range r.PlayerIDs {
			player, _ := r.GetPlayer(uid)
			playersInfo = append(playersInfo, map[string]interface{}{
				"uid":              uid,
				"nickname":         player.Nickname,
				"is_landlord":      player.IsLandlord,
				"is_bot":           player.IsBot,
				"is_ai_controlled": player.IsAIControlled,
			})
		}

		// 1. 先广播显示底牌（翻开展示）和地主信息
		hm.broadcastMsg(r.ID, MsgCallLandlordNotify, map[string]interface{}{
			"uid":          r.LandlordUID,
			"action":       1,
			"score":        r.CallScore,
			"landlord_uid": r.LandlordUID,
			"players":      playersInfo,
			"bottom_cards": r.BottomCards,
		})

		// 2. 延迟5秒后才把底牌发给地主并开始出牌阶段
		roomID := r.ID
		landlordUID := r.LandlordUID
		hub := hm.hub
		roomMgr := hm.roomMgr
		time.AfterFunc(5*time.Second, func() {
			log.Printf("Room %s: 5 seconds passed, now sending bottom cards to landlord and starting game", roomID)

			// 获取房间
			r, exists := roomMgr.GetRoom(roomID)
			if !exists {
				return
			}

			// 确保进入出牌阶段前，所有玩家的 IsAIControlled 都是 false
			log.Printf("Room %s: resetting IsAIControlled for all non-bot players before starting play phase", r.ID)
			for _, uid := range r.PlayerIDs {
				if player, exists := r.GetPlayer(uid); exists && !player.IsBot {
					log.Printf("Room %s: player %s IsAIControlled before reset: %v", r.ID, uid, player.IsAIControlled)
					player.IsAIControlled = false
					log.Printf("Room %s: player %s reset IsAIControlled to false before starting play phase", r.ID, uid)
				}
			}

			// 给地主发送带底牌的手牌
			if landlord, exists := r.GetPlayer(landlordUID); exists {
				landlordClient := hub.GetClientByUID(landlord.UID)
				if landlordClient != nil {
					log.Printf("Room %s: Sending DEAL_CARDS_NOTIFY to landlord %s, cards=%d, bottom_cards=%d",
						roomID, landlord.UID, len(landlord.Cards), len(r.BottomCards))
					// 重新获取 playersInfo（底牌已添加到地主手中）
					playersInfoAfter := make([]map[string]interface{}, 0, len(r.PlayerIDs))
					for _, uid := range r.PlayerIDs {
						player, _ := r.GetPlayer(uid)
						playersInfoAfter = append(playersInfoAfter, map[string]interface{}{
							"uid":              uid,
							"nickname":         player.Nickname,
							"is_landlord":      player.IsLandlord,
							"is_bot":           player.IsBot,
							"is_ai_controlled": player.IsAIControlled,
						})
					}
					hm.sendMsg(landlordClient, MsgDealCardsNotify, map[string]interface{}{
						"your_cards":   landlord.Cards,
						"players":      playersInfoAfter,
						"counts":       r.GetCardCounts(),
						"is_landlord":  true,
						"bottom_cards": r.BottomCards,
						"landlord_uid": landlordUID,
					})
				}
			}

			// 给所有玩家广播手牌数量
			hm.broadcastMsg(r.ID, MsgPlayCardsNotify, map[string]interface{}{
				"uid":          "",
				"cards":        []interface{}{},
				"card_counts":  r.GetCardCounts(),
				"landlord_uid": landlordUID,
				"players":      playersInfo,
			})

			r.StartTimer(15, landlordUID)
			hm.broadcastMsg(r.ID, MsgTimerNotify, map[string]interface{}{
				"remaining_seconds": 15,
				"current_turn_uid":  landlordUID,
			})
			// 如果地主是 Bot，立即触发 AI
			log.Printf("Room %s: triggering bot if needed after landlord confirmed", roomID)
			roomMgr.TriggerBotIfNeeded(r)
		})
	} else {
		nextIdx := gsm.CurrentCallIdx()
		if nextIdx < 0 || nextIdx >= len(r.PlayerIDs) {
			nextIdx = 0
		}
		nextUID := r.PlayerIDs[nextIdx]
		r.StartTimer(15, nextUID)
		hm.broadcastMsg(r.ID, MsgTimerNotify, map[string]interface{}{
			"remaining_seconds": 15,
			"current_turn_uid":  nextUID,
		})
		// 通知客户端轮到叫地主
		hm.broadcastMsg(r.ID, MsgCallLandlordNotify, map[string]interface{}{
			"uid":    nextUID,
			"action": 0,
			"score":  0,
			"turn":   true,
		})
		// 如果下一个是 Bot，延迟触发 AI
		if nextPlayer, exists := r.GetPlayer(nextUID); exists && (nextPlayer.IsBot || nextPlayer.IsAIControlled) {
			time.AfterFunc(1500*time.Millisecond, func() {
				hm.roomMgr.TriggerBotIfNeeded(r)
			})
		}
	}
}

// handlePlayCards 处理出牌
func (hm *HandlerManager) handlePlayCards(client *websocket.Client, msgID uint16, payload []byte) {
	log.Printf("=== handlePlayCards START ===")
	log.Printf("client=%s, UID=%s, RoomID=%s, payload=%s", client.ID, client.UID, client.RoomID, string(payload))

	type PlayReq struct {
		Cards interface{} `json:"cards"` // 接受字符串数组或对象数组
	}
	var req PlayReq
	if err := json.Unmarshal(payload, &req); err != nil {
		log.Printf("ERROR: failed to unmarshal request: %v", err)
		hm.sendError(client, msgID, 400, "invalid request")
		return
	}
	log.Printf("Parsed request: Cards=%v", req.Cards)

	// 转换牌数据：支持字符串格式 ["S3", "H7"] 或对象格式 [{value: 3, suit: 1}]
	cards, err := parseCards(req.Cards)
	if err != nil {
		log.Printf("ERROR: failed to parse cards: %v, req.Cards=%v", err, req.Cards)
		hm.sendError(client, msgID, 400, "invalid cards format")
		return
	}
	log.Printf("Parsed cards: %v, count=%d", cards, len(cards))

	log.Printf("Looking up room: %s", client.RoomID)
	r, exists := hm.roomMgr.GetRoom(client.RoomID)
	if !exists {
		log.Printf("ERROR: Room not found: %s", client.RoomID)
		hm.sendError(client, msgID, 404, "room not found")
		return
	}
	log.Printf("Room found: %s, player count: %d", r.ID, len(r.Players))

	log.Printf("Getting game state for room: %s", r.ID)
	gsm := r.GetGameState()
	if gsm == nil {
		log.Printf("ERROR: Game state is nil for room: %s", r.ID)
		hm.sendError(client, msgID, 500, "game not started")
		return
	}
	log.Printf("Game state found: %+v", gsm)

	log.Printf("handlePlayCards: client=%s, UID=%s, room=%s, cards=%v, currentTurn=%s",
		client.ID, client.UID, client.RoomID, cards, r.CurrentTurnUID)

	// 用户手动出牌，取消AI托管
	if player, exists := r.GetPlayer(client.UID); exists && !player.IsBot {
		player.IsAIControlled = false
		log.Printf("handlePlayCards: player %s manual play, AI control disabled", client.UID)
	}

	result, gameEnded, err := gsm.PlayCards(client.UID, cards)
	if err != nil {
		log.Printf("handlePlayCards: PlayCards failed for %s: %v (cards=%v, LastPlayedUID=%s, LastPattern=%s)",
			client.UID, err, cards, r.CurrentTurnUID, r.LastPattern)
		hm.sendError(client, msgID, 500, err.Error())
		return
	}

	log.Printf("handlePlayCards: PlayCards success for %s: result=%+v, gameEnded=%v", client.UID, result, gameEnded)

	if result == nil {
		log.Printf("handlePlayCards: player %s passed", client.UID)
		hm.broadcastMsg(client.RoomID, MsgPassNotify, map[string]interface{}{
			"uid": client.UID,
		})
	} else {
		player, _ := r.GetPlayer(client.UID)
		hm.broadcastMsg(client.RoomID, MsgPlayCardsNotify, map[string]interface{}{
			"uid":        client.UID,
			"cards":      cards,
			"pattern":    result.Pattern.String(),
			"card_count": len(player.Cards),
			"is_last":    len(player.Cards) == 0,
		})
	}

	if gameEnded {
		log.Printf("handlePlayCards: Game ended after player %s played last card", client.UID)
		hm.handleGameEnd(r, gsm)
		return
	}

	log.Printf("handlePlayCards: Game continues, currentTurn=%s", r.CurrentTurnUID)

	nextUID := gsm.NextTurnAfterPlay()
	log.Printf("handlePlayCards: next player is %s, starting timer", nextUID)
	if nextUID != "" {
		// 清除 IsLastRound 标记，因为有玩家出了牌，不是连续 PASS 的情况
		r.IsLastRound = false
		
		// 在启动定时器前，确保非机器人玩家的 IsAIControlled 被正确重置
		if player, exists := r.GetPlayer(nextUID); exists && !player.IsBot && player.IsAIControlled {
			log.Printf("handlePlayCards: resetting IsAIControlled to false for player %s before starting timer", nextUID)
			player.IsAIControlled = false
		}
		
		r.StartTimer(15, nextUID)
		hm.broadcastMsg(client.RoomID, MsgTimerNotify, map[string]interface{}{
			"remaining_seconds": 15,
			"current_turn_uid":  nextUID,
		})
	}
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
	if r.CurrentTurnUID == client.UID && r.State == room.StatePlaying {
		r.StartTimer(15, client.UID)
		hm.broadcastMsg(client.RoomID, MsgTimerNotify, map[string]interface{}{
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
	if r.State == room.StatePlaying {
		respData["current_turn_uid"] = r.CurrentTurnUID
		respData["last_played_uid"] = r.LastPlayedUID
		respData["last_played_cards"] = r.LastPlayedCards
	}

	hm.sendMsg(client, MsgReconnectResp, respData)

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

		hm.sendMsg(c, MsgDealCardsNotify, map[string]interface{}{
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

	hm.broadcastMsg(r.ID, MsgTimerNotify, map[string]interface{}{
		"remaining_seconds": 15,
		"current_turn_uid":  firstCaller,
	})

	// 发送"轮到第一位玩家叫地主"的通知（turn=true 表示轮到叫地主）
	hm.broadcastMsg(r.ID, MsgCallLandlordNotify, map[string]interface{}{
		"uid":    firstCaller,
		"action": 0,
		"score":  0,
		"turn":   true, // 标识这是轮到叫地主的通知
	})
}

// handleGameEnd 处理游戏结束
func (hm *HandlerManager) handleGameEnd(r *room.Room, gsm *room.GameStateMachine) {
	r.StopTimer()

	// 重置所有玩家的状态
	r.ResetPlayersState()

	settlement := gsm.CalculateSettlement()

	results := make([]map[string]interface{}, 0)
	for _, ps := range settlement.PlayerResults {
		results = append(results, map[string]interface{}{
			"uid":          ps.UID,
			"is_landlord":  ps.IsLandlord,
			"score_change": ps.ScoreChange,
			"new_elo":      ps.NewELO,
			"new_tier":     ps.NewTier,
			"is_promoted":  ps.IsPromoted,
			"is_demoted":   ps.IsDemoted,
		})
	}

	winnerSide := 0
	if settlement.WinnerSide == room.WinnerSidePeasant {
		winnerSide = 1
	}

	hm.broadcastMsg(r.ID, MsgGameEndNotify, map[string]interface{}{
		"winner_uid":        settlement.WinnerUID,
		"winner_side":       winnerSide,
		"results":           results,
		"base_score":        settlement.BaseScore,
		"multiplier":        settlement.Multiplier,
		"is_spring":         settlement.IsSpring,
		"is_counter_spring": settlement.IsCounterSpring,
	})

	log.Printf("Room %s: game ended. Winner: %s, Spring: %v, CounterSpring: %v, Multiplier: %d",
		r.ID, settlement.WinnerUID, settlement.IsSpring, settlement.IsCounterSpring, settlement.Multiplier)

	// 保存结算数据到数据库
	go hm.saveGameResult(r.ID, settlement, r.PlayerIDs)

	go func() {
		time.Sleep(30 * time.Second)
		hm.roomMgr.RemoveRoom(r.ID)

		for uid := range r.Players {
			c := hm.hub.GetClientByUID(uid)
			if c != nil {
				c.RoomID = ""
			}
		}
	}()
}

// saveGameResult 保存游戏结算结果到数据库
func (hm *HandlerManager) saveGameResult(roomID string, settlement *room.SettlementResult, playerIDs []string) {
	if hm.db == nil {
		log.Printf("saveGameResult: database not configured, skipping")
		return
	}

	ctx := context.Background()

	// 插入游戏记录
	gameResultSQL := `
		INSERT INTO game_records (room_id, players, winner_uid, winner_side, results, 
			base_score, multiplier, is_spring, is_counter_spring)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	playerIDsJSON, _ := json.Marshal(playerIDs)
	resultsJSON, _ := json.Marshal(settlement.PlayerResults)
	_, err := hm.db.ExecContext(ctx, gameResultSQL,
		roomID,
		string(playerIDsJSON),
		settlement.WinnerUID,
		int(settlement.WinnerSide),
		string(resultsJSON),
		settlement.BaseScore,
		settlement.Multiplier,
		settlement.IsSpring,
		settlement.IsCounterSpring,
	)
	if err != nil {
		log.Printf("saveGameResult: insert game_result failed: %v", err)
		return
	}

	log.Printf("saveGameResult: game result saved successfully for room %s", roomID)
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
	hm.sendMsg(client, MsgErrorResponse, map[string]interface{}{
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

	hm.sendMsg(client, MsgMatchStartReq, map[string]interface{}{
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
