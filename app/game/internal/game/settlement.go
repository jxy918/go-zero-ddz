package game

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"go-zero-ddz/app/game/internal/room"
	"go-zero-ddz/pkg/types"
)

// HandleGameEnd 处理游戏结束
func (gl *GameLogic) HandleGameEnd(r *room.Room, gsm *room.GameStateMachine, broadcastMsg func(string, uint16, interface{})) {
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
	if settlement.WinnerSide == types.WinnerSidePeasant {
		winnerSide = 1
	}

	broadcastMsg(r.ID, gl.msgTypes.MsgGameEndNotify, map[string]interface{}{
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
	go gl.saveGameResult(r.ID, settlement, r.PlayerIDs)

	go func() {
		time.Sleep(30 * time.Second)
		gl.roomMgr.RemoveRoom(r.ID)

		for uid := range r.Players {
			c := gl.hub.GetClientByUID(uid)
			if c != nil {
				c.RoomID = ""
			}
		}
	}()
}

// saveGameResult 保存游戏结算结果到数据库
func (gl *GameLogic) saveGameResult(roomID string, settlement *types.SettlementResult, playerIDs []string) {
	if gl.db == nil {
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
	_, err := gl.db.ExecContext(ctx, gameResultSQL,
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
