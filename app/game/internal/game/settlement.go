package game

import (
	"context"
	"encoding/json"
	"time"

	"github.com/zeromicro/go-zero/core/logx"

	"go-zero-ddz/app/game/internal/room"
	"go-zero-ddz/pkg/types"
)

// HandleGameEnd 处理游戏结束
func (gl *GameLogic) HandleGameEnd(r *room.Room, gsm *room.GameStateMachine, broadcastMsg func(string, uint16, interface{})) {
	r.StopTimer()

	// 先计算结算结果（需要保留玩家的 IsLandlord 状态）
	settlement := gsm.CalculateSettlement()

	// 重置所有玩家的状态
	r.ResetPlayersState()

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

	logx.Infof("Room %s: game ended. Winner: %s, Spring: %v, CounterSpring: %v, Multiplier: %d",
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
		logx.Errorf("saveGameResult: database not configured, skipping")
		return
	}

	ctx := context.Background()

	// 获取房间的打牌轮次记录
	var playRoundsJSON string
	if gl.roomMgr != nil {
		if room, exists := gl.roomMgr.GetRoom(roomID); exists && room != nil {
			roundsJSON, _ := json.Marshal(room.PlayRounds)
			playRoundsJSON = string(roundsJSON)
			logx.Infof("saveGameResult: play rounds recorded for room %s: %d rounds", roomID, len(room.PlayRounds))
		}
	}

	// 插入游戏记录
	gameResultSQL := `
		INSERT INTO game_records (room_id, players, winner_uid, winner_side, results, 
			play_rounds, base_score, multiplier, is_spring, is_counter_spring, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NOW())
	`
	playerIDsJSON, _ := json.Marshal(playerIDs)
	resultsJSON, _ := json.Marshal(settlement.PlayerResults)
	_, err := gl.db.ExecContext(ctx, gameResultSQL,
		roomID,
		string(playerIDsJSON),
		settlement.WinnerUID,
		int(settlement.WinnerSide),
		string(resultsJSON),
		playRoundsJSON,
		settlement.BaseScore,
		settlement.Multiplier,
		settlement.IsSpring,
		settlement.IsCounterSpring,
	)
	if err != nil {
		logx.Errorf("saveGameResult: insert game_result failed: %v", err)
	} else {
		logx.Infof("saveGameResult: game result saved successfully for room %s", roomID)
	}

	// 更新玩家信息（只更新真人玩家）
	updatePlayerSQL := `
		UPDATE users SET elo = ?, tier = ?, gold = gold + ?, 
			wins = wins + ?, losses = losses + ?
		WHERE uid = ?
	`
	for uid, result := range settlement.PlayerResults {
		if result.IsBot {
			continue // 跳过机器人
		}

		isWinner := uid == settlement.WinnerUID
		_, err := gl.db.ExecContext(ctx, updatePlayerSQL,
			result.NewELO,
			result.NewTier,
			result.ScoreChange,
			func() int {
				if isWinner {
					return 1
				}
				return 0
			}(),
			func() int {
				if isWinner {
					return 0
				}
				return 1
			}(),
			uid,
		)
		if err != nil {
			logx.Errorf("saveGameResult: update player %s failed: %v", uid, err)
		} else {
			logx.Infof("saveGameResult: player %s updated: elo=%d, tier=%s, score_change=%d",
				uid, result.NewELO, result.NewTier, result.ScoreChange)
		}
	}

	logx.Infof("saveGameResult: all player updates completed for room %s", roomID)
}
