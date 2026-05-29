package ai

import (
	"go-zero-ddz/pkg/cardutil"
)

// CardCounter 记牌器
type CardCounter struct {
	playedCards    map[cardutil.CardValue]int
	bigCardHistory []BigCardRecord
}

// BigCardRecord 大牌出牌记录
type BigCardRecord struct {
	Value     cardutil.CardValue
	PlayerUID string
	Timestamp int64
}

// NewCardCounter 创建记牌器
func NewCardCounter() *CardCounter {
	return &CardCounter{
		playedCards: make(map[cardutil.CardValue]int),
	}
}

// RecordPlayed 记录已出的牌
func (cc *CardCounter) RecordPlayed(cards []cardutil.Card, playerUID string, timestamp int64) {
	for _, c := range cards {
		cc.playedCards[c.Value]++

		// 记录大牌（10 及以上）
		if c.Value >= cardutil.CardValue10 {
			cc.bigCardHistory = append(cc.bigCardHistory, BigCardRecord{
				Value:     c.Value,
				PlayerUID: playerUID,
				Timestamp: timestamp,
			})
		}
	}
}

// Remaining 返回剩余未出的牌
func (cc *CardCounter) Remaining() map[cardutil.CardValue]int {
	remaining := make(map[cardutil.CardValue]int)
	fullDeck := cc.fullDeck()

	for val, total := range fullDeck {
		played := cc.playedCards[val]
		remaining[val] = total - played
	}

	return remaining
}

// IsCardOutThere 判断某张牌是否可能在外
func (cc *CardCounter) IsCardOutThere(val cardutil.CardValue) bool {
	return cc.Remaining()[val] > 0
}

// CountRemaining 返回某牌值剩余数量
func (cc *CardCounter) CountRemaining(val cardutil.CardValue) int {
	return cc.Remaining()[val]
}

// HasBigCardsOut 判断大牌是否已出
func (cc *CardCounter) HasBigCardsOut(val cardutil.CardValue) bool {
	fullDeck := cc.fullDeck()
	return cc.playedCards[val] >= fullDeck[val]
}

// fullDeck 返回完整牌组统计
func (cc *CardCounter) fullDeck() map[cardutil.CardValue]int {
	deck := make(map[cardutil.CardValue]int)
	for v := cardutil.CardValue3; v <= cardutil.CardValue2; v++ {
		deck[v] = 4
	}
	deck[cardutil.CardValueJokerS] = 1
	deck[cardutil.CardValueJokerB] = 1
	return deck
}
