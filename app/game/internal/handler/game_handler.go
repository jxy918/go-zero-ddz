package handler

import (
	"encoding/json"
	"fmt"

	"go-zero-ddz/app/game/internal/room"
	"go-zero-ddz/app/game/internal/websocket"
	"go-zero-ddz/pkg/cardutil"
	"go-zero-ddz/pkg/types"

	"github.com/zeromicro/go-zero/core/logx"
)

// handleCallLandlord 处理叫地主
func (hm *HandlerManager) handleCallLandlord(client *websocket.Client, msgID uint16, payload []byte) {
	logx.Infof("=== handleCallLandlord START (delegated to gameLogic) ===")

	type CallReq struct {
		Action int   `json:"action"`
		Score  int32 `json:"score"`
	}
	var req CallReq
	if err := json.Unmarshal(payload, &req); err != nil {
		logx.Errorf("Failed to unmarshal call landlord request: %v", err)
		hm.sendError(client, msgID, 400, "invalid request")
		return
	}

	hm.gameLogic.HandleCallLandlord(client, req.Action, req.Score, msgID, hm.sendError, hm.broadcastMsg, hm.sendMsg)
}

// handlePlayCards 处理出牌
func (hm *HandlerManager) handlePlayCards(client *websocket.Client, msgID uint16, payload []byte) {
	logx.Infof("=== handlePlayCards START (delegated to gameLogic) ===")

	type PlayReq struct {
		Cards interface{} `json:"cards"`
	}
	var req PlayReq
	if err := json.Unmarshal(payload, &req); err != nil {
		logx.Errorf("ERROR: failed to unmarshal request: %v", err)
		hm.sendError(client, msgID, 400, "invalid request")
		return
	}

	cards, err := parseCards(req.Cards)
	if err != nil {
		logx.Errorf("ERROR: failed to parse cards: %v", err)
		hm.sendError(client, msgID, 400, "invalid cards format")
		return
	}

	hm.gameLogic.HandlePlayCards(client, cards, msgID, hm.sendError, hm.broadcastMsg)
}

// handleCancelAIControl 处理取消AI托管请求
func (hm *HandlerManager) handleCancelAIControl(client *websocket.Client, msgID uint16, payload []byte) {
	logx.Infof("=== handleCancelAIControl START ===")
	logx.Infof("client=%s, UID=%s, RoomID=%s", client.ID, client.UID, client.RoomID)

	r, exists := hm.roomMgr.GetRoom(client.RoomID)
	if !exists || r == nil {
		logx.Infof("handleCancelAIControl: room not found")
		return
	}

	player, exists := r.GetPlayer(client.UID)
	if !exists {
		logx.Infof("handleCancelAIControl: player not found")
		return
	}

	if player.IsBot {
		logx.Infof("handleCancelAIControl: cannot cancel AI control for bot player")
		return
	}

	player.IsAIControlled = false
	player.GraceWarningSent = false
	logx.Infof("handleCancelAIControl: player %s AI control disabled", client.UID)

	if r.CurrentTurnUID == client.UID && r.State == types.StatePlaying {
		r.StartTimer(15, client.UID)
		hm.broadcastMsg(client.RoomID, types.MsgTimerNotify, map[string]interface{}{
			"remaining_seconds": 15,
			"current_turn_uid":  client.UID,
		})
		logx.Infof("handleCancelAIControl: started timer for player %s", client.UID)
	}
}

// startGame 开始游戏
func (hm *HandlerManager) startGame(r *room.Room) {
	gsm := r.GetGameState()
	if gsm == nil {
		logx.Infof("Room %s: game state not initialized", r.ID)
		return
	}
	hm.startGameWithState(r, gsm)
}

// startGameWithState 使用已初始化的游戏状态开始游戏（避免重复获取锁）
func (hm *HandlerManager) startGameWithState(r *room.Room, gsm *room.GameStateMachine) {
	logx.Infof("Room %s: starting game with state", r.ID)

	for _, uid := range r.PlayerIDs {
		if player, exists := r.GetPlayer(uid); exists {
			player.IsAIControlled = false
			player.GraceWarningSent = false
		}
	}

	hands, _, err := gsm.DealCards()
	if err != nil {
		logx.Errorf("Failed to deal cards: %v", err)
		return
	}

	// 构造所有玩家的手牌数量
	cardCounts := make(map[string]int)
	for i, uid := range r.PlayerIDs {
		cardCounts[uid] = len(hands[i])
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
			"landlord_uid":       "",
			"counts":             cardCounts, // 添加所有玩家的手牌数量
		})
	}

	firstCaller := r.PlayerIDs[0]
	logx.Infof("startGameWithState: firstCaller=%s, PlayerIDs=%v", firstCaller, r.PlayerIDs)
	r.StartTimer(15, firstCaller)

	hm.broadcastMsg(r.ID, types.MsgTimerNotify, map[string]interface{}{
		"remaining_seconds": 15,
		"current_turn_uid":  firstCaller,
	})

	hm.broadcastMsg(r.ID, types.MsgCallLandlordNotify, map[string]interface{}{
		"uid":    firstCaller,
		"action": 0,
		"score":  0,
		"turn":   true,
	})
}

// parseCards 解析牌数据：支持字符串格式 ["S3", "H7"] 和对象格式 [{value: 3, suit: 1}]
func parseCards(data interface{}) ([]cardutil.Card, error) {
	if data == nil {
		return []cardutil.Card{}, nil
	}

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
				card, err := parseCardObject(obj)
				if err != nil {
					return nil, err
				}
				cards = append(cards, card)
			}
		}
		return cards, nil
	}

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
