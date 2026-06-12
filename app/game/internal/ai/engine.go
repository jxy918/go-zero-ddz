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
	// 农民配合策略配置
	PassToTeammateEnabled bool    // 是否启用放过队友策略
	PassProbabilityHigh   float64 // 队友≤3张牌时的放过概率（高优先级）
	PassProbabilityMedium float64 // 队友≤5张牌时的放过概率（中优先级）
	PassProbabilityLow    float64 // 队友≤7张牌时的放过概率（低优先级）
}

// NewAIEngine 创建 AI 引擎
func NewAIEngine(config *AIConfig) *AIEngine {
	// 设置默认放过概率配置
	if config.PassProbabilityHigh == 0 {
		config.PassProbabilityHigh = 0.8 // 队友≤3张牌，80%概率放过
	}
	if config.PassProbabilityMedium == 0 {
		config.PassProbabilityMedium = 0.5 // 队友≤5张牌，50%概率放过
	}
	if config.PassProbabilityLow == 0 {
		config.PassProbabilityLow = 0.3 // 队友≤7张牌，30%概率放过
	}
	// 默认启用放过队友策略
	if !config.PassToTeammateEnabled && config.PassProbabilityHigh > 0 {
		config.PassToTeammateEnabled = true
	}

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
	// 1. 优先尝试快速清场
	if quickPlay := ai.findQuickClearPlays(ctx.MyCards, combos); quickPlay != nil {
		return &PlayDecision{Action: ActionPlay, Cards: quickPlay}
	}

	// 2. 判断是否需要保留大牌
	shouldReserve := ai.shouldReserveBigCards(ctx)

	// 3. 根据策略选择出牌
	if shouldReserve {
		// 保留大牌模式：优先出小牌
		if smallPlay := ai.findSmallCardPlay(ctx.MyCards, combos); smallPlay != nil {
			return &PlayDecision{Action: ActionPlay, Cards: smallPlay}
		}
	}

	// 4. 评估拆牌代价，选择最优出牌
	bestPlay := ai.findBestFreePlay(ctx.MyCards, combos)
	if bestPlay != nil {
		return &PlayDecision{Action: ActionPlay, Cards: bestPlay}
	}

	// 5. fallback：出最小的牌
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

	// 上家是队友 → 根据策略决定是否放过
	if ai.shouldPassToTeammate(ctx) {
		return &PlayDecision{Action: ActionPass}
	}

	// 选择最小的能管上的牌
	bestCombo := ai.findMinimumCombo(validCombos)

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
	if !ai.config.PassToTeammateEnabled {
		return false
	}

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

	// 只有农民队友才放过
	if lastPlayer.IsLandlord || ctx.MyRole != types.RolePeasant {
		return false
	}

	// 评估队友局势
	situation := ai.evaluateTeammateSituation(ctx, lastPlayer)

	// 根据队友剩余牌数决定放过概率
	passProbability := 0.0

	switch {
	case situation.RemainingCards <= 3:
		// 队友剩余≤3张牌，高概率放过
		passProbability = ai.config.PassProbabilityHigh
	case situation.RemainingCards <= 5 && situation.MyHandWeak:
		// 队友剩余≤5张牌且自己手弱，中等概率放过
		passProbability = ai.config.PassProbabilityMedium
	case situation.RemainingCards <= 7 && situation.MyHandVeryWeak:
		// 队友剩余≤7张牌且自己手很弱，低概率放过
		passProbability = ai.config.PassProbabilityLow
	}

	// 随机决定是否放过
	return ai.rng.Float64() < passProbability
}

// TeammateSituation 队友局势评估
type TeammateSituation struct {
	RemainingCards  int     // 队友剩余牌数
	IsLandlord      bool    // 队友是否是地主
	CurrentPlaySize int     // 当前牌型大小
	MyHandWeak      bool    // 自己手牌是否弱
	MyHandVeryWeak  bool    // 自己手牌是否很弱
	TeammateUrgent  bool    // 队友是否紧急（剩余牌很少）
}

// evaluateTeammateSituation 评估队友局势
func (ai *AIEngine) evaluateTeammateSituation(ctx *AIContext, teammate *PlayerInfo) *TeammateSituation {
	situation := &TeammateSituation{
		RemainingCards:  teammate.CardCount,
		IsLandlord:      teammate.IsLandlord,
		CurrentPlaySize: 0,
		MyHandWeak:      false,
		MyHandVeryWeak:  false,
		TeammateUrgent:  teammate.CardCount <= 3,
	}

	// 计算当前牌型大小
	if ctx.LastPlay != nil {
		situation.CurrentPlaySize = int(ctx.LastPlay.Main)
	}

	// 评估自己手牌强度
	situation.MyHandWeak = ai.isHandWeak(ctx.MyCards)
	situation.MyHandVeryWeak = ai.isHandVeryWeak(ctx.MyCards)

	return situation
}

// isHandVeryWeak 判断手牌是否很弱
func (ai *AIEngine) isHandVeryWeak(cards []cardutil.Card) bool {
	if len(cards) <= 3 {
		return false
	}

	// 没有中等以上的牌（10以上）
	hasMediumCard := false
	for _, c := range cards {
		if c.Value >= cardutil.CardValue10 {
			hasMediumCard = true
			break
		}
	}

	return !hasMediumCard
}

// isHandWeak 判断手牌是否弱
func (ai *AIEngine) isHandWeak(cards []cardutil.Card) bool {
	if len(cards) <= 3 {
		return false
	}

	// 综合评估手牌强度
	totalScore := 0

	// 1. 大牌得分
	totalScore += ai.countBigCards(cards)

	// 2. 炸弹得分
	totalScore += ai.countBombs(cards)

	// 3. 牌型结构得分
	totalScore += ai.evaluateHandStructure(cards)

	// 根据总分判断手牌强弱
	// 总分>=8：手牌强（返回false）
	// 总分>=5：手牌中等（返回false）
	// 总分<5：手牌弱（返回true）
	return totalScore < 5
}

// countBombs 统计炸弹数量得分
// 每个炸弹计3分，王炸计5分
func (ai *AIEngine) countBombs(cards []cardutil.Card) int {
	score := 0

	// 检查是否有王炸
	hasSmallJoker := false
	hasBigJoker := false
	for _, c := range cards {
		if c.Value == cardutil.CardValueJokerS {
			hasSmallJoker = true
		}
		if c.Value == cardutil.CardValueJokerB {
			hasBigJoker = true
		}
	}
	if hasSmallJoker && hasBigJoker {
		score += 5
	}

	// 检查普通炸弹（四张相同的牌）
	groups := groupByValue(cards)
	for _, group := range groups {
		if len(group) == 4 {
			score += 3
		}
	}

	return score
}

// evaluateHandStructure 评估牌型结构得分
// 剩余牌数少（<=5）计2分，有完整牌型（顺子、连对等）计1分
func (ai *AIEngine) evaluateHandStructure(cards []cardutil.Card) int {
	score := 0

	// 剩余牌数少，容易出完
	if len(cards) <= 5 {
		score += 2
	}

	// 检查是否有完整牌型
	result := cardutil.AnalyzePattern(cards)
	if result.Valid {
		// 如果所有牌能组成一个完整牌型，加分
		switch result.Pattern {
		case cardutil.PatternStraight, cardutil.PatternStraightPair,
			cardutil.PatternAirplane, cardutil.PatternAirplaneWings:
			score += 1
		}
	}

	return score
}

// canPlayAllAtOnce 判断是否能一手出完
func (ai *AIEngine) canPlayAllAtOnce(cards []cardutil.Card) bool {
	result := cardutil.AnalyzePattern(cards)
	return result.Valid
}

// enumerateLegalPlays 枚举所有合法出牌组合
func (ai *AIEngine) enumerateLegalPlays(ctx *AIContext) []CardCombo {
	combos := make([]CardCombo, 0)
	valueGroups := groupByValue(ctx.MyCards)

	// 1. 单张
	for _, c := range ctx.MyCards {
		combos = append(combos, CardCombo{Cards: []cardutil.Card{c}})
	}

	// 2. 对子
	for _, group := range valueGroups {
		if len(group) >= 2 {
			combos = append(combos, CardCombo{Cards: group[:2]})
		}
	}

	// 3. 三条
	for _, group := range valueGroups {
		if len(group) >= 3 {
			combos = append(combos, CardCombo{Cards: group[:3]})
		}
	}

	// 4. 炸弹（四张相同）
	for _, group := range valueGroups {
		if len(group) == 4 {
			combos = append(combos, CardCombo{Cards: group})
		}
	}

	// 5. 王炸
	if rocket := ai.findRocket(ctx.MyCards); rocket != nil {
		combos = append(combos, CardCombo{Cards: rocket})
	}

	// 6. 三带一
	combos = append(combos, ai.findThreeWithOne(ctx.MyCards, valueGroups)...)

	// 7. 三带二
	combos = append(combos, ai.findThreeWithTwo(ctx.MyCards, valueGroups)...)

	// 8. 顺子
	combos = append(combos, ai.findStraights(ctx.MyCards, valueGroups)...)

	// 9. 连对
	combos = append(combos, ai.findDoubleStraights(ctx.MyCards, valueGroups)...)

	// 10. 飞机（含带翅膀）
	combos = append(combos, ai.findPlanes(ctx.MyCards, valueGroups)...)

	// 11. 四带二
	combos = append(combos, ai.findFourWithTwo(ctx.MyCards, valueGroups)...)

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

// shouldUseBomb 判断是否应该使用炸弹（基于场景决策）
func (ai *AIEngine) shouldUseBomb(ctx *AIContext, currentPlay []cardutil.Card) bool {
	if len(currentPlay) == 0 {
		return false
	}

	// 评估炸弹使用场景
	score := ai.evaluateBombSituation(ctx)

	// 决策逻辑
	if score >= 40 {
		// 关键时刻，必须使用炸弹
		return true
	}

	if score >= 20 && !ai.isHandWeak(ctx.MyCards) {
		// 优势时刻且手牌强，使用炸弹
		return true
	}

	// 其他情况不使用炸弹
	return false
}

// evaluateBombSituation 评估炸弹使用场景，返回分数
func (ai *AIEngine) evaluateBombSituation(ctx *AIContext) int {
	score := 0

	// 1. 关键时刻：地主剩牌≤2张
	if ai.isLandlordCritical(ctx) {
		score += 50
	}

	// 2. 关键时刻：队友即将出完（剩牌≤3张且是农民）
	if ai.isTeammateFinishing(ctx) {
		score += 40
	}

	// 3. 优势时刻：自己剩牌≤3张且炸弹能清场
	if ai.canBombFinishGame(ctx) {
		score += 30
	}

	// 4. 保守时刻：游戏前期（剩余牌>10张）
	if len(ctx.MyCards) > 10 {
		score -= 30
	}

	// 5. 保守时刻：对手可能有大牌
	if ai.opponentMayHaveBigCards(ctx) {
		score -= 20
	}

	return score
}

// isLandlordCritical 判断地主是否处于关键时刻（剩牌≤2张）
func (ai *AIEngine) isLandlordCritical(ctx *AIContext) bool {
	// 如果自己是地主，检查农民的剩余牌数
	if ctx.MyRole == types.RoleLandlord {
		for _, player := range ctx.Players {
			if !player.IsLandlord && player.CardCount <= 2 {
				return true
			}
		}
		return false
	}

	// 如果自己是农民，检查地主的剩余牌数
	for _, player := range ctx.Players {
		if player.IsLandlord && player.CardCount <= 2 {
			return true
		}
	}

	return false
}

// isTeammateFinishing 判断队友是否即将出完（剩牌≤3张且是农民）
func (ai *AIEngine) isTeammateFinishing(ctx *AIContext) bool {
	// 地主没有队友
	if ctx.MyRole == types.RoleLandlord {
		return false
	}

	// 检查其他农民队友
	for uid, player := range ctx.Players {
		// 跳过自己和地主
		if uid == ctx.LastPlayerUID || player.IsLandlord {
			continue
		}

		// 队友是农民且剩牌≤3张
		if !player.IsLandlord && player.CardCount <= 3 {
			return true
		}
	}

	return false
}

// canBombFinishGame 判断自己是否可以用炸弹清场结束游戏
func (ai *AIEngine) canBombFinishGame(ctx *AIContext) bool {
	// 自己剩牌必须≤3张
	if len(ctx.MyCards) > 3 {
		return false
	}

	// 检查是否有炸弹
	bomb := ai.findSmallestBomb(ctx.MyCards)
	if bomb == nil {
		return false
	}

	// 计算使用炸弹后的剩余牌数
	remainingCards := len(ctx.MyCards) - 4

	// 如果炸弹能清场或剩余牌能一手出完
	if remainingCards == 0 {
		return true
	}

	// 检查剩余牌是否能一手出完
	if remainingCards > 0 && remainingCards <= 5 {
		remaining := ai.removeCards(ctx.MyCards, bomb)
		if ai.canPlayAllAtOnce(remaining) {
			return true
		}
	}

	return false
}

// opponentMayHaveBigCards 判断对手是否可能有大牌
func (ai *AIEngine) opponentMayHaveBigCards(ctx *AIContext) bool {
	// 如果有记牌器，检查大牌是否已出完
	if ctx.CardCounter != nil {
		// 检查大王、小王、2是否还在对手手中
		bigCardsRemaining := ctx.CardCounter.CountRemaining(cardutil.CardValueJokerB) +
			ctx.CardCounter.CountRemaining(cardutil.CardValueJokerS) +
			ctx.CardCounter.CountRemaining(cardutil.CardValue2)

		// 如果大牌剩余数量 > 自己手中的大牌数量，说明对手可能有大牌
		myBigCards := ai.countBigCards(ctx.MyCards)
		if bigCardsRemaining > myBigCards {
			return true
		}
	}

	// 如果没有记牌器，保守估计
	// 检查自己手中是否有大牌
	if ai.countBigCards(ctx.MyCards) == 0 {
		// 自己没有大牌，对手可能有
		return true
	}

	return false
}

// countBigCards 统计大牌数量得分
// 每个2计2分，每个A计1分，大王计3分，小王计2分
func (ai *AIEngine) countBigCards(cards []cardutil.Card) int {
	score := 0
	for _, c := range cards {
		switch c.Value {
		case cardutil.CardValue2:
			score += 2
		case cardutil.CardValueA:
			score += 1
		case cardutil.CardValueJokerB: // 大王
			score += 3
		case cardutil.CardValueJokerS: // 小王
			score += 2
		}
	}
	return score
}

// removeCards 从手牌中移除指定牌
func (ai *AIEngine) removeCards(cards []cardutil.Card, toRemove []cardutil.Card) []cardutil.Card {
	result := make([]cardutil.Card, 0)
	removeMap := make(map[string]bool)

	for _, c := range toRemove {
		key := string(rune(c.Value)) + string(rune(c.Suit))
		removeMap[key] = true
	}

	for _, c := range cards {
		key := string(rune(c.Value)) + string(rune(c.Suit))
		if !removeMap[key] {
			result = append(result, c)
		}
	}

	return result
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

// findRocket 查找王炸组合
func (ai *AIEngine) findRocket(cards []cardutil.Card) []cardutil.Card {
	var jokerS, jokerB *cardutil.Card
	for i := range cards {
		if cards[i].Value == cardutil.CardValueJokerS {
			jokerS = &cards[i]
		}
		if cards[i].Value == cardutil.CardValueJokerB {
			jokerB = &cards[i]
		}
	}
	if jokerS != nil && jokerB != nil {
		return []cardutil.Card{*jokerS, *jokerB}
	}
	return nil
}

// findThreeWithOne 查找所有三带一组合
func (ai *AIEngine) findThreeWithOne(cards []cardutil.Card, valueGroups map[cardutil.CardValue][]cardutil.Card) []CardCombo {
	combos := make([]CardCombo, 0)

	// 找出所有三条
	triples := make([]cardutil.CardValue, 0)
	for v, group := range valueGroups {
		if len(group) >= 3 && v >= cardutil.CardValue3 && v <= cardutil.CardValue2 {
			triples = append(triples, v)
		}
	}

	// 找出所有可以作为单张的牌值
	singles := make([]cardutil.CardValue, 0)
	for v, group := range valueGroups {
		if len(group) >= 1 {
			singles = append(singles, v)
		}
	}

	// 枚举所有三带一组合
	for _, tripleVal := range triples {
		tripleCards := valueGroups[tripleVal][:3]
		for _, singleVal := range singles {
			if singleVal == tripleVal {
				// 如果该牌值有4张，可以用多出的一张作为单张
				if len(valueGroups[tripleVal]) == 4 {
					combos = append(combos, CardCombo{
						Cards: append(copyCards(tripleCards), valueGroups[tripleVal][3]),
					})
				}
				continue
			}
			// 正常的三带一
			combos = append(combos, CardCombo{
				Cards: append(copyCards(tripleCards), valueGroups[singleVal][0]),
			})
		}
	}

	return combos
}

// findThreeWithTwo 查找所有三带二组合
func (ai *AIEngine) findThreeWithTwo(cards []cardutil.Card, valueGroups map[cardutil.CardValue][]cardutil.Card) []CardCombo {
	combos := make([]CardCombo, 0)

	// 找出所有三条
	triples := make([]cardutil.CardValue, 0)
	for v, group := range valueGroups {
		if len(group) >= 3 && v >= cardutil.CardValue3 && v <= cardutil.CardValue2 {
			triples = append(triples, v)
		}
	}

	// 找出所有对子
	pairs := make([]cardutil.CardValue, 0)
	for v, group := range valueGroups {
		if len(group) >= 2 {
			pairs = append(pairs, v)
		}
	}

	// 枚举所有三带二组合
	for _, tripleVal := range triples {
		tripleCards := valueGroups[tripleVal][:3]
		for _, pairVal := range pairs {
			if pairVal == tripleVal {
				continue // 三条和对子不能是同一个牌值
			}
			pairCards := valueGroups[pairVal][:2]
			combos = append(combos, CardCombo{
				Cards: append(copyCards(tripleCards), pairCards...),
			})
		}
	}

	return combos
}

// findStraights 查找所有顺子组合（5张及以上连续单张，不含2和王）
func (ai *AIEngine) findStraights(cards []cardutil.Card, valueGroups map[cardutil.CardValue][]cardutil.Card) []CardCombo {
	combos := make([]CardCombo, 0)

	// 收集所有可用的单张牌值（不含2和王，且至少有1张）
	availableValues := make([]cardutil.CardValue, 0)
	for v := cardutil.CardValue3; v <= cardutil.CardValueA; v++ {
		if group, exists := valueGroups[v]; exists && len(group) >= 1 {
			availableValues = append(availableValues, v)
		}
	}

	if len(availableValues) < 5 {
		return combos // 顺子至少需要5张
	}

	// 枚举所有可能的顺子长度（5到最大可用长度）
	maxLen := len(availableValues)
	for length := 5; length <= maxLen; length++ {
		// 滑动窗口找连续的牌值
		for i := 0; i <= len(availableValues)-length; i++ {
			// 检查是否连续
			isConsecutive := true
			for j := 1; j < length; j++ {
				if availableValues[i+j] != availableValues[i+j-1]+1 {
					isConsecutive = false
					break
				}
			}
			if !isConsecutive {
				continue
			}

			// 构建顺子（每个牌值取一张）
			straightCards := make([]cardutil.Card, 0, length)
			for j := 0; j < length; j++ {
				straightCards = append(straightCards, valueGroups[availableValues[i+j]][0])
			}
			combos = append(combos, CardCombo{Cards: straightCards})
		}
	}

	return combos
}

// findDoubleStraights 查找所有连对组合（3对及以上连续对子）
func (ai *AIEngine) findDoubleStraights(cards []cardutil.Card, valueGroups map[cardutil.CardValue][]cardutil.Card) []CardCombo {
	combos := make([]CardCombo, 0)

	// 收集所有可用的对子牌值（不含2和王，且至少有2张）
	availablePairs := make([]cardutil.CardValue, 0)
	for v := cardutil.CardValue3; v <= cardutil.CardValueA; v++ {
		if group, exists := valueGroups[v]; exists && len(group) >= 2 {
			availablePairs = append(availablePairs, v)
		}
	}

	if len(availablePairs) < 3 {
		return combos // 连对至少需要3对
	}

	// 枚举所有可能的连对长度（3对到最大可用对数）
	maxPairs := len(availablePairs)
	for pairCount := 3; pairCount <= maxPairs; pairCount++ {
		// 滑动窗口找连续的对子牌值
		for i := 0; i <= len(availablePairs)-pairCount; i++ {
			// 检查是否连续
			isConsecutive := true
			for j := 1; j < pairCount; j++ {
				if availablePairs[i+j] != availablePairs[i+j-1]+1 {
					isConsecutive = false
					break
				}
			}
			if !isConsecutive {
				continue
			}

			// 构建连对（每个牌值取两张）
			pairCards := make([]cardutil.Card, 0, pairCount*2)
			for j := 0; j < pairCount; j++ {
				pairCards = append(pairCards, valueGroups[availablePairs[i+j]][:2]...)
			}
			combos = append(combos, CardCombo{Cards: pairCards})
		}
	}

	return combos
}

// findPlanes 查找所有飞机组合（含带翅膀）
func (ai *AIEngine) findPlanes(cards []cardutil.Card, valueGroups map[cardutil.CardValue][]cardutil.Card) []CardCombo {
	combos := make([]CardCombo, 0)

	// 收集所有可用的三条牌值（不含2和王，且至少有3张）
	availableTriples := make([]cardutil.CardValue, 0)
	for v := cardutil.CardValue3; v <= cardutil.CardValueA; v++ {
		if group, exists := valueGroups[v]; exists && len(group) >= 3 {
			availableTriples = append(availableTriples, v)
		}
	}

	if len(availableTriples) < 2 {
		return combos // 飞机至少需要2个三条
	}

	// 枚举所有可能的飞机长度（2个到最大可用三条数）
	maxTriples := len(availableTriples)
	for tripleCount := 2; tripleCount <= maxTriples; tripleCount++ {
		// 滑动窗口找连续的三条牌值
		for i := 0; i <= len(availableTriples)-tripleCount; i++ {
			// 检查是否连续
			isConsecutive := true
			for j := 1; j < tripleCount; j++ {
				if availableTriples[i+j] != availableTriples[i+j-1]+1 {
					isConsecutive = false
					break
				}
			}
			if !isConsecutive {
				continue
			}

			// 构建飞机主体（每个牌值取三张）
			tripleValues := availableTriples[i : i+tripleCount]
			planeCards := make([]cardutil.Card, 0, tripleCount*3)
			for _, v := range tripleValues {
				planeCards = append(planeCards, valueGroups[v][:3]...)
			}

			// 1. 飞机不带翅膀
			combos = append(combos, CardCombo{Cards: copyCards(planeCards)})

			// 2. 飞机带单张（需要等量的单张）
			singleWings := ai.findWings(valueGroups, tripleValues, 1, tripleCount)
			for _, wing := range singleWings {
				combos = append(combos, CardCombo{
					Cards: append(copyCards(planeCards), wing...),
				})
			}

			// 3. 飞机带对子（需要等量的对子）
			pairWings := ai.findWings(valueGroups, tripleValues, 2, tripleCount)
			for _, wing := range pairWings {
				combos = append(combos, CardCombo{
					Cards: append(copyCards(planeCards), wing...),
				})
			}
		}
	}

	return combos
}

// findWings 查找飞机翅膀
// wingType: 1=单张, 2=对子
// count: 需要的翅膀数量
func (ai *AIEngine) findWings(valueGroups map[cardutil.CardValue][]cardutil.Card,
	excludeValues []cardutil.CardValue, wingType, count int) [][]cardutil.Card {
	wings := make([][]cardutil.Card, 0)

	// 收集可用的翅膀牌值（排除飞机主体使用的牌值）
	excludeSet := make(map[cardutil.CardValue]bool)
	for _, v := range excludeValues {
		excludeSet[v] = true
	}

	availableWings := make([]cardutil.CardValue, 0)
	for v, group := range valueGroups {
		if excludeSet[v] {
			// 如果是飞机主体的牌值，检查是否有额外的牌可用
			if wingType == 1 && len(group) >= 4 {
				// 四条可以拆出一张作为翅膀
				availableWings = append(availableWings, v)
			}
			continue
		}
		if wingType == 1 && len(group) >= 1 {
			availableWings = append(availableWings, v)
		} else if wingType == 2 && len(group) >= 2 {
			availableWings = append(availableWings, v)
		}
	}

	if len(availableWings) < count {
		return wings
	}

	// 使用回溯法枚举所有翅膀组合
	ai.enumerateWings(valueGroups, excludeSet, wingType, count, 0, make([]cardutil.Card, 0), &wings)

	return wings
}

// enumerateWings 回溯枚举翅膀组合
func (ai *AIEngine) enumerateWings(valueGroups map[cardutil.CardValue][]cardutil.Card,
	excludeSet map[cardutil.CardValue]bool, wingType, count, startIdx int,
	current []cardutil.Card, result *[][]cardutil.Card) {

	if len(current) == count*wingType {
		*result = append(*result, copyCards(current))
		return
	}

	// 收集可用的牌值
	availableValues := make([]cardutil.CardValue, 0)
	for v, group := range valueGroups {
		if excludeSet[v] {
			// 飞机主体的牌值，只有四条可以额外提供单张翅膀
			if wingType == 1 && len(group) == 4 {
				availableValues = append(availableValues, v)
			}
			continue
		}
		if wingType == 1 && len(group) >= 1 {
			availableValues = append(availableValues, v)
		} else if wingType == 2 && len(group) >= 2 {
			availableValues = append(availableValues, v)
		}
	}

	for i := startIdx; i < len(availableValues); i++ {
		v := availableValues[i]
		var wingCards []cardutil.Card
		if excludeSet[v] {
			// 四条的第4张作为翅膀
			wingCards = []cardutil.Card{valueGroups[v][3]}
		} else if wingType == 1 {
			wingCards = []cardutil.Card{valueGroups[v][0]}
		} else {
			wingCards = valueGroups[v][:2]
		}
		ai.enumerateWings(valueGroups, excludeSet, wingType, count, i+1,
			append(copyCards(current), wingCards...), result)
	}
}

// findFourWithTwo 查找所有四带二组合
func (ai *AIEngine) findFourWithTwo(cards []cardutil.Card, valueGroups map[cardutil.CardValue][]cardutil.Card) []CardCombo {
	combos := make([]CardCombo, 0)

	// 找出所有四条
	fours := make([]cardutil.CardValue, 0)
	for v, group := range valueGroups {
		if len(group) == 4 && v >= cardutil.CardValue3 && v <= cardutil.CardValue2 {
			fours = append(fours, v)
		}
	}

	if len(fours) == 0 {
		return combos
	}

	// 收集所有可用的单张和对子
	singles := make([]cardutil.CardValue, 0)
	pairs := make([]cardutil.CardValue, 0)
	for v, group := range valueGroups {
		if len(group) >= 1 {
			singles = append(singles, v)
		}
		if len(group) >= 2 {
			pairs = append(pairs, v)
		}
	}

	for _, fourVal := range fours {
		fourCards := valueGroups[fourVal]

		// 四带两单
		for i := 0; i < len(singles); i++ {
			if singles[i] == fourVal {
				continue
			}
			for j := i + 1; j < len(singles); j++ {
				if singles[j] == fourVal {
					continue
				}
				combos = append(combos, CardCombo{
					Cards: append(copyCards(fourCards),
						valueGroups[singles[i]][0],
						valueGroups[singles[j]][0]),
				})
			}
		}

		// 四带一对
		for _, pairVal := range pairs {
			if pairVal == fourVal {
				continue
			}
			combos = append(combos, CardCombo{
				Cards: append(copyCards(fourCards), valueGroups[pairVal][:2]...),
			})
		}
	}

	return combos
}

// copyCards 复制牌切片
func copyCards(cards []cardutil.Card) []cardutil.Card {
	result := make([]cardutil.Card, len(cards))
	copy(result, cards)
	return result
}

// findBestComboWithSplitCost 考虑拆牌代价,找到最优出牌组合
func (ai *AIEngine) findBestComboWithSplitCost(ctx *AIContext, combos []CardCombo) []cardutil.Card {
	if len(combos) == 0 {
		return nil
	}

	type ComboScore struct {
		Cards        []cardutil.Card
		PatternValue int // 牌型价值
		SplitCost    int // 拆牌代价
		NetValue     int // 净价值 = 牌型价值 - 拆牌代价
		MainValue    cardutil.CardValue
	}

	scores := make([]ComboScore, 0, len(combos))

	// 计算每个组合的分数
	for _, combo := range combos {
		result := cardutil.AnalyzePattern(combo.Cards)
		if !result.Valid {
			continue
		}

		patternValue := ai.evaluatePatternValue(combo.Cards, result)
		splitCost := ai.calculateSplitCost(combo.Cards, ctx.MyCards)
		netValue := patternValue - splitCost

		scores = append(scores, ComboScore{
			Cards:        combo.Cards,
			PatternValue: patternValue,
			SplitCost:    splitCost,
			NetValue:     netValue,
			MainValue:    result.Main,
		})
	}

	if len(scores) == 0 {
		return nil
	}

	// 判断是否是队友出牌
	isTeammatePlay := ai.isTeammatePlay(ctx)

	// 筛选策略
	candidateScores := make([]ComboScore, 0)
	for _, score := range scores {
		// 如果是队友出牌,更倾向于不拆牌(提高拆牌代价的惩罚)
		if isTeammatePlay {
			// 队友出牌时,拆牌代价惩罚加倍
			adjustedNetValue := score.PatternValue - score.SplitCost*2
			if adjustedNetValue > 0 {
				candidateScores = append(candidateScores, score)
			}
		} else {
			// 对手出牌时,只要净价值>0就考虑
			if score.NetValue > 0 {
				candidateScores = append(candidateScores, score)
			}
		}
	}

	// 如果没有符合条件的组合,选择拆牌代价最小的
	if len(candidateScores) == 0 {
		minCostCombo := scores[0]
		for _, score := range scores[1:] {
			if score.SplitCost < minCostCombo.SplitCost {
				minCostCombo = score
			} else if score.SplitCost == minCostCombo.SplitCost && score.MainValue < minCostCombo.MainValue {
				// 拆牌代价相同时,选择牌值更小的
				minCostCombo = score
			}
		}
		return minCostCombo.Cards
	}

	// 从候选组合中选择牌值最小的
	bestCombo := candidateScores[0]
	for _, score := range candidateScores[1:] {
		if score.MainValue < bestCombo.MainValue {
			bestCombo = score
		}
	}

	return bestCombo.Cards
}

// isTeammatePlay 判断是否是队友出牌
func (ai *AIEngine) isTeammatePlay(ctx *AIContext) bool {
	if ctx.MyRole == types.RoleLandlord {
		return false // 地主没有队友
	}

	if ctx.LastPlayerUID == "" {
		return false
	}

	lastPlayer, exists := ctx.Players[ctx.LastPlayerUID]
	if !exists {
		return false
	}

	// 只有农民队友才算队友
	return !lastPlayer.IsLandlord && ctx.MyRole == types.RolePeasant
}

// calculateSplitCost 计算拆牌代价
// 评估打出cardsToPlay后,对allCards中其他牌型的破坏程度
func (ai *AIEngine) calculateSplitCost(cardsToPlay []cardutil.Card, allCards []cardutil.Card) int {
	if len(cardsToPlay) == 0 || len(allCards) == 0 {
		return 0
	}

	// 计算剩余手牌
	remainingCards := ai.removeCards(allCards, cardsToPlay)

	// 分析原始手牌中的完整牌型
	originalPatterns := ai.identifyCompletePatterns(allCards)

	// 分析剩余手牌中的完整牌型
	remainingPatterns := ai.identifyCompletePatterns(remainingCards)

	// 计算拆牌代价 = 原始牌型价值 - 剩余牌型价值
	originalValue := 0
	for _, pattern := range originalPatterns {
		originalValue += ai.getPatternSplitCost(pattern.Pattern)
	}

	remainingValue := 0
	for _, pattern := range remainingPatterns {
		remainingValue += ai.getPatternSplitCost(pattern.Pattern)
	}

	splitCost := originalValue - remainingValue
	if splitCost < 0 {
		splitCost = 0
	}

	return splitCost
}

// PatternInfo 牌型信息
type PatternInfo struct {
	Pattern cardutil.CardPattern
	Cards   []cardutil.Card
	Main    cardutil.CardValue
}

// identifyCompletePatterns 识别手牌中的完整牌型
func (ai *AIEngine) identifyCompletePatterns(cards []cardutil.Card) []PatternInfo {
	patterns := make([]PatternInfo, 0)
	if len(cards) == 0 {
		return patterns
	}

	// 按牌值分组
	valueGroups := groupByValue(cards)

	// 1. 识别王炸
	if rocket := ai.findRocket(cards); rocket != nil {
		patterns = append(patterns, PatternInfo{
			Pattern: cardutil.PatternRocket,
			Cards:   rocket,
			Main:    cardutil.CardValueJokerB,
		})
	}

	// 2. 识别炸弹
	for v, group := range valueGroups {
		if len(group) == 4 && v >= cardutil.CardValue3 && v <= cardutil.CardValue2 {
			patterns = append(patterns, PatternInfo{
				Pattern: cardutil.PatternBomb,
				Cards:   group,
				Main:    v,
			})
		}
	}

	// 3. 识别顺子
	straights := ai.findStraights(cards, valueGroups)
	for _, straight := range straights {
		result := cardutil.AnalyzePattern(straight.Cards)
		if result.Valid {
			patterns = append(patterns, PatternInfo{
				Pattern: result.Pattern,
				Cards:   straight.Cards,
				Main:    result.Main,
			})
		}
	}

	// 4. 识别连对
	doubleStraights := ai.findDoubleStraights(cards, valueGroups)
	for _, ds := range doubleStraights {
		result := cardutil.AnalyzePattern(ds.Cards)
		if result.Valid {
			patterns = append(patterns, PatternInfo{
				Pattern: result.Pattern,
				Cards:   ds.Cards,
				Main:    result.Main,
			})
		}
	}

	// 5. 识别飞机
	planes := ai.findPlanes(cards, valueGroups)
	for _, plane := range planes {
		result := cardutil.AnalyzePattern(plane.Cards)
		if result.Valid {
			patterns = append(patterns, PatternInfo{
				Pattern: result.Pattern,
				Cards:   plane.Cards,
				Main:    result.Main,
			})
		}
	}

	// 6. 识别三带一、三带二
	threeWithOne := ai.findThreeWithOne(cards, valueGroups)
	for _, two := range threeWithOne {
		result := cardutil.AnalyzePattern(two.Cards)
		if result.Valid && result.Pattern == cardutil.PatternTripleOne {
			patterns = append(patterns, PatternInfo{
				Pattern: result.Pattern,
				Cards:   two.Cards,
				Main:    result.Main,
			})
		}
	}

	threeWithTwo := ai.findThreeWithTwo(cards, valueGroups)
	for _, twt := range threeWithTwo {
		result := cardutil.AnalyzePattern(twt.Cards)
		if result.Valid && result.Pattern == cardutil.PatternTripleTwo {
			patterns = append(patterns, PatternInfo{
				Pattern: result.Pattern,
				Cards:   twt.Cards,
				Main:    result.Main,
			})
		}
	}

	return patterns
}

// getPatternSplitCost 获取牌型的拆牌代价
func (ai *AIEngine) getPatternSplitCost(pattern cardutil.CardPattern) int {
	switch pattern {
	case cardutil.PatternBomb:
		return 10 // 拆炸弹代价=10分
	case cardutil.PatternRocket:
		return 15 // 拆王炸代价=15分
	case cardutil.PatternStraight:
		return 3 // 拆顺子代价=3分
	case cardutil.PatternStraightPair:
		return 4 // 拆连对代价=4分
	case cardutil.PatternAirplane, cardutil.PatternAirplaneWings:
		return 6 // 拆飞机代价=6分
	case cardutil.PatternTripleOne, cardutil.PatternTripleTwo:
		return 2 // 拆三带代价=2分
	default:
		return 0 // 其他牌型无拆牌代价
	}
}

// evaluatePatternValue 评估牌型价值
func (ai *AIEngine) evaluatePatternValue(cards []cardutil.Card, result cardutil.PlayResult) int {
	if !result.Valid {
		return 0
	}

	value := 0

	switch result.Pattern {
	case cardutil.PatternRocket:
		// 王炸价值=100分
		value = 100

	case cardutil.PatternBomb:
		// 炸弹价值=50分 + 牌值
		value = 50 + int(result.Main)

	case cardutil.PatternSingle, cardutil.PatternPair, cardutil.PatternTriple:
		// 大牌(2、王)价值=牌值*5,小牌价值=牌值
		if result.Main >= cardutil.CardValue2 {
			value = int(result.Main) * 5
		} else {
			value = int(result.Main)
		}

	case cardutil.PatternTripleOne, cardutil.PatternTripleTwo:
		// 三带:基础价值=牌值*2
		value = int(result.Main) * 2

	case cardutil.PatternStraight:
		// 顺子:基础价值=主牌值 + 长度加成
		value = int(result.Main) + result.Length

	case cardutil.PatternStraightPair:
		// 连对:基础价值=主牌值*2 + 长度加成
		value = int(result.Main)*2 + result.Length

	case cardutil.PatternAirplane, cardutil.PatternAirplaneWings:
		// 飞机:基础价值=主牌值*3 + 长度加成
		value = int(result.Main)*3 + result.Length

	case cardutil.PatternFourTwo:
		// 四带二:基础价值=40 + 牌值
		value = 40 + int(result.Main)

	default:
		// 其他牌型:基础价值=牌值
		value = int(result.Main)
	}

	return value
}

// findQuickClearPlays 查找快速清场牌型
// 如果剩余牌数<=5，尝试找到能一手出完的牌型
func (ai *AIEngine) findQuickClearPlays(cards []cardutil.Card, combos []CardCombo) []cardutil.Card {
	if len(cards) == 0 {
		return nil
	}

	// 如果剩余牌数<=5，检查是否能一手出完
	if len(cards) <= 5 && ai.canPlayAllAtOnce(cards) {
		return cards
	}

	// 如果剩余牌数<=8，尝试找到能快速清场的组合（剩余牌<=2）
	if len(cards) <= 8 {
		// 按牌数从大到小排序组合，优先出大牌型
		sortedCombos := make([]CardCombo, len(combos))
		copy(sortedCombos, combos)

		// 按牌数降序排序
		for i := 0; i < len(sortedCombos); i++ {
			for j := i + 1; j < len(sortedCombos); j++ {
				if len(sortedCombos[i].Cards) < len(sortedCombos[j].Cards) {
					sortedCombos[i], sortedCombos[j] = sortedCombos[j], sortedCombos[i]
				}
			}
		}

		// 查找能快速清场的组合
		for _, combo := range sortedCombos {
			remaining := ai.removeCards(cards, combo.Cards)
			if len(remaining) == 0 {
				// 能一手清场
				return combo.Cards
			}
			if len(remaining) <= 2 && ai.canPlayAllAtOnce(remaining) {
				// 剩余牌能一手出完
				return combo.Cards
			}
		}
	}

	return nil
}

// shouldReserveBigCards 判断是否应该保留大牌
func (ai *AIEngine) shouldReserveBigCards(ctx *AIContext) bool {
	// 游戏前期（剩余牌>10张）：保留大牌
	if len(ctx.MyCards) > 10 {
		return true
	}

	// 检查是否有炸弹或王炸
	hasBomb := false
	hasRocket := false

	// 统计炸弹
	valueGroups := groupByValue(ctx.MyCards)
	for _, group := range valueGroups {
		if len(group) == 4 {
			hasBomb = true
			break
		}
	}

	// 检查王炸
	rocket := ai.findRocket(ctx.MyCards)
	if rocket != nil {
		hasRocket = true
	}

	// 有炸弹/王炸：保留用于关键时刻
	if hasBomb || hasRocket {
		// 如果剩余牌数还比较多，保留炸弹
		if len(ctx.MyCards) > 6 {
			return true
		}
	}

	// 地主先手：保留大牌压制
	if ctx.MyRole == types.RoleLandlord && len(ctx.MyCards) > 8 {
		return true
	}

	return false
}

// findSmallCardPlay 查找小牌出牌（避免出大牌）
func (ai *AIEngine) findSmallCardPlay(cards []cardutil.Card, combos []CardCombo) []cardutil.Card {
	if len(combos) == 0 {
		return nil
	}

	// 定义小牌阈值（小于10的牌）
	smallThreshold := cardutil.CardValue10

	// 筛选只包含小牌的组合
	smallCombos := make([]CardCombo, 0)
	for _, combo := range combos {
		allSmall := true
		for _, c := range combo.Cards {
			if c.Value >= smallThreshold {
				allSmall = false
				break
			}
		}
		if allSmall {
			smallCombos = append(smallCombos, combo)
		}
	}

	// 如果有小牌组合，选择最优的
	if len(smallCombos) > 0 {
		return ai.findBestFreePlay(cards, smallCombos)
	}

	// 如果没有小牌，选择拆牌代价最小的组合
	return ai.findBestFreePlay(cards, combos)
}

// findBestFreePlay 评估拆牌代价，选择最优出牌
func (ai *AIEngine) findBestFreePlay(cards []cardutil.Card, combos []CardCombo) []cardutil.Card {
	if len(combos) == 0 {
		return nil
	}

	type ComboScore struct {
		Combo     []cardutil.Card
		Score     float64
		BreakCost int
	}

	scores := make([]ComboScore, 0, len(combos))

	for _, combo := range combos {
		// 计算拆牌代价
		breakCost := ai.evaluateBreakCost(cards, combo.Cards)

		// 计算牌型得分（优先出小牌、优先出大牌型）
		comboResult := cardutil.AnalyzePattern(combo.Cards)
		score := 0.0

		// 牌数越多越好（快速减少手牌）
		score += float64(len(combo.Cards)) * 2.0

		// 牌值越小越好
		if comboResult.Valid {
			score -= float64(comboResult.Main) * 0.5
		}

		// 拆牌代价越小越好
		score -= float64(breakCost) * 1.5

		scores = append(scores, ComboScore{
			Combo:     combo.Cards,
			Score:     score,
			BreakCost: breakCost,
		})
	}

	// 按得分降序排序
	for i := 0; i < len(scores); i++ {
		for j := i + 1; j < len(scores); j++ {
			if scores[i].Score < scores[j].Score {
				scores[i], scores[j] = scores[j], scores[i]
			}
		}
	}

	// 返回得分最高的组合
	if len(scores) > 0 {
		return scores[0].Combo
	}

	return nil
}

// evaluateBreakCost 评估拆牌代价
// 返回值：代价越高，越不应该拆
func (ai *AIEngine) evaluateBreakCost(cards []cardutil.Card, toPlay []cardutil.Card) int {
	remaining := ai.removeCards(cards, toPlay)

	if len(remaining) == 0 {
		return 0 // 清场，无代价
	}

	// 检查剩余牌是否能组成完整牌型
	remainingResult := cardutil.AnalyzePattern(remaining)
	if remainingResult.Valid {
		// 剩余牌能一手出完，代价较低
		return 1
	}

	// 检查拆牌前是否有完整牌型
	originalResult := cardutil.AnalyzePattern(cards)

	// 如果原本是完整牌型，拆牌代价高
	if originalResult.Valid {
		return 10
	}

	// 评估剩余牌的结构完整性
	cost := 0

	// 统计剩余牌的牌型结构
	valueGroups := groupByValue(remaining)

	// 检查是否有孤立的牌（单张）
	isolatedCards := 0
	for _, group := range valueGroups {
		if len(group) == 1 {
			isolatedCards++
		}
	}

	// 孤立牌越多，代价越高
	cost += isolatedCards * 2

	// 如果剩余牌数很少，代价较低
	if len(remaining) <= 3 {
		cost = cost / 2
	}

	return cost
}
