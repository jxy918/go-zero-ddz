package ai

import (
	"math/rand"
	"time"

	"go-zero-ddz/pkg/cardutil"
	"go-zero-ddz/pkg/types"
)

// PlayAction 出牌动作
type PlayAction int

const (
	ActionPass PlayAction = iota
	ActionPlay
)

// PlayDecision AI 出牌决策
type PlayDecision struct {
	Action PlayAction
	Cards  []cardutil.Card
}

// AIContext AI 决策上下文
type AIContext struct {
	MyCards       []cardutil.Card
	LastPlay      *LastPlayInfo
	LastPlayerUID string
	MyRole        types.PlayerRole
	Players       map[string]*PlayerInfo
	CardCounter   *CardCounter
	Difficulty    string // easy | normal | hard
}

// LastPlayInfo 上一手出牌信息
type LastPlayInfo struct {
	Cards   []cardutil.Card
	Pattern cardutil.CardPattern
	Main    cardutil.CardValue
	UID     string
}

// PlayerInfo 玩家信息
type PlayerInfo struct {
	UID        string
	IsLandlord bool
	CardCount  int
}

// AIEngine AI 决策引擎
type AIEngine struct {
	config *AIConfig
	rng    *rand.Rand
}

// AIConfig AI 配置
type AIConfig struct {
	RememberCards bool
	UseBomb       bool
	Strategy      string  // random | minimum | optimal
	ResponseRate  float64 // 0.0 - 1.0
	DelayMsMin    int
	DelayMsMax    int
	CardInference bool
}

// NewAIEngine 创建 AI 引擎
func NewAIEngine(config *AIConfig) *AIEngine {
	return &AIEngine{
		config: config,
		rng:    rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// DecidePlay 决策出牌
func (ai *AIEngine) DecidePlay(ctx *AIContext) *PlayDecision {
	// 枚举所有合法出牌
	legalCombos := ai.enumerateLegalPlays(ctx)

	if len(legalCombos) == 0 {
		return &PlayDecision{Action: ActionPass}
	}

	// 自由出牌（没人出牌或上家都 Pass）
	if ctx.LastPlay == nil {
		return ai.decideFreePlay(ctx, legalCombos)
	}

	// 需要管牌
	return ai.decideResponsePlay(ctx, legalCombos)
}

// decideFreePlay 自由出牌策略
func (ai *AIEngine) decideFreePlay(ctx *AIContext, combos []CardCombo) *PlayDecision {
	// 检查是否能一手出完
	if len(ctx.MyCards) > 0 && len(ctx.MyCards) <= 5 {
		if ai.canPlayAllAtOnce(ctx.MyCards) {
			return &PlayDecision{Action: ActionPlay, Cards: ctx.MyCards}
		}
	}

	// 出最小的单张
	smallest := ai.findSmallestSingle(ctx.MyCards)
	if smallest != nil {
		return &PlayDecision{Action: ActionPlay, Cards: []cardutil.Card{*smallest}}
	}

	// 出最小的对子
	smallestPair := ai.findSmallestPair(ctx.MyCards)
	if smallestPair != nil {
		return &PlayDecision{Action: ActionPlay, Cards: smallestPair}
	}

	// fallback
	if len(ctx.MyCards) > 0 {
		return &PlayDecision{Action: ActionPlay, Cards: []cardutil.Card{ai.findSmallestCard(ctx.MyCards)}}
	}

	return &PlayDecision{Action: ActionPass}
}

// decideResponsePlay 管牌策略
func (ai *AIEngine) decideResponsePlay(ctx *AIContext, combos []CardCombo) *PlayDecision {
	validCombos := ai.filterBeatingCombos(combos, ctx.LastPlay)
	if len(validCombos) == 0 {
		return &PlayDecision{Action: ActionPass}
	}

	// 选择最小的能管上的牌
	bestCombo := ai.findMinimumCombo(validCombos)

	// 上家是队友 → 有机会就pass给队友
	if ai.shouldPassToTeammate(ctx) {
		// 如果手牌不强且队友可能能接牌，50%概率pass
		if ai.isHandWeak(ctx.MyCards) && len(ctx.MyCards) > 5 {
			if ai.rng.Float64() < 0.3 {
				return &PlayDecision{Action: ActionPass}
			}
		}
	}

	// 是否用炸弹？
	if ai.config.UseBomb && ai.shouldUseBomb(ctx, bestCombo) {
		bombCombo := ai.findSmallestBomb(ctx.MyCards)
		if bombCombo != nil {
			return &PlayDecision{Action: ActionPlay, Cards: bombCombo}
		}
	}

	return &PlayDecision{Action: ActionPlay, Cards: bestCombo}
}

// shouldPassToTeammate 是否应该放过队友
func (ai *AIEngine) shouldPassToTeammate(ctx *AIContext) bool {
	if ctx.MyRole == types.RoleLandlord {
		return false // 地主没有队友
	}

	if ctx.LastPlayerUID == "" {
		return false
	}

	// 判断上家是否是队友
	lastPlayer, exists := ctx.Players[ctx.LastPlayerUID]
	if !exists {
		return false
	}

	if !lastPlayer.IsLandlord && ctx.MyRole == types.RolePeasant {
		// 队友出牌，如果队友剩很少牌，放过
		if lastPlayer.CardCount <= 3 {
			return true
		}
	}

	return false
}

// isHandWeak 判断手牌是否弱
func (ai *AIEngine) isHandWeak(cards []cardutil.Card) bool {
	if len(cards) <= 3 {
		return false
	}

	// 没有大牌（A 以上）
	hasBigCard := false
	for _, c := range cards {
		if c.Value >= cardutil.CardValueA {
			hasBigCard = true
			break
		}
	}

	return !hasBigCard
}

// canPlayAllAtOnce 判断是否能一手出完
func (ai *AIEngine) canPlayAllAtOnce(cards []cardutil.Card) bool {
	result := cardutil.AnalyzePattern(cards)
	return result.Valid
}

// enumerateLegalPlays 枚举所有合法出牌组合（简化版）
func (ai *AIEngine) enumerateLegalPlays(ctx *AIContext) []CardCombo {
	// 简化：返回所有单张、对子、三条等组合
	combos := make([]CardCombo, 0)

	// 单张
	for _, c := range ctx.MyCards {
		combos = append(combos, CardCombo{Cards: []cardutil.Card{c}})
	}

	// 对子
	valueGroups := groupByValue(ctx.MyCards)
	for _, group := range valueGroups {
		if len(group) >= 2 {
			combos = append(combos, CardCombo{Cards: group[:2]})
		}
		if len(group) >= 3 {
			combos = append(combos, CardCombo{Cards: group[:3]})
		}
		if len(group) == 4 {
			combos = append(combos, CardCombo{Cards: group})
		}
	}

	return combos
}

// filterBeatingCombos 过滤能大过上家的组合
func (ai *AIEngine) filterBeatingCombos(combos []CardCombo, lastPlay *LastPlayInfo) []CardCombo {
	if lastPlay == nil {
		return combos
	}

	result := make([]CardCombo, 0)
	for _, combo := range combos {
		comboResult := cardutil.AnalyzePattern(combo.Cards)
		if !comboResult.Valid {
			continue
		}

		// 同类型比较
		if comboResult.Pattern == lastPlay.Pattern {
			if comboResult.Main > lastPlay.Main {
				result = append(result, combo)
			}
		} else if comboResult.Pattern.IsBomb() {
			// 炸弹可以打任何非炸弹
			if !lastPlay.Pattern.IsBomb() {
				result = append(result, combo)
			}
		}
	}

	return result
}

// findMinimumCombo 找到最小的组合
func (ai *AIEngine) findMinimumCombo(combos []CardCombo) []cardutil.Card {
	if len(combos) == 0 {
		return nil
	}

	minCombo := combos[0]
	for _, combo := range combos[1:] {
		result := cardutil.AnalyzePattern(combo.Cards)
		minResult := cardutil.AnalyzePattern(minCombo.Cards)
		if result.Main < minResult.Main {
			minCombo = combo
		}
	}

	return minCombo.Cards
}

// findSmallestSingle 找到最小的单张
func (ai *AIEngine) findSmallestSingle(cards []cardutil.Card) *cardutil.Card {
	if len(cards) == 0 {
		return nil
	}

	smallest := &cards[0]
	for i := 1; i < len(cards); i++ {
		if cards[i].Value < smallest.Value {
			smallest = &cards[i]
		}
	}

	return smallest
}

// findSmallestPair 找到最小的对子
func (ai *AIEngine) findSmallestPair(cards []cardutil.Card) []cardutil.Card {
	groups := groupByValue(cards)
	for _, group := range groups {
		if len(group) >= 2 {
			return group[:2]
		}
	}
	return nil
}

// findSmallestCard 找到最小的牌
func (ai *AIEngine) findSmallestCard(cards []cardutil.Card) cardutil.Card {
	if len(cards) == 0 {
		return cardutil.Card{}
	}

	smallest := cards[0]
	for _, c := range cards[1:] {
		if c.Value < smallest.Value {
			smallest = c
		}
	}

	return smallest
}

// findSmallestBomb 找到最小的炸弹
func (ai *AIEngine) findSmallestBomb(cards []cardutil.Card) []cardutil.Card {
	groups := groupByValue(cards)
	for _, group := range groups {
		if len(group) == 4 {
			return group
		}
	}
	return nil
}

// shouldUseBomb 判断是否应该使用炸弹
func (ai *AIEngine) shouldUseBomb(ctx *AIContext, currentPlay []cardutil.Card) bool {
	// 简化：如果当前出牌很小，且对手可能有大牌，则用炸弹
	if len(currentPlay) == 0 {
		return false
	}

	result := cardutil.AnalyzePattern(currentPlay)
	if result.Main < cardutil.CardValueK {
		// 出牌很小，考虑用炸弹
		return ai.rng.Float64() < 0.3
	}

	return false
}

// CardCombo 牌组合
type CardCombo struct {
	Cards []cardutil.Card
}

// groupByValue 按牌值分组
func groupByValue(cards []cardutil.Card) map[cardutil.CardValue][]cardutil.Card {
	groups := make(map[cardutil.CardValue][]cardutil.Card)
	for _, c := range cards {
		groups[c.Value] = append(groups[c.Value], c)
	}
	return groups
}
