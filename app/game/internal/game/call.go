package game

import (
	"log"
	"time"

	"go-zero-ddz/app/game/internal/websocket"
)

// HandleCallLandlord 处理叫地主请求
func (gl *GameLogic) HandleCallLandlord(client *websocket.Client, action int, score int32, msgID uint16, sendError func(*websocket.Client, uint16, int, string), broadcastMsg func(string, uint16, interface{}), sendMsg func(*websocket.Client, uint16, interface{})) {
	log.Printf("GameLogic.HandleCallLandlord called: client.UID=%s, client.RoomID=%s, action=%d, score=%d", client.UID, client.RoomID, action, score)

	r, exists := gl.roomMgr.GetRoom(client.RoomID)
	if !exists {
		log.Printf("Room not found: %s", client.RoomID)
		sendError(client, msgID, 404, "room not found")
		return
	}

	gsm := r.GetGameState()
	if gsm == nil {
		log.Printf("Game not started in room: %s", client.RoomID)
		sendError(client, msgID, 500, "game not started")
		return
	}

	if err := gsm.CallLandlord(client.UID, action, score); err != nil {
		log.Printf("CallLandlord failed: %v", err)
		sendError(client, msgID, 500, err.Error())
		return
	}

	log.Printf("Broadcasting call landlord result: uid=%s, action=%d, score=%d", client.UID, action, score)
	broadcastMsg(client.RoomID, gl.msgTypes.MsgCallLandlordNotify, map[string]interface{}{
		"uid":    client.UID,
		"action": action,
		"score":  score,
		"round":  gsm.CurrentCallRound(),
	})

	log.Printf("Room %s: after call, callCount=%d, round=%d, checking AllCalled()",
		client.RoomID, gsm.CallCount(), gsm.CurrentCallRound())

	// 如果还没叫完，延迟1秒后广播轮到下一个玩家叫地主
	if !gsm.AllCalled() {
		nextIdx := gsm.CurrentCallIdx()
		if nextIdx < 0 || nextIdx >= len(r.PlayerIDs) {
			nextIdx = 0
		}
		nextUID := r.PlayerIDs[nextIdx]

		roomID := client.RoomID

		time.AfterFunc(time.Second*1, func() {
			r, exists := gl.roomMgr.GetRoom(roomID)
			if !exists {
				return
			}
			r.CurrentTurnUID = nextUID
			r.StartTimer(15, nextUID)

			log.Printf("Room %s: next to call is %s", roomID, nextUID)
			broadcastMsg(roomID, gl.msgTypes.MsgCallLandlordNotify, map[string]interface{}{
				"uid":  nextUID,
				"turn": true,
			})
			broadcastMsg(roomID, gl.msgTypes.MsgTimerNotify, map[string]interface{}{
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
		broadcastMsg(r.ID, gl.msgTypes.MsgCallLandlordNotify, map[string]interface{}{
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

		time.AfterFunc(5*time.Second, func() {
			log.Printf("Room %s: 5 seconds passed, now sending bottom cards to landlord and starting game", roomID)

			// 获取房间
			r, exists := gl.roomMgr.GetRoom(roomID)
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
				landlordClient := gl.hub.GetClientByUID(landlord.UID)
				if landlordClient != nil {
					log.Printf("Room %s: Sending DEAL_CARDS_NOTIFY to landlord %s", roomID, landlord.UID)
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
					sendMsg(landlordClient, gl.msgTypes.MsgDealCardsNotify, map[string]interface{}{
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
			broadcastMsg(r.ID, gl.msgTypes.MsgPlayCardsNotify, map[string]interface{}{
				"uid":          "",
				"cards":        []interface{}{},
				"card_counts":  r.GetCardCounts(),
				"landlord_uid": landlordUID,
				"players":      playersInfo,
			})

			r.StartTimer(15, landlordUID)
			broadcastMsg(r.ID, gl.msgTypes.MsgTimerNotify, map[string]interface{}{
				"remaining_seconds": 15,
				"current_turn_uid":  landlordUID,
			})
			// 如果地主是 Bot，立即触发 AI
			log.Printf("Room %s: triggering bot if needed after landlord confirmed", roomID)
			gl.roomMgr.TriggerBotIfNeeded(r)
		})
	} else {
		nextIdx := gsm.CurrentCallIdx()
		if nextIdx < 0 || nextIdx >= len(r.PlayerIDs) {
			nextIdx = 0
		}
		nextUID := r.PlayerIDs[nextIdx]
		r.StartTimer(15, nextUID)
		broadcastMsg(r.ID, gl.msgTypes.MsgTimerNotify, map[string]interface{}{
			"remaining_seconds": 15,
			"current_turn_uid":  nextUID,
		})
		// 通知客户端轮到叫地主
		broadcastMsg(r.ID, gl.msgTypes.MsgCallLandlordNotify, map[string]interface{}{
			"uid":    nextUID,
			"action": 0,
			"score":  0,
			"turn":   true,
		})
		// 如果下一个是 Bot，延迟触发 AI
		if nextPlayer, exists := r.GetPlayer(nextUID); exists && (nextPlayer.IsBot || nextPlayer.IsAIControlled) {
			time.AfterFunc(1500*time.Millisecond, func() {
				gl.roomMgr.TriggerBotIfNeeded(r)
			})
		}
	}
}
