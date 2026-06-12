package room

import (
	"fmt"
	"log"
	"time"

	"go-zero-ddz/pkg/cardutil"
	"go-zero-ddz/pkg/types"
)

// GameStateMachine 游戏状态机
type GameStateMachine struct {
	room           *Room
	callRecords    []types.CallRecord
	callCount      int    // 已叫人数
	currentCallIdx int    // 当前叫地主的玩家索引
	callRound      int    // 当前叫地主轮次（1-3轮）
	passCount      int    // 连续pass人数
	lastCallerUID  string // 最后一个叫地主的玩家UID（用于在ConfirmLandlord时确定地主）

	// LandlordReady 当 ConfirmLandlord 的 5 秒延迟结束后关闭，
	// 用于通知其他 goroutine（如 manager.go 中的发牌逻辑）底牌已发放给地主。
	LandlordReady chan struct{}
}

// NewGameStateMachine 创建游戏状态机
func NewGameStateMachine(room *Room) *GameStateMachine {
	return &GameStateMachine{
		room:        room,
		callRecords: make([]types.CallRecord, 0, 3),
	}
}

// DealCards 发牌
func (gsm *GameStateMachine) DealCards() ([][]cardutil.Card, []cardutil.Card, error) {
	if gsm.room.Count() != 3 {
		return nil, nil, fmt.Errorf("need 3 players to deal cards")
	}

	// 创建并洗牌
	deck := cardutil.NewFullDeck()
	hands, bottomCards := cardutil.DealCards(deck)

	// 分配手牌
	for i, uid := range gsm.room.PlayerIDs {
		gsm.room.mu.Lock()
		if _, exists := gsm.room.Players[uid]; exists {
			gsm.room.Players[uid].Cards = cardutil.SortCards(hands[i])
		}
		gsm.room.mu.Unlock()
	}

	gsm.room.mu.Lock()
	gsm.room.BottomCards = bottomCards
	gsm.room.mu.Unlock()
	gsm.room.SetState(types.StateCalling)

	// 初始化叫地主索引，从第一个玩家开始
	gsm.currentCallIdx = 0
	log.Printf("Room %s: cards dealt, starting caller index: %d", gsm.room.ID, gsm.currentCallIdx)
	return hands, bottomCards, nil
}

// CallLandlord 叫地主
func (gsm *GameStateMachine) CallLandlord(uid string, action int, score int32) error {
	gsm.room.mu.Lock()
	defer gsm.room.mu.Unlock()

	_, exists := gsm.room.Players[uid]
	if !exists {
		return fmt.Errorf("player not found")
	}

	if gsm.room.State != types.StateCalling {
		return fmt.Errorf("not in calling state")
	}

	// 关键修复：玩家叫地主后立即清除定时器，避免倒计时继续
	gsm.room.StopTimer()

	// 记录叫分
	record := types.CallRecord{UID: uid, Action: action, Score: score}
	gsm.callRecords = append(gsm.callRecords, record)
	gsm.callCount++

	// 更新最高分
	if action == 1 && score > gsm.room.CallScore {
		gsm.room.CallScore = score
		// 不要立即设置 LandlordUID！只记录叫分者
		// 只有在 ConfirmLandlord() 时才最终确定地主
		gsm.lastCallerUID = uid
		// 每当有人叫出更高分时，重置 passCount = 0
		// 因为需要重新累计其他玩家的pass来确定地主
		// 例如：玩家1叫1分→玩家2pass→玩家3叫2分→此时passCount应该重置为0
		// 然后从玩家1开始继续叫，直到其他2人都pass才确定地主
		gsm.passCount = 0
		log.Printf("Room %s: player %s called landlord with score %d, CallScore=%d, passCount reset to 0", gsm.room.ID, uid, score, gsm.room.CallScore)
	} else {
		gsm.passCount++
		log.Printf("Room %s: player %s passed (passCount=%d)", gsm.room.ID, uid, gsm.passCount)
	}

	// 更新当前叫地主索引（逆时针顺序）
	gsm.currentCallIdx = (gsm.currentCallIdx - 1 + 3) % 3
	if gsm.currentCallIdx == 0 {
		gsm.callRound++
		log.Printf("Room %s: call round %d completed", gsm.room.ID, gsm.callRound)
	}

	log.Printf("Room %s: player %s called, action=%d, score=%d (total calls: %d, round=%d, passCount=%d)",
		gsm.room.ID, uid, action, score, gsm.callCount, gsm.callRound, gsm.passCount)

	return nil
}

// AllCalled 是否所有人都叫过了（3轮或连续3次pass或叫地主后其他人都pass）
func (gsm *GameStateMachine) AllCalled() bool {
	// 3轮叫地主完成
	if gsm.callRound >= 3 {
		return true
	}
	// 连续3人pass（没人叫地主）
	if gsm.passCount >= 3 {
		return true
	}
	// 如果有人叫了地主，且其他两个玩家都已经pass
	// 叫地主后，剩下两个玩家都pass = 地主确定
	// 使用 lastCallerUID 而不是 LandlordUID，因为 LandlordUID 只在 ConfirmLandlord 时设置
	if gsm.lastCallerUID != "" && gsm.passCount >= 2 {
		log.Printf("Room %s: AllCalled=true - LandlordUID=%s, passCount=%d", gsm.room.ID, gsm.room.LandlordUID, gsm.passCount)
		return true
	}
	return false
}

// CallCount 已叫人数
func (gsm *GameStateMachine) CallCount() int {
	return gsm.callCount
}

// CurrentCallIdx 当前叫地主的玩家索引
func (gsm *GameStateMachine) CurrentCallIdx() int {
	return gsm.currentCallIdx
}

// CurrentCallRound 当前叫地主轮次
func (gsm *GameStateMachine) CurrentCallRound() int {
	return gsm.callRound
}

// PassCount 连续pass人数
func (gsm *GameStateMachine) PassCount() int {
	return gsm.passCount
}

// ConfirmLandlord 确认地主（所有人叫完后调用）
func (gsm *GameStateMachine) ConfirmLandlord() error {
	log.Printf("ConfirmLandlord called for room %s", gsm.room.ID)
	gsm.room.mu.Lock()

	log.Printf("Room %s: before confirm, lastCallerUID=%s, call_score=%d", gsm.room.ID, gsm.lastCallerUID, gsm.room.CallScore)

	if gsm.room.CallScore == 0 || gsm.lastCallerUID == "" {
		// 没人叫地主，随机选一个玩家当地主
		randIdx := int(time.Now().UnixNano()/1e9) % len(gsm.room.PlayerIDs)
		gsm.room.LandlordUID = gsm.room.PlayerIDs[randIdx]
		gsm.room.BaseScore = 1
		log.Printf("Room %s: no landlord called, randomly selecting %s as landlord", gsm.room.ID, gsm.room.LandlordUID)
	} else {
		// 有人叫地主，使用最后一个叫地主的玩家
		gsm.room.LandlordUID = gsm.lastCallerUID
		log.Printf("Room %s: landlord confirmed: %s with score %d", gsm.room.ID, gsm.room.LandlordUID, gsm.room.CallScore)
	}

	// 重置所有非机器人玩家的AI托管状态
	for uid := range gsm.room.Players {
		if player, exists := gsm.room.Players[uid]; exists && !player.IsBot {
			player.IsAIControlled = false
			log.Printf("Room %s: ConfirmLandlord reset player %s IsAIControlled to false", gsm.room.ID, uid)
		}
	}

	// 设置地主
	landlord, exists := gsm.room.Players[gsm.room.LandlordUID]
	if !exists {
		log.Printf("Room %s: landlord not found: %s", gsm.room.ID, gsm.room.LandlordUID)
		gsm.room.mu.Unlock()
		return fmt.Errorf("landlord not found")
	}
	landlord.IsLandlord = true
	landlord.Role = types.RoleLandlord
	log.Printf("Room %s: set landlord: %s, is_bot=%v", gsm.room.ID, gsm.room.LandlordUID, landlord.IsBot)

	// 设置农民
	for _, uid := range gsm.room.PlayerIDs {
		if uid != gsm.room.LandlordUID {
			if p, exists := gsm.room.Players[uid]; exists {
				p.Role = types.RolePeasant
			}
		}
	}

	// 给地主发放底牌
	landlord.Cards = append(landlord.Cards, gsm.room.BottomCards...)
	landlord.Cards = cardutil.SortCards(landlord.Cards)
	log.Printf("Room %s: landlord %s received bottom cards, total cards: %d", gsm.room.ID, gsm.room.LandlordUID, len(landlord.Cards))

	// 设置第一个出牌的玩家（地主）
	gsm.room.CurrentTurnUID = gsm.room.LandlordUID
	// 保存旧状态
	oldState := gsm.room.State
	// 现在切换到出牌状态
	gsm.room.State = types.StatePlaying
	gsm.room.mu.Unlock()

	// 停止之前的计时器
	gsm.room.StopTimer()

	// 初始化 LandlordReady 信号 channel，用于通知 manager.go 等候的 goroutine
	gsm.LandlordReady = make(chan struct{})
	// 立即关闭信号 channel，通知 manager.go 底牌已发放、地主手牌已就绪
	close(gsm.LandlordReady)

	log.Printf("Room %s: landlord confirmed: %s, base_score=%d, state transitioned to playing",
		gsm.room.ID, gsm.room.LandlordUID, gsm.room.BaseScore)

	// 调用状态变更回调
	if gsm.room.OnStateChange != nil {
		gsm.room.OnStateChange(gsm.room, oldState, types.StatePlaying)
	}

	return nil
}

// PlayCards 出牌，返回：出牌结果，游戏是否结束，错误
func (gsm *GameStateMachine) PlayCards(uid string, cards []cardutil.Card) (*cardutil.PlayResult, bool, error) {
	gsm.room.mu.Lock()
	defer gsm.room.mu.Unlock()

	if gsm.room.State != types.StatePlaying {
		return nil, false, fmt.Errorf("not in playing state")
	}

	if gsm.room.CurrentTurnUID != uid {
		return nil, false, fmt.Errorf("not your turn")
	}

	// Pass（不出牌）
	if len(cards) == 0 {
		// 如果是自由出牌（没人出牌或上家都 Pass），不能 Pass
		if gsm.room.LastPlayedUID == "" || gsm.room.PassCount >= 2 {
			return nil, false, fmt.Errorf("cannot pass when it's a free turn")
		}

		gsm.room.PassCount++
		lastPlayedUID := gsm.room.LastPlayedUID

		// 如果连续两个 Pass，轮回到最后出牌的人
		if gsm.room.PassCount >= 2 {
			gsm.room.PassCount = 0
			gsm.room.CurrentTurnUID = lastPlayedUID
			gsm.room.LastPlayedUID = "" // 重置上一手出牌记录，表示新轮次
			gsm.room.LastPlayedCards = nil
			gsm.room.LastPattern = cardutil.PatternUnknown
			// 设置一个标记，表示连续 PASS，需要特殊处理
			gsm.room.IsLastRound = true
			log.Printf("Room %s: two consecutive passes, returning turn to %s", gsm.room.ID, lastPlayedUID)
		} else {
			// 推进到下一个回合（已持有锁，使用 Locked 版本）
			gsm.NextTurnAfterPlayLocked()
		}

		log.Printf("Room %s: player %s passed (passCount=%d)", gsm.room.ID, uid, gsm.room.PassCount)
		return nil, false, nil
	}

	// 校验出牌
	result := cardutil.AnalyzePattern(cards)
	if !result.Valid {
		return nil, false, fmt.Errorf("invalid card pattern")
	}

	// 如果是自由出牌，直接通过
	if gsm.room.LastPlayedUID == "" || gsm.room.PassCount >= 2 {
		gsm.room.PassCount = 0
	} else {
		// 需要大过上家：使用 AnalyzePattern 从已出牌中解析出真正的牌型主值
		// 关键修复：不能直接用 getMainValue(max)，三带一/顺子等牌型的主值
		// 不等于最大牌面值（例如三带一 5553 的主值是 5 不是 3 的最大值）
		lastAnalyzed := cardutil.AnalyzePattern(gsm.room.LastPlayedCards)
		lastPlay := cardutil.PlayResult{
			Valid:   true,
			Pattern: gsm.room.LastPattern,
			Main:    lastAnalyzed.Main,
			Length:  len(gsm.room.LastPlayedCards),
		}

		if !cardutil.CanBeat(result, lastPlay) {
			return nil, false, fmt.Errorf("cards cannot beat last play")
		}
	}

	// 从手牌中移除出的牌
	player, exists := gsm.room.Players[uid]
	if !exists {
		return nil, false, fmt.Errorf("player not found")
	}

	player.Cards = removeCards(player.Cards, cards)

	// 更新出牌记录
	gsm.room.LastPlayedCards = cards
	gsm.room.LastPlayedUID = uid
	gsm.room.LastPattern = result.Pattern
	gsm.room.PassCount = 0
	gsm.room.IsLastRound = false // 清除连续PASS标记，因为有玩家出了牌

	// 添加打牌轮次记录
	cardValues := make([]int, len(cards))
	for i, c := range cards {
		cardValues[i] = int(c.Value)
	}
	gsm.room.PlayRounds = append(gsm.room.PlayRounds, types.PlayRoundRecord{
		RoundIndex: len(gsm.room.PlayRounds),
		PlayerUID:  uid,
		Action:     types.PlayActionPlay,
		Cards:      cardValues,
		Pattern:    result.Pattern.String(),
		Timestamp:  time.Now().Unix(),
	})

	// 检查炸弹，增加倍数
	if result.Pattern.IsBomb() {
		gsm.room.Multiplier *= 2
	}

	// 检查是否出完（胜利）
	if len(player.Cards) == 0 {
		// 使用 setStateLocked 避免死锁（已持有 room.mu.Lock）
		gsm.room.setStateLocked(types.StateSettlement)
		gsm.room.StopTimer()
		log.Printf("Room %s: player %s wins! (played %d cards, pattern=%s)", gsm.room.ID, uid, len(cards), result.Pattern)
		return &result, true, nil
	}

	log.Printf("Room %s: player %s played %v, remaining cards: %d", gsm.room.ID, uid, cardutil.CardsToString(cards), len(player.Cards))

	// 推进到下一个玩家（已持有锁，使用 Locked 版本）
	gsm.NextTurnAfterPlayLocked()
	// 停止当前计时器，等待下一个玩家启动新计时器
	gsm.room.StopTimer()

	return &result, false, nil
}

// NextTurnAfterPlay 出牌后推进到下一个回合
// 注意：调用此方法前必须确保没有持有 room.mu 锁，否则会造成死锁
func (gsm *GameStateMachine) NextTurnAfterPlay() string {
	gsm.room.mu.Lock()
	currentUID := gsm.room.CurrentTurnUID
	nextUID := gsm.room.NextTurn(currentUID)
	gsm.room.CurrentTurnUID = nextUID
	gsm.room.mu.Unlock()

	return nextUID
}

// NextTurnAfterPlayLocked 出牌后推进到下一个回合（已持有锁版本）
// 注意：调用此方法前必须已经持有 room.mu 锁
func (gsm *GameStateMachine) NextTurnAfterPlayLocked() string {
	currentUID := gsm.room.CurrentTurnUID

	// 直接计算下一个玩家（已持有锁，不需要再获取锁）
	for i, uid := range gsm.room.PlayerIDs {
		if uid == currentUID {
			// 逆时针方向：i-1，处理边界情况
			nextIdx := (i - 1 + len(gsm.room.PlayerIDs)) % len(gsm.room.PlayerIDs)
			nextUID := gsm.room.PlayerIDs[nextIdx]
			gsm.room.CurrentTurnUID = nextUID
			log.Printf("Room %s: turn changed from %s to %s", gsm.room.ID, currentUID, nextUID)
			return nextUID
		}
	}
	return ""
}

// CalculateSettlement 计算结算结果
func (gsm *GameStateMachine) CalculateSettlement() *types.SettlementResult {
	gsm.room.mu.RLock()
	defer gsm.room.mu.RUnlock()

	result := &types.SettlementResult{
		PlayerResults: make(map[string]*types.PlayerSettlement),
		BaseScore:     gsm.room.BaseScore,
		Multiplier:    gsm.room.Multiplier,
	}

	// 找出赢家
	var winnerUID string
	for uid, p := range gsm.room.Players {
		if len(p.Cards) == 0 {
			winnerUID = uid
			break
		}
	}
	result.WinnerUID = winnerUID

	// 判断赢家阵营
	winner, exists := gsm.room.Players[winnerUID]
	if !exists {
		return result
	}

	if winner.IsLandlord {
		result.WinnerSide = types.WinnerSideLandlord
	} else {
		result.WinnerSide = types.WinnerSidePeasant
	}

	// 春天/反春判定
	result.IsSpring, result.IsCounterSpring = gsm.checkSpring(winnerUID)

	// 春天/反春加倍
	if result.IsSpring || result.IsCounterSpring {
		result.Multiplier *= 2
	}

	// 计算每个玩家的积分变化
	totalScore := result.BaseScore * result.Multiplier

	for uid, p := range gsm.room.Players {
		ps := &types.PlayerSettlement{
			UID:        uid,
			IsLandlord: p.IsLandlord,
			IsBot:      p.IsBot,
		}

		if result.WinnerSide == types.WinnerSideLandlord {
			// 地主赢
			if p.IsLandlord {
				ps.ScoreChange = totalScore * 2 // 地主赢双倍
			} else {
				ps.ScoreChange = -totalScore
			}
		} else {
			// 农民赢
			if p.IsLandlord {
				ps.ScoreChange = -totalScore * 2
			} else {
				ps.ScoreChange = totalScore
			}
		}

		result.PlayerResults[uid] = ps
	}

	return result
}

// checkSpring 检查春天/反春
func (gsm *GameStateMachine) checkSpring(winnerUID string) (bool, bool) {
	winner, _ := gsm.room.Players[winnerUID]

	if winner.IsLandlord {
		// 地主春天：农民一张牌都没出过
		for uid, p := range gsm.room.Players {
			if uid != winnerUID && !p.IsLandlord {
				// 如果农民初始17张，结束还是17张 → 春天
				if len(p.Cards) == 17 {
					return true, false
				}
			}
		}
	} else {
		// 农民反春：地主只出了一次牌（出完底牌后出的第一手）
		landlord, _ := gsm.room.Players[gsm.room.LandlordUID]
		// 地主初始20张(17+3底牌)，如果剩余 >= 17 → 反春
		if len(landlord.Cards) >= 17 {
			return false, true
		}
	}

	return false, false
}

// getMainValue 获取牌组的主牌值
func getMainValue(cards []cardutil.Card) cardutil.CardValue {
	if len(cards) == 0 {
		return cardutil.CardValueUnknown
	}
	maxVal := cards[0].Value
	for _, c := range cards {
		if c.Value > maxVal {
			maxVal = c.Value
		}
	}
	return maxVal
}

// removeCards 从手牌中移除指定的牌
func removeCards(hand []cardutil.Card, toRemove []cardutil.Card) []cardutil.Card {
	removeMap := make(map[cardutil.CardValue]int)
	for _, c := range toRemove {
		removeMap[c.Value]++
	}

	result := make([]cardutil.Card, 0, len(hand))
	for _, c := range hand {
		if count, exists := removeMap[c.Value]; exists && count > 0 {
			removeMap[c.Value]--
			continue
		}
		result = append(result, c)
	}

	return result
}
