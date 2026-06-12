package ai

import (
	"testing"

	"go-zero-ddz/pkg/cardutil"
	"go-zero-ddz/pkg/types"
)

func TestNewAIEngine(t *testing.T) {
	config := &AIConfig{
		RememberCards: true,
		UseBomb:       true,
		Strategy:      "optimal",
		ResponseRate:  1.0,
		DelayMsMin:    100,
		DelayMsMax:    500,
	}

	engine := NewAIEngine(config)
	if engine == nil {
		t.Error("NewAIEngine returned nil")
	}
	if engine.config != config {
		t.Error("Engine config mismatch")
	}
}

func TestDecidePlay_FreePlay(t *testing.T) {
	engine := NewAIEngine(&AIConfig{})

	// 测试自由出牌场景
	ctx := &AIContext{
		MyCards: []cardutil.Card{
			{Value: cardutil.CardValue3, Suit: cardutil.CardSuitSpade},
			{Value: cardutil.CardValue4, Suit: cardutil.CardSuitHeart},
			{Value: cardutil.CardValue5, Suit: cardutil.CardSuitDiamond},
		},
		LastPlay:   nil,
		MyRole:     types.RolePeasant,
		Difficulty: "normal",
	}

	decision := engine.DecidePlay(ctx)

	if decision.Action != ActionPlay {
		t.Errorf("Expected ActionPlay, got %v", decision.Action)
	}
	if len(decision.Cards) != 1 {
		t.Errorf("Expected 1 card, got %d", len(decision.Cards))
	}
	if decision.Cards[0].Value != cardutil.CardValue3 {
		t.Errorf("Expected smallest card (3), got %v", decision.Cards[0].Value)
	}
}

func TestDecidePlay_FreePlay_CanPlayAll(t *testing.T) {
	engine := NewAIEngine(&AIConfig{})

	// 测试可以一手出完的场景（三个3）
	ctx := &AIContext{
		MyCards: []cardutil.Card{
			{Value: cardutil.CardValue3, Suit: cardutil.CardSuitSpade},
			{Value: cardutil.CardValue3, Suit: cardutil.CardSuitHeart},
			{Value: cardutil.CardValue3, Suit: cardutil.CardSuitDiamond},
		},
		LastPlay:   nil,
		MyRole:     types.RolePeasant,
		Difficulty: "normal",
	}

	decision := engine.DecidePlay(ctx)

	if decision.Action != ActionPlay {
		t.Errorf("Expected ActionPlay, got %v", decision.Action)
	}
	if len(decision.Cards) != 3 {
		t.Errorf("Expected 3 cards (triple), got %d", len(decision.Cards))
	}
}

func TestDecidePlay_ResponsePlay_CanBeat(t *testing.T) {
	engine := NewAIEngine(&AIConfig{UseBomb: false})

	// 上家出了一张3，我有4可以管
	ctx := &AIContext{
		MyCards: []cardutil.Card{
			{Value: cardutil.CardValue4, Suit: cardutil.CardSuitSpade},
			{Value: cardutil.CardValue5, Suit: cardutil.CardSuitHeart},
		},
		LastPlay: &LastPlayInfo{
			Cards:   []cardutil.Card{{Value: cardutil.CardValue3, Suit: cardutil.CardSuitSpade}},
			Pattern: cardutil.PatternSingle,
			Main:    cardutil.CardValue3,
			UID:     "opponent1",
		},
		MyRole:     types.RolePeasant,
		Difficulty: "normal",
	}

	decision := engine.DecidePlay(ctx)

	if decision.Action != ActionPlay {
		t.Errorf("Expected ActionPlay, got %v", decision.Action)
	}
	if len(decision.Cards) != 1 {
		t.Errorf("Expected 1 card, got %d", len(decision.Cards))
	}
	if decision.Cards[0].Value != cardutil.CardValue4 {
		t.Errorf("Expected smallest beating card (4), got %v", decision.Cards[0].Value)
	}
}

func TestDecidePlay_ResponsePlay_CannotBeat(t *testing.T) {
	engine := NewAIEngine(&AIConfig{UseBomb: false})

	// 上家出了一张K，我只有3和4，无法管
	ctx := &AIContext{
		MyCards: []cardutil.Card{
			{Value: cardutil.CardValue3, Suit: cardutil.CardSuitSpade},
			{Value: cardutil.CardValue4, Suit: cardutil.CardSuitHeart},
		},
		LastPlay: &LastPlayInfo{
			Cards:   []cardutil.Card{{Value: cardutil.CardValueK, Suit: cardutil.CardSuitSpade}},
			Pattern: cardutil.PatternSingle,
			Main:    cardutil.CardValueK,
			UID:     "opponent1",
		},
		MyRole:     types.RolePeasant,
		Difficulty: "normal",
	}

	decision := engine.DecidePlay(ctx)

	if decision.Action != ActionPass {
		t.Errorf("Expected ActionPass, got %v", decision.Action)
	}
}

func TestDecidePlay_ResponsePlay_Bomb(t *testing.T) {
	engine := NewAIEngine(&AIConfig{UseBomb: true})

	// 上家出了一张3，我有炸弹和小牌
	ctx := &AIContext{
		MyCards: []cardutil.Card{
			{Value: cardutil.CardValue3, Suit: cardutil.CardSuitSpade},
			{Value: cardutil.CardValue3, Suit: cardutil.CardSuitHeart},
			{Value: cardutil.CardValue3, Suit: cardutil.CardSuitDiamond},
			{Value: cardutil.CardValue3, Suit: cardutil.CardSuitClub},
			{Value: cardutil.CardValue4, Suit: cardutil.CardSuitSpade},
		},
		LastPlay: &LastPlayInfo{
			Cards:   []cardutil.Card{{Value: cardutil.CardValue2, Suit: cardutil.CardSuitSpade}},
			Pattern: cardutil.PatternSingle,
			Main:    cardutil.CardValue2,
			UID:     "opponent1",
		},
		MyRole:     types.RoleLandlord,
		Difficulty: "normal",
	}

	decision := engine.DecidePlay(ctx)

	if decision.Action != ActionPlay {
		t.Errorf("Expected ActionPlay, got %v", decision.Action)
	}
	// 应该用炸弹打2
	result := cardutil.AnalyzePattern(decision.Cards)
	if !result.Pattern.IsBomb() {
		t.Errorf("Expected bomb, got %v", result.Pattern)
	}
}

func TestShouldPassToTeammate(t *testing.T) {
	engine := NewAIEngine(&AIConfig{})

	// 农民，队友出牌且剩3张以下
	ctx := &AIContext{
		MyRole:        types.RolePeasant,
		LastPlayerUID: "teammate",
		Players: map[string]*PlayerInfo{
			"teammate": {UID: "teammate", IsLandlord: false, CardCount: 2},
			"opponent": {UID: "opponent", IsLandlord: true, CardCount: 10},
		},
	}

	result := engine.shouldPassToTeammate(ctx)
	if !result {
		t.Error("Expected shouldPassToTeammate to return true")
	}

	// 地主没有队友
	ctx.MyRole = types.RoleLandlord
	result = engine.shouldPassToTeammate(ctx)
	if result {
		t.Error("Expected shouldPassToTeammate to return false for landlord")
	}

	// 队友剩很多牌
	ctx.MyRole = types.RolePeasant
	ctx.Players["teammate"].CardCount = 10
	result = engine.shouldPassToTeammate(ctx)
	if result {
		t.Error("Expected shouldPassToTeammate to return false when teammate has many cards")
	}
}

func TestIsHandWeak(t *testing.T) {
	engine := NewAIEngine(&AIConfig{})

	// 没有大牌（3-10），至少4张牌
	cards := []cardutil.Card{
		{Value: cardutil.CardValue3, Suit: cardutil.CardSuitSpade},
		{Value: cardutil.CardValue5, Suit: cardutil.CardSuitHeart},
		{Value: cardutil.CardValue10, Suit: cardutil.CardSuitDiamond},
		{Value: cardutil.CardValue7, Suit: cardutil.CardSuitClub},
	}
	result := engine.isHandWeak(cards)
	if !result {
		t.Error("Expected isHandWeak to return true for weak hand")
	}

	// 有大牌（A和2），总分应该>=5，不算弱
	cards = []cardutil.Card{
		{Value: cardutil.CardValueA, Suit: cardutil.CardSuitSpade},
		{Value: cardutil.CardValue2, Suit: cardutil.CardSuitHeart},
		{Value: cardutil.CardValue3, Suit: cardutil.CardSuitDiamond},
		{Value: cardutil.CardValue4, Suit: cardutil.CardSuitClub},
		{Value: cardutil.CardValue5, Suit: cardutil.CardSuitSpade},
	}
	result = engine.isHandWeak(cards)
	if result {
		t.Error("Expected isHandWeak to return false when has big cards (A+2)")
	}

	// 牌太少（<=3张）不算弱
	cards = []cardutil.Card{
		{Value: cardutil.CardValue3, Suit: cardutil.CardSuitSpade},
		{Value: cardutil.CardValue4, Suit: cardutil.CardSuitHeart},
	}
	result = engine.isHandWeak(cards)
	if result {
		t.Error("Expected isHandWeak to return false for small hand (<=3 cards)")
	}
}

func TestCanPlayAllAtOnce(t *testing.T) {
	engine := NewAIEngine(&AIConfig{})

	// 三条可以一手出完
	cards := []cardutil.Card{
		{Value: cardutil.CardValue3, Suit: cardutil.CardSuitSpade},
		{Value: cardutil.CardValue3, Suit: cardutil.CardSuitHeart},
		{Value: cardutil.CardValue3, Suit: cardutil.CardSuitDiamond},
	}
	result := engine.canPlayAllAtOnce(cards)
	if !result {
		t.Error("Expected canPlayAllAtOnce to return true for triple")
	}

	// 乱牌不能一手出完
	cards = []cardutil.Card{
		{Value: cardutil.CardValue3, Suit: cardutil.CardSuitSpade},
		{Value: cardutil.CardValue5, Suit: cardutil.CardSuitHeart},
		{Value: cardutil.CardValue7, Suit: cardutil.CardSuitDiamond},
	}
	result = engine.canPlayAllAtOnce(cards)
	if result {
		t.Error("Expected canPlayAllAtOnce to return false for random cards")
	}
}

func TestFindSmallestSingle(t *testing.T) {
	engine := NewAIEngine(&AIConfig{})

	cards := []cardutil.Card{
		{Value: cardutil.CardValue5, Suit: cardutil.CardSuitSpade},
		{Value: cardutil.CardValue3, Suit: cardutil.CardSuitHeart},
		{Value: cardutil.CardValue7, Suit: cardutil.CardSuitDiamond},
	}

	result := engine.findSmallestSingle(cards)
	if result == nil {
		t.Error("Expected non-nil result")
	} else if result.Value != cardutil.CardValue3 {
		t.Errorf("Expected smallest card (3), got %v", result.Value)
	}
}

func TestFindSmallestPair(t *testing.T) {
	engine := NewAIEngine(&AIConfig{})

	// 有对子
	cards := []cardutil.Card{
		{Value: cardutil.CardValue3, Suit: cardutil.CardSuitSpade},
		{Value: cardutil.CardValue3, Suit: cardutil.CardSuitHeart},
		{Value: cardutil.CardValue5, Suit: cardutil.CardSuitDiamond},
	}

	result := engine.findSmallestPair(cards)
	if result == nil {
		t.Error("Expected non-nil result")
	} else if len(result) != 2 {
		t.Errorf("Expected 2 cards, got %d", len(result))
	}

	// 没有对子
	cards = []cardutil.Card{
		{Value: cardutil.CardValue3, Suit: cardutil.CardSuitSpade},
		{Value: cardutil.CardValue5, Suit: cardutil.CardSuitHeart},
		{Value: cardutil.CardValue7, Suit: cardutil.CardSuitDiamond},
	}

	result = engine.findSmallestPair(cards)
	if result != nil {
		t.Error("Expected nil result for no pair")
	}
}

func TestFindSmallestBomb(t *testing.T) {
	engine := NewAIEngine(&AIConfig{})

	// 有炸弹
	cards := []cardutil.Card{
		{Value: cardutil.CardValue3, Suit: cardutil.CardSuitSpade},
		{Value: cardutil.CardValue3, Suit: cardutil.CardSuitHeart},
		{Value: cardutil.CardValue3, Suit: cardutil.CardSuitDiamond},
		{Value: cardutil.CardValue3, Suit: cardutil.CardSuitClub},
		{Value: cardutil.CardValue5, Suit: cardutil.CardSuitSpade},
	}

	result := engine.findSmallestBomb(cards)
	if result == nil {
		t.Error("Expected non-nil result")
	} else if len(result) != 4 {
		t.Errorf("Expected 4 cards, got %d", len(result))
	}

	// 没有炸弹
	cards = []cardutil.Card{
		{Value: cardutil.CardValue3, Suit: cardutil.CardSuitSpade},
		{Value: cardutil.CardValue3, Suit: cardutil.CardSuitHeart},
		{Value: cardutil.CardValue3, Suit: cardutil.CardSuitDiamond},
		{Value: cardutil.CardValue5, Suit: cardutil.CardSuitSpade},
	}

	result = engine.findSmallestBomb(cards)
	if result != nil {
		t.Error("Expected nil result for no bomb")
	}
}

func TestFilterBeatingCombos(t *testing.T) {
	engine := NewAIEngine(&AIConfig{})

	lastPlay := &LastPlayInfo{
		Cards:   []cardutil.Card{{Value: cardutil.CardValue3, Suit: cardutil.CardSuitSpade}},
		Pattern: cardutil.PatternSingle,
		Main:    cardutil.CardValue3,
		UID:     "opponent",
	}

	combos := []CardCombo{
		{Cards: []cardutil.Card{{Value: cardutil.CardValue2, Suit: cardutil.CardSuitSpade}}},
		{Cards: []cardutil.Card{{Value: cardutil.CardValue4, Suit: cardutil.CardSuitHeart}}},
		{Cards: []cardutil.Card{{Value: cardutil.CardValue3, Suit: cardutil.CardSuitHeart}}},
	}

	result := engine.filterBeatingCombos(combos, lastPlay)
	if len(result) != 2 {
		t.Errorf("Expected 2 beating combos, got %d", len(result))
	}
}

func TestDifficultyConfig(t *testing.T) {
	// 测试不同难度配置
	config := getDifficultyConfig("easy")
	if config.UseBomb {
		t.Error("Easy mode should not use bomb")
	}
	if config.ResponseRate != 0.5 {
		t.Errorf("Easy mode response rate should be 0.5, got %v", config.ResponseRate)
	}

	config = getDifficultyConfig("normal")
	if !config.UseBomb {
		t.Error("Normal mode should use bomb")
	}
	if config.ResponseRate != 0.8 {
		t.Errorf("Normal mode response rate should be 0.8, got %v", config.ResponseRate)
	}

	config = getDifficultyConfig("hard")
	if !config.UseBomb {
		t.Error("Hard mode should use bomb")
	}
	if !config.RememberCards {
		t.Error("Hard mode should remember cards")
	}
	if !config.CardInference {
		t.Error("Hard mode should have card inference")
	}
}

// ============================================
// 牌型枚举函数测试
// ============================================

func TestFindRocket(t *testing.T) {
	engine := NewAIEngine(&AIConfig{})

	// 测试有王炸的情况
	cards := []cardutil.Card{
		{Value: cardutil.CardValueJokerS, Suit: cardutil.CardSuitJoker},
		{Value: cardutil.CardValueJokerB, Suit: cardutil.CardSuitJoker},
		{Value: cardutil.CardValue3, Suit: cardutil.CardSuitSpade},
	}
	result := engine.findRocket(cards)
	if result == nil {
		t.Error("Expected to find rocket")
	} else if len(result) != 2 {
		t.Errorf("Expected 2 cards in rocket, got %d", len(result))
	} else {
		// 验证是否包含大小王
		hasSmallJoker := false
		hasBigJoker := false
		for _, c := range result {
			if c.Value == cardutil.CardValueJokerS {
				hasSmallJoker = true
			}
			if c.Value == cardutil.CardValueJokerB {
				hasBigJoker = true
			}
		}
		if !hasSmallJoker || !hasBigJoker {
			t.Error("Rocket should contain both jokers")
		}
	}

	// 测试无王炸的情况（只有小王）
	cards = []cardutil.Card{
		{Value: cardutil.CardValueJokerS, Suit: cardutil.CardSuitJoker},
		{Value: cardutil.CardValue3, Suit: cardutil.CardSuitSpade},
	}
	result = engine.findRocket(cards)
	if result != nil {
		t.Error("Expected nil when no rocket available")
	}

	// 测试无王炸的情况（只有大王）
	cards = []cardutil.Card{
		{Value: cardutil.CardValueJokerB, Suit: cardutil.CardSuitJoker},
		{Value: cardutil.CardValue3, Suit: cardutil.CardSuitSpade},
	}
	result = engine.findRocket(cards)
	if result != nil {
		t.Error("Expected nil when no rocket available")
	}

	// 测试无王炸的情况（都没有）
	cards = []cardutil.Card{
		{Value: cardutil.CardValue3, Suit: cardutil.CardSuitSpade},
		{Value: cardutil.CardValue4, Suit: cardutil.CardSuitHeart},
	}
	result = engine.findRocket(cards)
	if result != nil {
		t.Error("Expected nil when no jokers available")
	}
}

func TestFindStraights(t *testing.T) {
	engine := NewAIEngine(&AIConfig{})

	// 测试5张顺子（3-7）
	cards := []cardutil.Card{
		{Value: cardutil.CardValue3, Suit: cardutil.CardSuitSpade},
		{Value: cardutil.CardValue4, Suit: cardutil.CardSuitHeart},
		{Value: cardutil.CardValue5, Suit: cardutil.CardSuitDiamond},
		{Value: cardutil.CardValue6, Suit: cardutil.CardSuitClub},
		{Value: cardutil.CardValue7, Suit: cardutil.CardSuitSpade},
	}
	valueGroups := groupByValue(cards)
	combos := engine.findStraights(cards, valueGroups)

	if len(combos) == 0 {
		t.Error("Expected to find at least one straight")
	} else {
		// 验证顺子长度为5
		if len(combos[0].Cards) != 5 {
			t.Errorf("Expected straight length 5, got %d", len(combos[0].Cards))
		}
		// 验证牌型
		result := cardutil.AnalyzePattern(combos[0].Cards)
		if result.Pattern != cardutil.PatternStraight {
			t.Errorf("Expected PatternStraight, got %v", result.Pattern)
		}
	}

	// 测试6张顺子（3-8）
	cards = []cardutil.Card{
		{Value: cardutil.CardValue3, Suit: cardutil.CardSuitSpade},
		{Value: cardutil.CardValue4, Suit: cardutil.CardSuitHeart},
		{Value: cardutil.CardValue5, Suit: cardutil.CardSuitDiamond},
		{Value: cardutil.CardValue6, Suit: cardutil.CardSuitClub},
		{Value: cardutil.CardValue7, Suit: cardutil.CardSuitSpade},
		{Value: cardutil.CardValue8, Suit: cardutil.CardSuitHeart},
	}
	valueGroups = groupByValue(cards)
	combos = engine.findStraights(cards, valueGroups)

	if len(combos) == 0 {
		t.Error("Expected to find straights")
	} else {
		// 应该找到两个顺子：3-7（5张）和3-8（6张）
		found5 := false
		found6 := false
		for _, combo := range combos {
			if len(combo.Cards) == 5 {
				found5 = true
			}
			if len(combo.Cards) == 6 {
				found6 = true
			}
		}
		if !found5 {
			t.Error("Expected to find 5-card straight")
		}
		if !found6 {
			t.Error("Expected to find 6-card straight")
		}
	}

	// 测试7张顺子（3-9）
	cards = []cardutil.Card{
		{Value: cardutil.CardValue3, Suit: cardutil.CardSuitSpade},
		{Value: cardutil.CardValue4, Suit: cardutil.CardSuitHeart},
		{Value: cardutil.CardValue5, Suit: cardutil.CardSuitDiamond},
		{Value: cardutil.CardValue6, Suit: cardutil.CardSuitClub},
		{Value: cardutil.CardValue7, Suit: cardutil.CardSuitSpade},
		{Value: cardutil.CardValue8, Suit: cardutil.CardSuitHeart},
		{Value: cardutil.CardValue9, Suit: cardutil.CardSuitDiamond},
	}
	valueGroups = groupByValue(cards)
	combos = engine.findStraights(cards, valueGroups)

	if len(combos) == 0 {
		t.Error("Expected to find straights")
	} else {
		// 应该找到三个顺子：5张、6张、7张
		found5 := false
		found6 := false
		found7 := false
		for _, combo := range combos {
			if len(combo.Cards) == 5 {
				found5 = true
			}
			if len(combo.Cards) == 6 {
				found6 = true
			}
			if len(combo.Cards) == 7 {
				found7 = true
			}
		}
		if !found5 || !found6 || !found7 {
			t.Errorf("Expected to find 5, 6, 7-card straights, found: %d combos", len(combos))
		}
	}

	// 测试不含2和王的顺子
	cards = []cardutil.Card{
		{Value: cardutil.CardValue3, Suit: cardutil.CardSuitSpade},
		{Value: cardutil.CardValue4, Suit: cardutil.CardSuitHeart},
		{Value: cardutil.CardValue5, Suit: cardutil.CardSuitDiamond},
		{Value: cardutil.CardValue6, Suit: cardutil.CardSuitClub},
		{Value: cardutil.CardValue7, Suit: cardutil.CardSuitSpade},
		{Value: cardutil.CardValue2, Suit: cardutil.CardSuitHeart}, // 2不应该参与顺子
		{Value: cardutil.CardValueJokerS, Suit: cardutil.CardSuitJoker}, // 王不应该参与顺子
	}
	valueGroups = groupByValue(cards)
	combos = engine.findStraights(cards, valueGroups)

	if len(combos) == 0 {
		t.Error("Expected to find straights excluding 2 and jokers")
	} else {
		// 验证所有顺子都不包含2和王
		for _, combo := range combos {
			for _, c := range combo.Cards {
				if c.Value == cardutil.CardValue2 || c.Value == cardutil.CardValueJokerS || c.Value == cardutil.CardValueJokerB {
					t.Error("Straight should not contain 2 or jokers")
				}
			}
		}
	}

	// 测试不连续的牌（无法组成顺子）
	cards = []cardutil.Card{
		{Value: cardutil.CardValue3, Suit: cardutil.CardSuitSpade},
		{Value: cardutil.CardValue5, Suit: cardutil.CardSuitHeart},
		{Value: cardutil.CardValue7, Suit: cardutil.CardSuitDiamond},
		{Value: cardutil.CardValue9, Suit: cardutil.CardSuitClub},
		{Value: cardutil.CardValueJ, Suit: cardutil.CardSuitSpade},
	}
	valueGroups = groupByValue(cards)
	combos = engine.findStraights(cards, valueGroups)

	if len(combos) != 0 {
		t.Errorf("Expected no straights for non-consecutive cards, got %d", len(combos))
	}

	// 测试牌数不足（少于5张）
	cards = []cardutil.Card{
		{Value: cardutil.CardValue3, Suit: cardutil.CardSuitSpade},
		{Value: cardutil.CardValue4, Suit: cardutil.CardSuitHeart},
		{Value: cardutil.CardValue5, Suit: cardutil.CardSuitDiamond},
		{Value: cardutil.CardValue6, Suit: cardutil.CardSuitClub},
	}
	valueGroups = groupByValue(cards)
	combos = engine.findStraights(cards, valueGroups)

	if len(combos) != 0 {
		t.Errorf("Expected no straights for less than 5 cards, got %d", len(combos))
	}
}

func TestFindDoubleStraights(t *testing.T) {
	engine := NewAIEngine(&AIConfig{})

	// 测试3对连对（33-44-55）
	cards := []cardutil.Card{
		{Value: cardutil.CardValue3, Suit: cardutil.CardSuitSpade},
		{Value: cardutil.CardValue3, Suit: cardutil.CardSuitHeart},
		{Value: cardutil.CardValue4, Suit: cardutil.CardSuitDiamond},
		{Value: cardutil.CardValue4, Suit: cardutil.CardSuitClub},
		{Value: cardutil.CardValue5, Suit: cardutil.CardSuitSpade},
		{Value: cardutil.CardValue5, Suit: cardutil.CardSuitHeart},
	}
	valueGroups := groupByValue(cards)
	combos := engine.findDoubleStraights(cards, valueGroups)

	if len(combos) == 0 {
		t.Error("Expected to find double straight")
	} else {
		// 验证连对长度为6（3对）
		if len(combos[0].Cards) != 6 {
			t.Errorf("Expected double straight length 6, got %d", len(combos[0].Cards))
		}
		// 验证牌型
		result := cardutil.AnalyzePattern(combos[0].Cards)
		if result.Pattern != cardutil.PatternStraightPair {
			t.Errorf("Expected PatternStraightPair, got %v", result.Pattern)
		}
	}

	// 测试4对连对（33-44-55-66）
	cards = []cardutil.Card{
		{Value: cardutil.CardValue3, Suit: cardutil.CardSuitSpade},
		{Value: cardutil.CardValue3, Suit: cardutil.CardSuitHeart},
		{Value: cardutil.CardValue4, Suit: cardutil.CardSuitDiamond},
		{Value: cardutil.CardValue4, Suit: cardutil.CardSuitClub},
		{Value: cardutil.CardValue5, Suit: cardutil.CardSuitSpade},
		{Value: cardutil.CardValue5, Suit: cardutil.CardSuitHeart},
		{Value: cardutil.CardValue6, Suit: cardutil.CardSuitDiamond},
		{Value: cardutil.CardValue6, Suit: cardutil.CardSuitClub},
	}
	valueGroups = groupByValue(cards)
	combos = engine.findDoubleStraights(cards, valueGroups)

	if len(combos) == 0 {
		t.Error("Expected to find double straights")
	} else {
		// 应该找到两个连对：3对和4对
		found3Pairs := false
		found4Pairs := false
		for _, combo := range combos {
			if len(combo.Cards) == 6 {
				found3Pairs = true
			}
			if len(combo.Cards) == 8 {
				found4Pairs = true
			}
		}
		if !found3Pairs {
			t.Error("Expected to find 3-pair double straight")
		}
		if !found4Pairs {
			t.Error("Expected to find 4-pair double straight")
		}
	}

	// 测试对子数不足（少于3对）
	cards = []cardutil.Card{
		{Value: cardutil.CardValue3, Suit: cardutil.CardSuitSpade},
		{Value: cardutil.CardValue3, Suit: cardutil.CardSuitHeart},
		{Value: cardutil.CardValue4, Suit: cardutil.CardSuitDiamond},
		{Value: cardutil.CardValue4, Suit: cardutil.CardSuitClub},
	}
	valueGroups = groupByValue(cards)
	combos = engine.findDoubleStraights(cards, valueGroups)

	if len(combos) != 0 {
		t.Errorf("Expected no double straights for less than 3 pairs, got %d", len(combos))
	}

	// 测试不连续的对子
	cards = []cardutil.Card{
		{Value: cardutil.CardValue3, Suit: cardutil.CardSuitSpade},
		{Value: cardutil.CardValue3, Suit: cardutil.CardSuitHeart},
		{Value: cardutil.CardValue5, Suit: cardutil.CardSuitDiamond},
		{Value: cardutil.CardValue5, Suit: cardutil.CardSuitClub},
		{Value: cardutil.CardValue7, Suit: cardutil.CardSuitSpade},
		{Value: cardutil.CardValue7, Suit: cardutil.CardSuitHeart},
	}
	valueGroups = groupByValue(cards)
	combos = engine.findDoubleStraights(cards, valueGroups)

	if len(combos) != 0 {
		t.Errorf("Expected no double straights for non-consecutive pairs, got %d", len(combos))
	}
}

func TestFindThreeWithOne(t *testing.T) {
	engine := NewAIEngine(&AIConfig{})

	// 测试三带一（333+4）
	cards := []cardutil.Card{
		{Value: cardutil.CardValue3, Suit: cardutil.CardSuitSpade},
		{Value: cardutil.CardValue3, Suit: cardutil.CardSuitHeart},
		{Value: cardutil.CardValue3, Suit: cardutil.CardSuitDiamond},
		{Value: cardutil.CardValue4, Suit: cardutil.CardSuitClub},
	}
	valueGroups := groupByValue(cards)
	combos := engine.findThreeWithOne(cards, valueGroups)

	if len(combos) == 0 {
		t.Error("Expected to find three-with-one")
	} else {
		// 验证长度为4
		if len(combos[0].Cards) != 4 {
			t.Errorf("Expected 4 cards, got %d", len(combos[0].Cards))
		}
		// 验证牌型
		result := cardutil.AnalyzePattern(combos[0].Cards)
		if result.Pattern != cardutil.PatternTripleOne {
			t.Errorf("Expected PatternTripleOne, got %v", result.Pattern)
		}
	}

	// 测试多个三带一组合
	cards = []cardutil.Card{
		{Value: cardutil.CardValue3, Suit: cardutil.CardSuitSpade},
		{Value: cardutil.CardValue3, Suit: cardutil.CardSuitHeart},
		{Value: cardutil.CardValue3, Suit: cardutil.CardSuitDiamond},
		{Value: cardutil.CardValue4, Suit: cardutil.CardSuitClub},
		{Value: cardutil.CardValue5, Suit: cardutil.CardSuitSpade},
		{Value: cardutil.CardValue5, Suit: cardutil.CardSuitHeart},
		{Value: cardutil.CardValue5, Suit: cardutil.CardSuitDiamond},
		{Value: cardutil.CardValue6, Suit: cardutil.CardSuitClub},
	}
	valueGroups = groupByValue(cards)
	combos = engine.findThreeWithOne(cards, valueGroups)

	if len(combos) < 2 {
		t.Errorf("Expected at least 2 three-with-one combos, got %d", len(combos))
	}

	// 测试四张牌作为三带一（3333可以拆成333+3）
	cards = []cardutil.Card{
		{Value: cardutil.CardValue3, Suit: cardutil.CardSuitSpade},
		{Value: cardutil.CardValue3, Suit: cardutil.CardSuitHeart},
		{Value: cardutil.CardValue3, Suit: cardutil.CardSuitDiamond},
		{Value: cardutil.CardValue3, Suit: cardutil.CardSuitClub},
	}
	valueGroups = groupByValue(cards)
	combos = engine.findThreeWithOne(cards, valueGroups)

	if len(combos) == 0 {
		t.Error("Expected to find three-with-one from four-of-a-kind")
	}

	// 测试无三带一的情况
	cards = []cardutil.Card{
		{Value: cardutil.CardValue3, Suit: cardutil.CardSuitSpade},
		{Value: cardutil.CardValue3, Suit: cardutil.CardSuitHeart},
		{Value: cardutil.CardValue4, Suit: cardutil.CardSuitDiamond},
		{Value: cardutil.CardValue5, Suit: cardutil.CardSuitClub},
	}
	valueGroups = groupByValue(cards)
	combos = engine.findThreeWithOne(cards, valueGroups)

	if len(combos) != 0 {
		t.Errorf("Expected no three-with-one for no triple, got %d", len(combos))
	}
}

func TestFindThreeWithTwo(t *testing.T) {
	engine := NewAIEngine(&AIConfig{})

	// 测试三带二（333+44）
	cards := []cardutil.Card{
		{Value: cardutil.CardValue3, Suit: cardutil.CardSuitSpade},
		{Value: cardutil.CardValue3, Suit: cardutil.CardSuitHeart},
		{Value: cardutil.CardValue3, Suit: cardutil.CardSuitDiamond},
		{Value: cardutil.CardValue4, Suit: cardutil.CardSuitClub},
		{Value: cardutil.CardValue4, Suit: cardutil.CardSuitSpade},
	}
	valueGroups := groupByValue(cards)
	combos := engine.findThreeWithTwo(cards, valueGroups)

	if len(combos) == 0 {
		t.Error("Expected to find three-with-two")
	} else {
		// 验证长度为5
		if len(combos[0].Cards) != 5 {
			t.Errorf("Expected 5 cards, got %d", len(combos[0].Cards))
		}
		// 验证牌型
		result := cardutil.AnalyzePattern(combos[0].Cards)
		if result.Pattern != cardutil.PatternTripleTwo {
			t.Errorf("Expected PatternTripleTwo, got %v", result.Pattern)
		}
	}

	// 测试多个三带二组合
	cards = []cardutil.Card{
		{Value: cardutil.CardValue3, Suit: cardutil.CardSuitSpade},
		{Value: cardutil.CardValue3, Suit: cardutil.CardSuitHeart},
		{Value: cardutil.CardValue3, Suit: cardutil.CardSuitDiamond},
		{Value: cardutil.CardValue4, Suit: cardutil.CardSuitClub},
		{Value: cardutil.CardValue4, Suit: cardutil.CardSuitSpade},
		{Value: cardutil.CardValue5, Suit: cardutil.CardSuitHeart},
		{Value: cardutil.CardValue5, Suit: cardutil.CardSuitDiamond},
		{Value: cardutil.CardValue5, Suit: cardutil.CardSuitClub},
		{Value: cardutil.CardValue6, Suit: cardutil.CardSuitSpade},
		{Value: cardutil.CardValue6, Suit: cardutil.CardSuitHeart},
	}
	valueGroups = groupByValue(cards)
	combos = engine.findThreeWithTwo(cards, valueGroups)

	if len(combos) < 2 {
		t.Errorf("Expected at least 2 three-with-two combos, got %d", len(combos))
	}

	// 测试无三带二的情况（有对子无三条）
	cards = []cardutil.Card{
		{Value: cardutil.CardValue3, Suit: cardutil.CardSuitSpade},
		{Value: cardutil.CardValue3, Suit: cardutil.CardSuitHeart},
		{Value: cardutil.CardValue4, Suit: cardutil.CardSuitDiamond},
		{Value: cardutil.CardValue4, Suit: cardutil.CardSuitClub},
	}
	valueGroups = groupByValue(cards)
	combos = engine.findThreeWithTwo(cards, valueGroups)

	if len(combos) != 0 {
		t.Errorf("Expected no three-with-two for no triple, got %d", len(combos))
	}

	// 测试无三带二的情况（有三条无对子）
	cards = []cardutil.Card{
		{Value: cardutil.CardValue3, Suit: cardutil.CardSuitSpade},
		{Value: cardutil.CardValue3, Suit: cardutil.CardSuitHeart},
		{Value: cardutil.CardValue3, Suit: cardutil.CardSuitDiamond},
		{Value: cardutil.CardValue4, Suit: cardutil.CardSuitClub},
	}
	valueGroups = groupByValue(cards)
	combos = engine.findThreeWithTwo(cards, valueGroups)

	if len(combos) != 0 {
		t.Errorf("Expected no three-with-two for no pair, got %d", len(combos))
	}
}

func TestFindPlanes(t *testing.T) {
	engine := NewAIEngine(&AIConfig{})

	// 测试纯飞机（333-444）
	cards := []cardutil.Card{
		{Value: cardutil.CardValue3, Suit: cardutil.CardSuitSpade},
		{Value: cardutil.CardValue3, Suit: cardutil.CardSuitHeart},
		{Value: cardutil.CardValue3, Suit: cardutil.CardSuitDiamond},
		{Value: cardutil.CardValue4, Suit: cardutil.CardSuitClub},
		{Value: cardutil.CardValue4, Suit: cardutil.CardSuitSpade},
		{Value: cardutil.CardValue4, Suit: cardutil.CardSuitHeart},
	}
	valueGroups := groupByValue(cards)
	combos := engine.findPlanes(cards, valueGroups)

	if len(combos) == 0 {
		t.Error("Expected to find plane")
	} else {
		// 验证至少有一个纯飞机（6张）
		foundPurePlane := false
		for _, combo := range combos {
			if len(combo.Cards) == 6 {
				result := cardutil.AnalyzePattern(combo.Cards)
				if result.Pattern == cardutil.PatternAirplane {
					foundPurePlane = true
				}
			}
		}
		if !foundPurePlane {
			t.Error("Expected to find pure plane (6 cards)")
		}
	}

	// 测试飞机带单（333-444+5+6）
	cards = []cardutil.Card{
		{Value: cardutil.CardValue3, Suit: cardutil.CardSuitSpade},
		{Value: cardutil.CardValue3, Suit: cardutil.CardSuitHeart},
		{Value: cardutil.CardValue3, Suit: cardutil.CardSuitDiamond},
		{Value: cardutil.CardValue4, Suit: cardutil.CardSuitClub},
		{Value: cardutil.CardValue4, Suit: cardutil.CardSuitSpade},
		{Value: cardutil.CardValue4, Suit: cardutil.CardSuitHeart},
		{Value: cardutil.CardValue5, Suit: cardutil.CardSuitDiamond},
		{Value: cardutil.CardValue6, Suit: cardutil.CardSuitClub},
	}
	valueGroups = groupByValue(cards)
	combos = engine.findPlanes(cards, valueGroups)

	if len(combos) == 0 {
		t.Error("Expected to find planes")
	} else {
		// 验证有飞机带单（8张）
		foundPlaneWithSingle := false
		for _, combo := range combos {
			if len(combo.Cards) == 8 {
				result := cardutil.AnalyzePattern(combo.Cards)
				if result.Pattern == cardutil.PatternAirplaneWings {
					foundPlaneWithSingle = true
				}
			}
		}
		if !foundPlaneWithSingle {
			t.Error("Expected to find plane with single wings (8 cards)")
		}
	}

	// 测试飞机带对（333-444+55+66）
	cards = []cardutil.Card{
		{Value: cardutil.CardValue3, Suit: cardutil.CardSuitSpade},
		{Value: cardutil.CardValue3, Suit: cardutil.CardSuitHeart},
		{Value: cardutil.CardValue3, Suit: cardutil.CardSuitDiamond},
		{Value: cardutil.CardValue4, Suit: cardutil.CardSuitClub},
		{Value: cardutil.CardValue4, Suit: cardutil.CardSuitSpade},
		{Value: cardutil.CardValue4, Suit: cardutil.CardSuitHeart},
		{Value: cardutil.CardValue5, Suit: cardutil.CardSuitDiamond},
		{Value: cardutil.CardValue5, Suit: cardutil.CardSuitClub},
		{Value: cardutil.CardValue6, Suit: cardutil.CardSuitSpade},
		{Value: cardutil.CardValue6, Suit: cardutil.CardSuitHeart},
	}
	valueGroups = groupByValue(cards)
	combos = engine.findPlanes(cards, valueGroups)

	if len(combos) == 0 {
		t.Error("Expected to find planes")
	} else {
		// 验证有飞机带对（10张）
		foundPlaneWithPair := false
		for _, combo := range combos {
			if len(combo.Cards) == 10 {
				result := cardutil.AnalyzePattern(combo.Cards)
				if result.Pattern == cardutil.PatternAirplaneWings {
					foundPlaneWithPair = true
				}
			}
		}
		if !foundPlaneWithPair {
			t.Error("Expected to find plane with pair wings (10 cards)")
		}
	}

	// 测试无飞机的情况（少于2个连续三条）
	cards = []cardutil.Card{
		{Value: cardutil.CardValue3, Suit: cardutil.CardSuitSpade},
		{Value: cardutil.CardValue3, Suit: cardutil.CardSuitHeart},
		{Value: cardutil.CardValue3, Suit: cardutil.CardSuitDiamond},
	}
	valueGroups = groupByValue(cards)
	combos = engine.findPlanes(cards, valueGroups)

	if len(combos) != 0 {
		t.Errorf("Expected no plane for less than 2 triples, got %d", len(combos))
	}

	// 测试三条不连续的情况
	cards = []cardutil.Card{
		{Value: cardutil.CardValue3, Suit: cardutil.CardSuitSpade},
		{Value: cardutil.CardValue3, Suit: cardutil.CardSuitHeart},
		{Value: cardutil.CardValue3, Suit: cardutil.CardSuitDiamond},
		{Value: cardutil.CardValue5, Suit: cardutil.CardSuitClub},
		{Value: cardutil.CardValue5, Suit: cardutil.CardSuitSpade},
		{Value: cardutil.CardValue5, Suit: cardutil.CardSuitHeart},
	}
	valueGroups = groupByValue(cards)
	combos = engine.findPlanes(cards, valueGroups)

	if len(combos) != 0 {
		t.Errorf("Expected no plane for non-consecutive triples, got %d", len(combos))
	}
}

func TestFindFourWithTwo(t *testing.T) {
	engine := NewAIEngine(&AIConfig{})

	// 测试四带两单（3333+4+5）
	cards := []cardutil.Card{
		{Value: cardutil.CardValue3, Suit: cardutil.CardSuitSpade},
		{Value: cardutil.CardValue3, Suit: cardutil.CardSuitHeart},
		{Value: cardutil.CardValue3, Suit: cardutil.CardSuitDiamond},
		{Value: cardutil.CardValue3, Suit: cardutil.CardSuitClub},
		{Value: cardutil.CardValue4, Suit: cardutil.CardSuitSpade},
		{Value: cardutil.CardValue5, Suit: cardutil.CardSuitHeart},
	}
	valueGroups := groupByValue(cards)
	combos := engine.findFourWithTwo(cards, valueGroups)

	if len(combos) == 0 {
		t.Error("Expected to find four-with-two")
	} else {
		// 验证长度为6
		found := false
		for _, combo := range combos {
			if len(combo.Cards) == 6 {
				result := cardutil.AnalyzePattern(combo.Cards)
				if result.Pattern == cardutil.PatternFourTwo {
					found = true
				}
			}
		}
		if !found {
			t.Error("Expected to find valid four-with-two combo")
		}
	}

	// 测试四带一对（3333+44）
	cards = []cardutil.Card{
		{Value: cardutil.CardValue3, Suit: cardutil.CardSuitSpade},
		{Value: cardutil.CardValue3, Suit: cardutil.CardSuitHeart},
		{Value: cardutil.CardValue3, Suit: cardutil.CardSuitDiamond},
		{Value: cardutil.CardValue3, Suit: cardutil.CardSuitClub},
		{Value: cardutil.CardValue4, Suit: cardutil.CardSuitSpade},
		{Value: cardutil.CardValue4, Suit: cardutil.CardSuitHeart},
	}
	valueGroups = groupByValue(cards)
	combos = engine.findFourWithTwo(cards, valueGroups)

	if len(combos) == 0 {
		t.Error("Expected to find four-with-two")
	}

	// 测试无四带二的情况
	cards = []cardutil.Card{
		{Value: cardutil.CardValue3, Suit: cardutil.CardSuitSpade},
		{Value: cardutil.CardValue3, Suit: cardutil.CardSuitHeart},
		{Value: cardutil.CardValue3, Suit: cardutil.CardSuitDiamond},
		{Value: cardutil.CardValue4, Suit: cardutil.CardSuitClub},
	}
	valueGroups = groupByValue(cards)
	combos = engine.findFourWithTwo(cards, valueGroups)

	if len(combos) != 0 {
		t.Errorf("Expected no four-with-two for no bomb, got %d", len(combos))
	}
}

func TestEnumerateLegalPlays(t *testing.T) {
	engine := NewAIEngine(&AIConfig{})

	// 测试枚举所有合法出牌
	cards := []cardutil.Card{
		{Value: cardutil.CardValue3, Suit: cardutil.CardSuitSpade},
		{Value: cardutil.CardValue3, Suit: cardutil.CardSuitHeart},
		{Value: cardutil.CardValue3, Suit: cardutil.CardSuitDiamond},
		{Value: cardutil.CardValue3, Suit: cardutil.CardSuitClub},
		{Value: cardutil.CardValue4, Suit: cardutil.CardSuitSpade},
		{Value: cardutil.CardValue4, Suit: cardutil.CardSuitHeart},
		{Value: cardutil.CardValue5, Suit: cardutil.CardSuitDiamond},
		{Value: cardutil.CardValueJokerS, Suit: cardutil.CardSuitJoker},
		{Value: cardutil.CardValueJokerB, Suit: cardutil.CardSuitJoker},
	}

	ctx := &AIContext{
		MyCards: cards,
	}

	combos := engine.enumerateLegalPlays(ctx)

	if len(combos) == 0 {
		t.Error("Expected to find legal plays")
	}

	// 验证包含各种牌型
	hasSingle := false
	hasPair := false
	hasTriple := false
	hasBomb := false
	hasRocket := false
	hasThreeWithOne := false
	hasThreeWithTwo := false

	for _, combo := range combos {
		if len(combo.Cards) == 0 {
			continue
		}
		result := cardutil.AnalyzePattern(combo.Cards)
		switch result.Pattern {
		case cardutil.PatternSingle:
			hasSingle = true
		case cardutil.PatternPair:
			hasPair = true
		case cardutil.PatternTriple:
			hasTriple = true
		case cardutil.PatternBomb:
			hasBomb = true
		case cardutil.PatternRocket:
			hasRocket = true
		case cardutil.PatternTripleOne:
			hasThreeWithOne = true
		case cardutil.PatternTripleTwo:
			hasThreeWithTwo = true
		}
	}

	if !hasSingle {
		t.Error("Expected to find single")
	}
	if !hasPair {
		t.Error("Expected to find pair")
	}
	if !hasTriple {
		t.Error("Expected to find triple")
	}
	if !hasBomb {
		t.Error("Expected to find bomb")
	}
	if !hasRocket {
		t.Error("Expected to find rocket")
	}
	if !hasThreeWithOne {
		t.Error("Expected to find three-with-one")
	}
	if !hasThreeWithTwo {
		t.Error("Expected to find three-with-two")
	}
}

func TestCalculateSplitCost_Bomb(t *testing.T) {
	engine := NewAIEngine(&AIConfig{})

	// 手牌有炸弹(4张3)，打出一张3，拆掉炸弹
	allCards := []cardutil.Card{
		{Value: cardutil.CardValue3, Suit: cardutil.CardSuitSpade},
		{Value: cardutil.CardValue3, Suit: cardutil.CardSuitHeart},
		{Value: cardutil.CardValue3, Suit: cardutil.CardSuitDiamond},
		{Value: cardutil.CardValue3, Suit: cardutil.CardSuitClub},
		{Value: cardutil.CardValue5, Suit: cardutil.CardSuitSpade},
	}
	cardsToPlay := []cardutil.Card{{Value: cardutil.CardValue3, Suit: cardutil.CardSuitSpade}}

	cost := engine.calculateSplitCost(cardsToPlay, allCards)
	if cost < 10 {
		t.Errorf("Expected split cost >= 10 (bomb), got %d", cost)
	}
}

func TestCalculateSplitCost_NoSplit(t *testing.T) {
	engine := NewAIEngine(&AIConfig{})

	// 手牌没有完整牌型，打出一张牌，无拆牌代价
	allCards := []cardutil.Card{
		{Value: cardutil.CardValue3, Suit: cardutil.CardSuitSpade},
		{Value: cardutil.CardValue5, Suit: cardutil.CardSuitHeart},
		{Value: cardutil.CardValue7, Suit: cardutil.CardSuitDiamond},
	}
	cardsToPlay := []cardutil.Card{{Value: cardutil.CardValue3, Suit: cardutil.CardSuitSpade}}

	cost := engine.calculateSplitCost(cardsToPlay, allCards)
	if cost != 0 {
		t.Errorf("Expected split cost 0, got %d", cost)
	}
}

func TestEvaluatePatternValue_Rocket(t *testing.T) {
	engine := NewAIEngine(&AIConfig{})

	// 王炸价值=100分
	cards := []cardutil.Card{
		{Value: cardutil.CardValueJokerS, Suit: cardutil.CardSuitJoker},
		{Value: cardutil.CardValueJokerB, Suit: cardutil.CardSuitJoker},
	}
	result := cardutil.AnalyzePattern(cards)

	value := engine.evaluatePatternValue(cards, result)
	if value != 100 {
		t.Errorf("Expected rocket value 100, got %d", value)
	}
}

func TestEvaluatePatternValue_Bomb(t *testing.T) {
	engine := NewAIEngine(&AIConfig{})

	// 炸弹价值=50分 + 牌值
	cards := []cardutil.Card{
		{Value: cardutil.CardValue3, Suit: cardutil.CardSuitSpade},
		{Value: cardutil.CardValue3, Suit: cardutil.CardSuitHeart},
		{Value: cardutil.CardValue3, Suit: cardutil.CardSuitDiamond},
		{Value: cardutil.CardValue3, Suit: cardutil.CardSuitClub},
	}
	result := cardutil.AnalyzePattern(cards)

	value := engine.evaluatePatternValue(cards, result)
	expectedValue := 50 + int(cardutil.CardValue3) // 50 + 3 = 53
	if value != expectedValue {
		t.Errorf("Expected bomb value %d, got %d", expectedValue, value)
	}
}

func TestEvaluatePatternValue_BigCard(t *testing.T) {
	engine := NewAIEngine(&AIConfig{})

	// 大牌(2)价值=牌值*5
	cards := []cardutil.Card{{Value: cardutil.CardValue2, Suit: cardutil.CardSuitSpade}}
	result := cardutil.AnalyzePattern(cards)

	value := engine.evaluatePatternValue(cards, result)
	expectedValue := int(cardutil.CardValue2) * 5 // 15 * 5 = 75
	if value != expectedValue {
		t.Errorf("Expected big card value %d, got %d", expectedValue, value)
	}
}

func TestEvaluatePatternValue_SmallCard(t *testing.T) {
	engine := NewAIEngine(&AIConfig{})

	// 小牌价值=牌值
	cards := []cardutil.Card{{Value: cardutil.CardValue3, Suit: cardutil.CardSuitSpade}}
	result := cardutil.AnalyzePattern(cards)

	value := engine.evaluatePatternValue(cards, result)
	if value != int(cardutil.CardValue3) {
		t.Errorf("Expected small card value %d, got %d", int(cardutil.CardValue3), value)
	}
}

func TestGetPatternSplitCost(t *testing.T) {
	engine := NewAIEngine(&AIConfig{})

	tests := []struct {
		pattern       cardutil.CardPattern
		expectedCost  int
		description   string
	}{
		{cardutil.PatternBomb, 10, "Bomb"},
		{cardutil.PatternRocket, 15, "Rocket"},
		{cardutil.PatternStraight, 3, "Straight"},
		{cardutil.PatternStraightPair, 4, "StraightPair"},
		{cardutil.PatternAirplane, 6, "Airplane"},
		{cardutil.PatternAirplaneWings, 6, "AirplaneWings"},
		{cardutil.PatternTripleOne, 2, "TripleOne"},
		{cardutil.PatternTripleTwo, 2, "TripleTwo"},
		{cardutil.PatternSingle, 0, "Single"},
	}

	for _, test := range tests {
		cost := engine.getPatternSplitCost(test.pattern)
		if cost != test.expectedCost {
			t.Errorf("%s: expected cost %d, got %d", test.description, test.expectedCost, cost)
		}
	}
}

func TestIdentifyCompletePatterns_Bomb(t *testing.T) {
	engine := NewAIEngine(&AIConfig{})

	// 手牌有炸弹
	cards := []cardutil.Card{
		{Value: cardutil.CardValue3, Suit: cardutil.CardSuitSpade},
		{Value: cardutil.CardValue3, Suit: cardutil.CardSuitHeart},
		{Value: cardutil.CardValue3, Suit: cardutil.CardSuitDiamond},
		{Value: cardutil.CardValue3, Suit: cardutil.CardSuitClub},
		{Value: cardutil.CardValue5, Suit: cardutil.CardSuitSpade},
	}

	patterns := engine.identifyCompletePatterns(cards)
	hasBomb := false
	for _, p := range patterns {
		if p.Pattern == cardutil.PatternBomb {
			hasBomb = true
			break
		}
	}
	if !hasBomb {
		t.Error("Expected to identify bomb pattern")
	}
}

func TestIdentifyCompletePatterns_Rocket(t *testing.T) {
	engine := NewAIEngine(&AIConfig{})

	// 手牌有王炸
	cards := []cardutil.Card{
		{Value: cardutil.CardValueJokerS, Suit: cardutil.CardSuitJoker},
		{Value: cardutil.CardValueJokerB, Suit: cardutil.CardSuitJoker},
		{Value: cardutil.CardValue5, Suit: cardutil.CardSuitSpade},
	}

	patterns := engine.identifyCompletePatterns(cards)
	hasRocket := false
	for _, p := range patterns {
		if p.Pattern == cardutil.PatternRocket {
			hasRocket = true
			break
		}
	}
	if !hasRocket {
		t.Error("Expected to identify rocket pattern")
	}
}

func TestFindBestComboWithSplitCost(t *testing.T) {
	engine := NewAIEngine(&AIConfig{})

	// 场景:手牌有炸弹(4张3)和单张4,上家出单张2
	// 应该选择拆牌代价更小的方案(出4而不是拆炸弹出3)
	allCards := []cardutil.Card{
		{Value: cardutil.CardValue3, Suit: cardutil.CardSuitSpade},
		{Value: cardutil.CardValue3, Suit: cardutil.CardSuitHeart},
		{Value: cardutil.CardValue3, Suit: cardutil.CardSuitDiamond},
		{Value: cardutil.CardValue3, Suit: cardutil.CardSuitClub},
		{Value: cardutil.CardValue4, Suit: cardutil.CardSuitSpade},
	}

	ctx := &AIContext{
		MyCards: allCards,
		LastPlay: &LastPlayInfo{
			Cards:   []cardutil.Card{{Value: cardutil.CardValue2, Suit: cardutil.CardSuitSpade}},
			Pattern: cardutil.PatternSingle,
			Main:    cardutil.CardValue2,
			UID:     "opponent",
		},
		MyRole:     types.RolePeasant,
		Difficulty: "normal",
		Players: map[string]*PlayerInfo{
			"opponent": {UID: "opponent", IsLandlord: true, CardCount: 10},
		},
	}

	// 枚举所有合法出牌
	combos := engine.enumerateLegalPlays(ctx)
	validCombos := engine.filterBeatingCombos(combos, ctx.LastPlay)

	bestCombo := engine.findBestComboWithSplitCost(ctx, validCombos)
	if bestCombo == nil {
		t.Error("Expected non-nil best combo")
		return
	}

	// 应该选择炸弹(因为单张4管不了2,只能用炸弹)
	result := cardutil.AnalyzePattern(bestCombo)
	if !result.Pattern.IsBomb() {
		t.Errorf("Expected bomb pattern, got %v", result.Pattern)
	}
}
