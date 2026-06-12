package game

import (
	"time"

	"github.com/zeromicro/go-zero/core/logx"

	"go-zero-ddz/app/game/internal/websocket"
	"go-zero-ddz/pkg/types"
)

// HandleCallLandlord 处理叫地主请求
func (gl *GameLogic) HandleCallLandlord(client *websocket.Client, action int, score int32, msgID uint16, sendError func(*websocket.Client, uint16, int, string), broadcastMsg func(string, uint16, interface{}), sendMsg func(*websocket.Client, uint16, interface{})) {
	logx.Infof("GameLogic.HandleCallLandlord called: client.UID=%s, client.RoomID=%s, action=%d, score=%d", client.UID, client.RoomID, action, score)

	r, exists := gl.roomMgr.GetRoom(client.RoomID)
	if !exists {
		logx.Errorf("Room not found: %s", client.RoomID)
		sendError(client, msgID, 404, "room not found")
		return
	}

	gsm := r.GetGameState()
	if gsm == nil {
		logx.Infof("Game not started in room: %s", client.RoomID)
		sendError(client, msgID, 500, "game not started")
		return
	}

	if err := gsm.CallLandlord(client.UID, action, score); err != nil {
		logx.Errorf("CallLandlord failed: %v", err)
		sendError(client, msgID, 500, err.Error())
		return
	}

	logx.Infof("Broadcasting call landlord result: uid=%s, action=%d, score=%d", client.UID, action, score)
	broadcastMsg(client.RoomID, gl.msgTypes.MsgCallLandlordNotify, map[string]interface{}{
		"uid":    client.UID,
		"action": action,
		"score":  score,
		"round":  gsm.CurrentCallRound(),
	})

	logx.Infof("Room %s: after call, callCount=%d, round=%d, checking AllCalled()",
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

			// 关键检查：只有在叫地主阶段才发送"轮到叫地主"消息
			// 如果叫地主阶段已经结束（状态变为Playing或Settlement），不发送消息
			// 注意：r.State 是公开字段，直接读取可能有竞态，但这是一个优化检查
			// 即使检查通过，真正的消息发送也不会影响游戏逻辑（只是UI提示）
			if r.State != types.StateCalling {
				logx.Infof("Room %s: skipping 'next to call' notification, state=%d (not calling phase)", roomID, r.State)
				return
			}

			// StartTimer 内部会处理 CurrentTurnUID 的设置和计时器启动
			r.StartTimer(15, nextUID)

			logx.Infof("Room %s: next to call is %s (human call path)", roomID, nextUID)
			// 注意：仅广播 {uid, turn:true}，不要带 action/score 字段，
			// 否则前端会把它误判为"X 不叫"。
			broadcastMsg(roomID, gl.msgTypes.MsgCallLandlordNotify, map[string]interface{}{
				"uid":  nextUID,
				"turn": true,
			})
			broadcastMsg(roomID, gl.msgTypes.MsgTimerNotify, map[string]interface{}{
				"remaining_seconds": 15,
				"current_turn_uid":  nextUID,
			})

			// 关键修复：如果下一个是机器人，触发机器人叫地主
			nextPlayer, nextExists := r.GetPlayer(nextUID)
			isNextBot := nextExists && nextPlayer.IsBot
			if isNextBot {
				logx.Infof("Room %s: next player %s is bot, triggering bot call landlord", roomID, nextUID)
				gl.roomMgr.TriggerBotIfNeeded(r)
			}
		})

		return
	}

	if gsm.AllCalled() {
		logx.Infof("Room %s: all players have called, confirming landlord", r.ID)
		if err := gsm.ConfirmLandlord(); err != nil {
			logx.Errorf("Failed to confirm landlord: %v", err)
			return
		}
		logx.Infof("Room %s: landlord confirmed: %s, currentTurn: %s, state: %d", r.ID, r.LandlordUID, r.CurrentTurnUID, r.State)

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

		// 打印底牌内容（调试）
		logx.Infof("Room %s: bottom_cards count=%d, cards=%v", r.ID, len(r.BottomCards), r.BottomCards)

		// 第一步：广播显示底牌（翻开展示）和地主信息
		// 注意：此时不包含 current_turn_uid，避免前端立即显示"轮到出牌"
		broadcastMsg(r.ID, gl.msgTypes.MsgCallLandlordNotify, map[string]interface{}{
			"landlord_uid": r.LandlordUID,
			"players":      playersInfo,
			"bottom_cards": r.BottomCards,
		})

		// 第二步：延迟3秒后开始出牌流程，让用户有时间看到底牌
		roomID := r.ID
		landlordUID := r.LandlordUID
		playersInfoCopy := playersInfo

		time.AfterFunc(3*time.Second, func() {
			r, exists := gl.roomMgr.GetRoom(roomID)
			if !exists {
				return
			}

			// 给地主发送带底牌的手牌
			if landlord, exists := r.GetPlayer(landlordUID); exists {
				landlordClient := gl.hub.GetClientByUID(landlord.UID)
				if landlordClient != nil {
					logx.Infof("Room %s: Sending DEAL_CARDS_NOTIFY to landlord %s (total %d)", roomID, landlord.UID, len(landlord.Cards))
					sendMsg(landlordClient, gl.msgTypes.MsgDealCardsNotify, map[string]interface{}{
						"your_cards":   landlord.Cards,
						"players":      playersInfoCopy,
						"counts":       r.GetCardCounts(),
						"is_landlord":  true,
						"landlord_uid": landlordUID,
					})
				}
			}

			// 给所有玩家广播手牌数量（地主 = 20 张）
			broadcastMsg(r.ID, gl.msgTypes.MsgPlayCardsNotify, map[string]interface{}{
				"uid":          "",
				"cards":        []interface{}{},
				"card_counts":  r.GetCardCounts(),
				"landlord_uid": landlordUID,
				"players":      playersInfoCopy,
			})

			// 开始计时器并广播
			broadcastMsg(r.ID, gl.msgTypes.MsgTimerNotify, map[string]interface{}{
				"remaining_seconds": 15,
				"current_turn_uid":  landlordUID,
			})

			// 如果地主是机器人，延迟触发AI出牌
			landlord, landlordExists := r.GetPlayer(landlordUID)
			if landlordExists && landlord.IsBot {
				logx.Infof("Room %s: landlord %s is bot, triggering bot play after 1s", roomID, landlordUID)
				time.AfterFunc(1*time.Second, func() {
					r, exists := gl.roomMgr.GetRoom(roomID)
					if !exists {
						return
					}
					gl.roomMgr.TriggerBotIfNeeded(r)
				})
			}
		})

		return
	}

	// 还没叫完，继续下一个玩家
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

		// 关键检查：只有在叫地主阶段才发送"轮到叫地主"消息
		if r.State != types.StateCalling {
			logx.Infof("Room %s: skipping 'next to call' notification, state=%d (not calling phase)", roomID, r.State)
			return
		}

		// StartTimer 内部会处理 CurrentTurnUID 的设置和计时器启动
		r.StartTimer(15, nextUID)

		logx.Infof("Room %s: next to call is %s", roomID, nextUID)
		broadcastMsg(roomID, gl.msgTypes.MsgCallLandlordNotify, map[string]interface{}{
			"uid":  nextUID,
			"turn": true,
		})
		broadcastMsg(roomID, gl.msgTypes.MsgTimerNotify, map[string]interface{}{
			"remaining_seconds": 15,
			"current_turn_uid":  nextUID,
		})

		// 如果下一个是 Bot，延迟触发 AI
		if nextPlayer, exists := r.GetPlayer(nextUID); exists && (nextPlayer.IsBot || nextPlayer.IsAIControlled) {
			time.AfterFunc(1500*time.Millisecond, func() {
				gl.roomMgr.TriggerBotIfNeeded(r)
			})
		}
	})
}
