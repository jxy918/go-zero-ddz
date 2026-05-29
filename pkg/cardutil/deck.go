package cardutil

import (
	"math/rand"
	"strings"
)

// NewFullDeck 创建一副完整的斗地主牌（54张）
func NewFullDeck() []Card {
	deck := make([]Card, 0, 54)

	// 3-A，每种花色4张
	suits := []CardSuit{CardSuitSpade, CardSuitHeart, CardSuitClub, CardSuitDiamond}
	values := []CardValue{
		CardValue3, CardValue4, CardValue5, CardValue6, CardValue7,
		CardValue8, CardValue9, CardValue10, CardValueJ, CardValueQ,
		CardValueK, CardValueA, CardValue2,
	}

	for _, v := range values {
		for _, s := range suits {
			deck = append(deck, Card{Value: v, Suit: s})
		}
	}

	// 大小王
	deck = append(deck, Card{Value: CardValueJokerS, Suit: CardSuitJoker})
	deck = append(deck, Card{Value: CardValueJokerB, Suit: CardSuitJoker})

	return deck
}

// Shuffle 洗牌（随机打乱）
func Shuffle(deck []Card, rng *rand.Rand) []Card {
	shuffled := make([]Card, len(deck))
	copy(shuffled, deck)

	for i := len(shuffled) - 1; i > 0; i-- {
		j := rng.Intn(i + 1)
		shuffled[i], shuffled[j] = shuffled[j], shuffled[i]
	}

	return shuffled
}

// DealCards 发牌：3人斗地主，每人17张，3张底牌
// 返回: [玩家1手牌, 玩家2手牌, 玩家3手牌, 底牌]
func DealCards(deck []Card) ([][]Card, []Card) {
	if len(deck) != 54 {
		panic("deck must have 54 cards")
	}

	// 洗牌
	rng := rand.New(rand.NewSource(rand.Int63()))
	shuffled := Shuffle(deck, rng)

	hands := make([][]Card, 3)
	for i := range 3 {
		hands[i] = shuffled[i*17 : (i+1)*17]
	}

	bottomCards := shuffled[51:54]

	return hands, bottomCards
}

// SortCards 按牌值排序（从大到小）
func SortCards(cards []Card) []Card {
	sorted := make([]Card, len(cards))
	copy(sorted, cards)

	// 排序：先按牌值从大到小，同牌值按花色
	for i := 0; i < len(sorted); i++ {
		for j := i + 1; j < len(sorted); j++ {
			if shouldSwap(sorted[i], sorted[j]) {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}

	return sorted
}

// shouldSwap 判断是否应该交换（a 应该排在 b 后面）
func shouldSwap(a, b Card) bool {
	if a.Value != b.Value {
		return a.Value < b.Value // 牌值小的排后面
	}
	return a.Suit < b.Suit // 同牌值，花色小的排后面
}

// CardsToString 将手牌转换为可读字符串
func CardsToString(cards []Card) string {
	sorted := SortCards(cards)
	var sb strings.Builder
	for i, c := range sorted {
		if i > 0 {
			sb.WriteString(" ")
		}
		sb.WriteString(c.String())
	}
	return sb.String()
}
