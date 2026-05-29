// Package cardutil 提供斗地主牌型判定、大小比较、牌堆操作等工具函数
package cardutil

// CardValue 牌面值
type CardValue int

const (
	CardValueUnknown CardValue = 0
	CardValue3       CardValue = 3
	CardValue4       CardValue = 4
	CardValue5       CardValue = 5
	CardValue6       CardValue = 6
	CardValue7       CardValue = 7
	CardValue8       CardValue = 8
	CardValue9       CardValue = 9
	CardValue10      CardValue = 10
	CardValueJ       CardValue = 11
	CardValueQ       CardValue = 12
	CardValueK       CardValue = 13
	CardValueA       CardValue = 14
	CardValue2       CardValue = 15
	CardValueJokerS  CardValue = 16 // 小王
	CardValueJokerB  CardValue = 17 // 大王
)

// CardSuit 花色
type CardSuit int

const (
	CardSuitUnknown CardSuit = 0
	CardSuitSpade   CardSuit = 1 // 黑桃
	CardSuitHeart   CardSuit = 2 // 红桃
	CardSuitClub    CardSuit = 3 // 梅花
	CardSuitDiamond CardSuit = 4 // 方块
	CardSuitJoker   CardSuit = 5 // 王牌
)

// Card 单张牌
type Card struct {
	Value CardValue `json:"value"`
	Suit  CardSuit  `json:"suit"`
}

// String 返回牌的字符串表示（如 "S3", "BJ"）
func (c Card) String() string {
	suitStr := map[CardSuit]string{
		CardSuitSpade:   "S", // 黑桃 Spade
		CardSuitHeart:   "H", // 红桃 Heart
		CardSuitClub:    "C", // 梅花 Club
		CardSuitDiamond: "D", // 方块 Diamond
		CardSuitJoker:   "J", // 王 Joker
	}
	valueStr := map[CardValue]string{
		CardValue3: "3", CardValue4: "4", CardValue5: "5", CardValue6: "6",
		CardValue7: "7", CardValue8: "8", CardValue9: "9", CardValue10: "10",
		CardValueJ: "J", CardValueQ: "Q", CardValueK: "K", CardValueA: "A",
		CardValue2:      "2",
		CardValueJokerS: "S", // 小王 Small joker
		CardValueJokerB: "B", // 大王 Big joker
	}
	s := suitStr[c.Suit]
	v := valueStr[c.Value]
	return s + v
}

// CardPattern 牌型
type CardPattern int

const (
	PatternUnknown       CardPattern = 0
	PatternSingle        CardPattern = 1  // 单张
	PatternPair          CardPattern = 2  // 对子
	PatternTriple        CardPattern = 3  // 三条
	PatternTripleOne     CardPattern = 4  // 三带一
	PatternTripleTwo     CardPattern = 5  // 三带二
	PatternStraight      CardPattern = 6  // 顺子（至少5张连续单牌）
	PatternStraightPair  CardPattern = 7  // 连对（至少3对连续对子）
	PatternAirplane      CardPattern = 8  // 飞机（至少2个连续三条）
	PatternAirplaneWings CardPattern = 9  // 飞机带翅膀（飞机+等量单牌或对子）
	PatternFourTwo       CardPattern = 10 // 四带二
	PatternBomb          CardPattern = 11 // 炸弹（四张相同）
	PatternRocket        CardPattern = 12 // 王炸（大王+小王）
)

// String 返回牌型名称
func (p CardPattern) String() string {
	names := map[CardPattern]string{
		PatternUnknown:       "未知",
		PatternSingle:        "单张",
		PatternPair:          "对子",
		PatternTriple:        "三条",
		PatternTripleOne:     "三带一",
		PatternTripleTwo:     "三带二",
		PatternStraight:      "顺子",
		PatternStraightPair:  "连对",
		PatternAirplane:      "飞机",
		PatternAirplaneWings: "飞机带翅膀",
		PatternFourTwo:       "四带二",
		PatternBomb:          "炸弹",
		PatternRocket:        "王炸",
	}
	if name, ok := names[p]; ok {
		return name
	}
	return "未知"
}

// PlayResult 出牌判定结果
type PlayResult struct {
	Valid   bool        // 是否合法
	Pattern CardPattern // 牌型
	Main    CardValue   // 主牌值（用于比较大小）
	Length  int         // 牌型长度（顺子张数、飞机个数等）
}

// IsBomb 判断是否为炸弹（炸弹或王炸）
func (p CardPattern) IsBomb() bool {
	return p == PatternBomb || p == PatternRocket
}
