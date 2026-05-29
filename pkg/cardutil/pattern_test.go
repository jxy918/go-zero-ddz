package cardutil

import (
	"testing"
)

// ============ 单张测试 ============

func TestAnalyzeSingle(t *testing.T) {
	cards := []Card{{Value: CardValue5, Suit: CardSuitSpade}}
	result := AnalyzePattern(cards)

	if !result.Valid {
		t.Error("single card should be valid")
	}
	if result.Pattern != PatternSingle {
		t.Errorf("expected PatternSingle, got %v", result.Pattern)
	}
	if result.Main != CardValue5 {
		t.Errorf("expected main CardValue5, got %v", result.Main)
	}
}

// ============ 对子测试 ============

func TestAnalyzePair(t *testing.T) {
	cards := []Card{
		{Value: CardValue8, Suit: CardSuitSpade},
		{Value: CardValue8, Suit: CardSuitHeart},
	}
	result := AnalyzePattern(cards)

	if !result.Valid {
		t.Error("pair should be valid")
	}
	if result.Pattern != PatternPair {
		t.Errorf("expected PatternPair, got %v", result.Pattern)
	}
	if result.Main != CardValue8 {
		t.Errorf("expected main CardValue8, got %v", result.Main)
	}
}

func TestInvalidPair(t *testing.T) {
	cards := []Card{
		{Value: CardValue8, Suit: CardSuitSpade},
		{Value: CardValue9, Suit: CardSuitHeart},
	}
	result := AnalyzePattern(cards)

	if result.Valid {
		t.Error("different values should not be a valid pair")
	}
}

// ============ 王炸测试 ============

func TestRocket(t *testing.T) {
	cards := []Card{
		{Value: CardValueJokerS, Suit: CardSuitJoker},
		{Value: CardValueJokerB, Suit: CardSuitJoker},
	}
	result := AnalyzePattern(cards)

	if !result.Valid {
		t.Error("rocket should be valid")
	}
	if result.Pattern != PatternRocket {
		t.Errorf("expected PatternRocket, got %v", result.Pattern)
	}
}

// ============ 三条测试 ============

func TestTriple(t *testing.T) {
	cards := []Card{
		{Value: CardValueK, Suit: CardSuitSpade},
		{Value: CardValueK, Suit: CardSuitHeart},
		{Value: CardValueK, Suit: CardSuitClub},
	}
	result := AnalyzePattern(cards)

	if !result.Valid {
		t.Error("triple should be valid")
	}
	if result.Pattern != PatternTriple {
		t.Errorf("expected PatternTriple, got %v", result.Pattern)
	}
	if result.Main != CardValueK {
		t.Errorf("expected main CardValueK, got %v", result.Main)
	}
}

// ============ 炸弹测试 ============

func TestBomb(t *testing.T) {
	cards := []Card{
		{Value: CardValue7, Suit: CardSuitSpade},
		{Value: CardValue7, Suit: CardSuitHeart},
		{Value: CardValue7, Suit: CardSuitClub},
		{Value: CardValue7, Suit: CardSuitDiamond},
	}
	result := AnalyzePattern(cards)

	if !result.Valid {
		t.Error("bomb should be valid")
	}
	if result.Pattern != PatternBomb {
		t.Errorf("expected PatternBomb, got %v", result.Pattern)
	}
}

// ============ 三带一测试 ============

func TestTripleOne(t *testing.T) {
	cards := []Card{
		{Value: CardValueQ, Suit: CardSuitSpade},
		{Value: CardValueQ, Suit: CardSuitHeart},
		{Value: CardValueQ, Suit: CardSuitClub},
		{Value: CardValue3, Suit: CardSuitDiamond},
	}
	result := AnalyzePattern(cards)

	if !result.Valid {
		t.Error("triple+one should be valid")
	}
	if result.Pattern != PatternTripleOne {
		t.Errorf("expected PatternTripleOne, got %v", result.Pattern)
	}
	if result.Main != CardValueQ {
		t.Errorf("expected main CardValueQ, got %v", result.Main)
	}
}

// ============ 三带二测试 ============

func TestTripleTwo(t *testing.T) {
	cards := []Card{
		{Value: CardValueJ, Suit: CardSuitSpade},
		{Value: CardValueJ, Suit: CardSuitHeart},
		{Value: CardValueJ, Suit: CardSuitClub},
		{Value: CardValue5, Suit: CardSuitDiamond},
		{Value: CardValue5, Suit: CardSuitSpade},
	}
	result := AnalyzePattern(cards)

	if !result.Valid {
		t.Error("triple+two should be valid")
	}
	if result.Pattern != PatternTripleTwo {
		t.Errorf("expected PatternTripleTwo, got %v", result.Pattern)
	}
	if result.Main != CardValueJ {
		t.Errorf("expected main CardValueJ, got %v", result.Main)
	}
}

// ============ 顺子测试 ============

func TestStraight(t *testing.T) {
	cards := []Card{
		{Value: CardValue3, Suit: CardSuitSpade},
		{Value: CardValue4, Suit: CardSuitHeart},
		{Value: CardValue5, Suit: CardSuitClub},
		{Value: CardValue6, Suit: CardSuitDiamond},
		{Value: CardValue7, Suit: CardSuitSpade},
	}
	result := AnalyzePattern(cards)

	if !result.Valid {
		t.Error("straight 3-7 should be valid")
	}
	if result.Pattern != PatternStraight {
		t.Errorf("expected PatternStraight, got %v", result.Pattern)
	}
	if result.Main != CardValue7 {
		t.Errorf("expected main CardValue7, got %v", result.Main)
	}
}

func TestStraightMinLength(t *testing.T) {
	// 4张不能构成顺子
	cards := []Card{
		{Value: CardValue3, Suit: CardSuitSpade},
		{Value: CardValue4, Suit: CardSuitHeart},
		{Value: CardValue5, Suit: CardSuitClub},
		{Value: CardValue6, Suit: CardSuitDiamond},
	}
	result := AnalyzePattern(cards)

	if result.Valid {
		t.Error("4 cards cannot form a straight")
	}
}

func TestStraightWith2(t *testing.T) {
	// 顺子不能有2
	cards := []Card{
		{Value: CardValue10, Suit: CardSuitSpade},
		{Value: CardValueJ, Suit: CardSuitHeart},
		{Value: CardValueQ, Suit: CardSuitClub},
		{Value: CardValueK, Suit: CardSuitDiamond},
		{Value: CardValue2, Suit: CardSuitSpade},
	}
	result := AnalyzePattern(cards)

	if result.Valid {
		t.Error("straight cannot contain 2")
	}
}

func TestStraightMax(t *testing.T) {
	// 最大顺子 3-A
	cards := []Card{
		{Value: CardValue3, Suit: CardSuitSpade},
		{Value: CardValue4, Suit: CardSuitHeart},
		{Value: CardValue5, Suit: CardSuitClub},
		{Value: CardValue6, Suit: CardSuitDiamond},
		{Value: CardValue7, Suit: CardSuitSpade},
		{Value: CardValue8, Suit: CardSuitHeart},
		{Value: CardValue9, Suit: CardSuitClub},
		{Value: CardValue10, Suit: CardSuitDiamond},
		{Value: CardValueJ, Suit: CardSuitSpade},
		{Value: CardValueQ, Suit: CardSuitHeart},
		{Value: CardValueK, Suit: CardSuitClub},
		{Value: CardValueA, Suit: CardSuitDiamond},
	}
	result := AnalyzePattern(cards)

	if !result.Valid {
		t.Error("straight 3-A should be valid")
	}
	if result.Pattern != PatternStraight {
		t.Errorf("expected PatternStraight, got %v", result.Pattern)
	}
	if result.Main != CardValueA {
		t.Errorf("expected main CardValueA, got %v", result.Main)
	}
}

// ============ 连对测试 ============

func TestStraightPair(t *testing.T) {
	cards := []Card{
		{Value: CardValue3, Suit: CardSuitSpade},
		{Value: CardValue3, Suit: CardSuitHeart},
		{Value: CardValue4, Suit: CardSuitClub},
		{Value: CardValue4, Suit: CardSuitDiamond},
		{Value: CardValue5, Suit: CardSuitSpade},
		{Value: CardValue5, Suit: CardSuitHeart},
	}
	result := AnalyzePattern(cards)

	if !result.Valid {
		t.Error("straight pair 33-44-55 should be valid")
	}
	if result.Pattern != PatternStraightPair {
		t.Errorf("expected PatternStraightPair, got %v", result.Pattern)
	}
}

func TestStraightPairMinLength(t *testing.T) {
	// 2对不能构成连对
	cards := []Card{
		{Value: CardValue3, Suit: CardSuitSpade},
		{Value: CardValue3, Suit: CardSuitHeart},
		{Value: CardValue4, Suit: CardSuitClub},
		{Value: CardValue4, Suit: CardSuitDiamond},
	}
	result := AnalyzePattern(cards)

	if result.Valid {
		t.Error("2 pairs cannot form a straight pair")
	}
}

// ============ 飞机测试 ============

func TestAirplane(t *testing.T) {
	cards := []Card{
		{Value: CardValue3, Suit: CardSuitSpade},
		{Value: CardValue3, Suit: CardSuitHeart},
		{Value: CardValue3, Suit: CardSuitClub},
		{Value: CardValue4, Suit: CardSuitDiamond},
		{Value: CardValue4, Suit: CardSuitSpade},
		{Value: CardValue4, Suit: CardSuitHeart},
	}
	result := AnalyzePattern(cards)

	if !result.Valid {
		t.Error("airplane 333-444 should be valid")
	}
	if result.Pattern != PatternAirplane {
		t.Errorf("expected PatternAirplane, got %v", result.Pattern)
	}
}

func TestAirplaneWithWings(t *testing.T) {
	// 飞机带单：333444 + 57
	cards := []Card{
		{Value: CardValue3, Suit: CardSuitSpade},
		{Value: CardValue3, Suit: CardSuitHeart},
		{Value: CardValue3, Suit: CardSuitClub},
		{Value: CardValue4, Suit: CardSuitDiamond},
		{Value: CardValue4, Suit: CardSuitSpade},
		{Value: CardValue4, Suit: CardSuitHeart},
		{Value: CardValue5, Suit: CardSuitClub},
		{Value: CardValue7, Suit: CardSuitDiamond},
	}
	result := AnalyzePattern(cards)

	if !result.Valid {
		t.Error("airplane with wings (single) should be valid")
	}
	if result.Pattern != PatternAirplaneWings {
		t.Errorf("expected PatternAirplaneWings, got %v", result.Pattern)
	}
}

func TestAirplaneWithWingsPair(t *testing.T) {
	// 飞机带对：333444 + 5577
	cards := []Card{
		{Value: CardValue3, Suit: CardSuitSpade},
		{Value: CardValue3, Suit: CardSuitHeart},
		{Value: CardValue3, Suit: CardSuitClub},
		{Value: CardValue4, Suit: CardSuitDiamond},
		{Value: CardValue4, Suit: CardSuitSpade},
		{Value: CardValue4, Suit: CardSuitHeart},
		{Value: CardValue5, Suit: CardSuitClub},
		{Value: CardValue5, Suit: CardSuitDiamond},
		{Value: CardValue7, Suit: CardSuitSpade},
		{Value: CardValue7, Suit: CardSuitHeart},
	}
	result := AnalyzePattern(cards)

	if !result.Valid {
		t.Error("airplane with wings (pair) should be valid")
	}
	if result.Pattern != PatternAirplaneWings {
		t.Errorf("expected PatternAirplaneWings, got %v", result.Pattern)
	}
}

func TestAirplaneInvalidWings(t *testing.T) {
	// 飞机翅膀数量不匹配：333444 + 556（2个对子+1个单张 = 5张翅膀，需要4张）
	cards := []Card{
		{Value: CardValue3, Suit: CardSuitSpade},
		{Value: CardValue3, Suit: CardSuitHeart},
		{Value: CardValue3, Suit: CardSuitClub},
		{Value: CardValue4, Suit: CardSuitDiamond},
		{Value: CardValue4, Suit: CardSuitSpade},
		{Value: CardValue4, Suit: CardSuitHeart},
		{Value: CardValue5, Suit: CardSuitClub},
		{Value: CardValue5, Suit: CardSuitDiamond},
		{Value: CardValue6, Suit: CardSuitSpade},
	}
	result := AnalyzePattern(cards)

	if result.Valid {
		t.Error("airplane with mismatched wings should be invalid")
	}
}

// ============ 四带二测试 ============

func TestFourTwo(t *testing.T) {
	cards := []Card{
		{Value: CardValue8, Suit: CardSuitSpade},
		{Value: CardValue8, Suit: CardSuitHeart},
		{Value: CardValue8, Suit: CardSuitClub},
		{Value: CardValue8, Suit: CardSuitDiamond},
		{Value: CardValue3, Suit: CardSuitSpade},
		{Value: CardValue4, Suit: CardSuitHeart},
	}
	result := AnalyzePattern(cards)

	if !result.Valid {
		t.Error("four+two should be valid")
	}
	if result.Pattern != PatternFourTwo {
		t.Errorf("expected PatternFourTwo, got %v", result.Pattern)
	}
}

// ============ 牌型比较测试 ============

func TestCompareSamePattern(t *testing.T) {
	// 同为单张，K > Q
	play1 := PlayResult{Valid: true, Pattern: PatternSingle, Main: CardValueK}
	play2 := PlayResult{Valid: true, Pattern: PatternSingle, Main: CardValueQ}

	if ComparePlays(play1, play2) <= 0 {
		t.Error("K should be bigger than Q")
	}
}

func TestBombBeatsNonBomb(t *testing.T) {
	// 炸弹 > 任何非炸弹
	bomb := PlayResult{Valid: true, Pattern: PatternBomb, Main: CardValue3}
	straight := PlayResult{Valid: true, Pattern: PatternStraight, Main: CardValueA, Length: 12}

	if !CanBeat(bomb, straight) {
		t.Error("bomb should beat straight")
	}
}

func TestRocketBeatsBomb(t *testing.T) {
	// 王炸 > 炸弹
	rocket := PlayResult{Valid: true, Pattern: PatternRocket, Main: CardValueJokerB}
	bomb := PlayResult{Valid: true, Pattern: PatternBomb, Main: CardValue2}

	if !CanBeat(rocket, bomb) {
		t.Error("rocket should beat bomb")
	}
}

func TestDifferentPatternCannotCompare(t *testing.T) {
	// 不同类型且都不是炸弹 → 无法比较
	pair := PlayResult{Valid: true, Pattern: PatternPair, Main: CardValue8}
	straight := PlayResult{Valid: true, Pattern: PatternStraight, Main: CardValue7, Length: 5}

	result := ComparePlays(pair, straight)
	if result != 0 {
		t.Error("different non-bomb patterns should not be comparable")
	}
}

func TestBombComparison(t *testing.T) {
	// 炸弹之间比较主牌值
	bomb7 := PlayResult{Valid: true, Pattern: PatternBomb, Main: CardValue7}
	bomb8 := PlayResult{Valid: true, Pattern: PatternBomb, Main: CardValue8}

	if !CanBeat(bomb8, bomb7) {
		t.Error("bomb 8888 should beat bomb 7777")
	}
	if CanBeat(bomb7, bomb8) {
		t.Error("bomb 7777 should not beat bomb 8888")
	}
}

// ============ 发牌测试 ============

func TestNewFullDeck(t *testing.T) {
	deck := NewFullDeck()

	if len(deck) != 54 {
		t.Errorf("expected 54 cards, got %d", len(deck))
	}

	// 检查每种牌值数量
	counts := make(map[CardValue]int)
	for _, c := range deck {
		counts[c.Value]++
	}

	// 3-A 应该各4张
	for v := CardValue3; v <= CardValue2; v++ {
		if counts[v] != 4 {
			t.Errorf("expected 4 cards of value %d, got %d", v, counts[v])
		}
	}

	// 大小王各1张
	if counts[CardValueJokerS] != 1 {
		t.Errorf("expected 1 small joker, got %d", counts[CardValueJokerS])
	}
	if counts[CardValueJokerB] != 1 {
		t.Errorf("expected 1 big joker, got %d", counts[CardValueJokerB])
	}
}

func TestDealCards(t *testing.T) {
	deck := NewFullDeck()
	hands, bottom := DealCards(deck)

	if len(hands) != 3 {
		t.Fatalf("expected 3 hands, got %d", len(hands))
	}

	for i, hand := range hands {
		if len(hand) != 17 {
			t.Errorf("hand %d should have 17 cards, got %d", i, len(hand))
		}
	}

	if len(bottom) != 3 {
		t.Errorf("bottom should have 3 cards, got %d", len(bottom))
	}

	// 检查总牌数
	total := 0
	for _, hand := range hands {
		total += len(hand)
	}
	total += len(bottom)

	if total != 54 {
		t.Errorf("total cards should be 54, got %d", total)
	}
}

func TestSortCards(t *testing.T) {
	cards := []Card{
		{Value: CardValue3, Suit: CardSuitSpade},
		{Value: CardValueA, Suit: CardSuitHeart},
		{Value: CardValue7, Suit: CardSuitClub},
		{Value: CardValueK, Suit: CardSuitDiamond},
		{Value: CardValue5, Suit: CardSuitSpade},
	}

	sorted := SortCards(cards)

	// 应该从大到小排序
	expected := []CardValue{CardValueA, CardValueK, CardValue7, CardValue5, CardValue3}
	for i, c := range sorted {
		if c.Value != expected[i] {
			t.Errorf("position %d: expected %v, got %v", i, expected[i], c.Value)
		}
	}
}

// ============ 边界用例测试 ============

func TestEmptyCards(t *testing.T) {
	result := AnalyzePattern([]Card{})
	if result.Valid {
		t.Error("empty cards should be invalid")
	}
}

func TestInvalidCombination(t *testing.T) {
	// 3张不同牌值 → 无效
	cards := []Card{
		{Value: CardValue3, Suit: CardSuitSpade},
		{Value: CardValue4, Suit: CardSuitHeart},
		{Value: CardValue5, Suit: CardSuitClub},
	}
	result := AnalyzePattern(cards)

	if result.Valid {
		t.Error("3 different cards should be invalid")
	}
}

func TestMixedSuitsStraight(t *testing.T) {
	// 顺子不要求同花色
	cards := []Card{
		{Value: CardValue3, Suit: CardSuitSpade},
		{Value: CardValue4, Suit: CardSuitHeart},
		{Value: CardValue5, Suit: CardSuitClub},
		{Value: CardValue6, Suit: CardSuitDiamond},
		{Value: CardValue7, Suit: CardSuitSpade},
	}
	result := AnalyzePattern(cards)

	if !result.Valid {
		t.Error("straight with mixed suits should be valid")
	}
}
