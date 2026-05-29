package cardutil

import (
	"cmp"
	"slices"
)

// AnalyzePattern 分析一组牌的牌型
// 返回 PlayResult 包含牌型、主牌值、长度等信息
func AnalyzePattern(cards []Card) PlayResult {
	if len(cards) == 0 {
		return PlayResult{Valid: false}
	}

	// 按牌值排序
	sorted := make([]Card, len(cards))
	copy(sorted, cards)
	slices.SortFunc(sorted, func(a, b Card) int {
		return cmp.Compare(a.Value, b.Value)
	})

	// 统计每种牌值的数量
	counts := countValues(sorted)

	switch len(cards) {
	case 1:
		return analyzeSingle(sorted)
	case 2:
		return analyzeDouble(sorted, counts)
	case 3:
		return analyzeTriple(sorted, counts)
	case 4:
		return analyzeFour(sorted, counts)
	default:
		return analyzeMulti(sorted, counts)
	}
}

// analyzeSingle 单张
func analyzeSingle(cards []Card) PlayResult {
	return PlayResult{
		Valid:   true,
		Pattern: PatternSingle,
		Main:    cards[0].Value,
		Length:  1,
	}
}

// analyzeDouble 两张牌：对子 or 王炸
func analyzeDouble(cards []Card, counts map[CardValue]int) PlayResult {
	// 王炸
	if len(counts) == 2 {
		_, hasJokerS := counts[CardValueJokerS]
		_, hasJokerB := counts[CardValueJokerB]
		if hasJokerS && hasJokerB {
			return PlayResult{
				Valid:   true,
				Pattern: PatternRocket,
				Main:    CardValueJokerB, // 王炸以大王为主牌值
				Length:  2,
			}
		}
	}

	// 对子
	if len(counts) == 1 {
		for v := range counts {
			if v >= CardValue3 && v <= CardValue2 {
				return PlayResult{
					Valid:   true,
					Pattern: PatternPair,
					Main:    v,
					Length:  2,
				}
			}
		}
	}

	return PlayResult{Valid: false}
}

// analyzeTriple 三张牌：三条 or 三带一（不可能，因为只有3张）
func analyzeTriple(cards []Card, counts map[CardValue]int) PlayResult {
	if len(counts) == 1 {
		for v := range counts {
			if v >= CardValue3 && v <= CardValue2 {
				return PlayResult{
					Valid:   true,
					Pattern: PatternTriple,
					Main:    v,
					Length:  3,
				}
			}
		}
	}
	return PlayResult{Valid: false}
}

// analyzeFour 四张牌：炸弹 or 三带一
func analyzeFour(cards []Card, counts map[CardValue]int) PlayResult {
	// 炸弹（四张相同）
	if len(counts) == 1 {
		for v := range counts {
			if v >= CardValue3 && v <= CardValue2 {
				return PlayResult{
					Valid:   true,
					Pattern: PatternBomb,
					Main:    v,
					Length:  4,
				}
			}
		}
	}

	// 三带一
	if result := isTripleOne(cards, counts); result.Valid {
		return result
	}

	return PlayResult{Valid: false}
}

// analyzeMulti 五张及以上
func analyzeMulti(cards []Card, counts map[CardValue]int) PlayResult {
	n := len(cards)

	// 炸弹（四张相同 + 任意一张）→ 实际是四带一，但斗地主中四带一是合法牌型
	// 先检查炸弹
	for v, c := range counts {
		if c == 4 && v >= CardValue3 && v <= CardValue2 {
			// 四带二（需要6张）或四带一（需要5张）
			if n == 5 {
				return PlayResult{
					Valid:   true,
					Pattern: PatternFourTwo, // 四带一也算四带二的一种
					Main:    v,
					Length:  5,
				}
			}
			if n == 6 {
				return PlayResult{
					Valid:   true,
					Pattern: PatternFourTwo,
					Main:    v,
					Length:  6,
				}
			}
		}
	}

	// 顺子：至少5张连续的单牌，不能有2和王牌
	if result := isStraight(cards, counts); result.Valid {
		return result
	}

	// 连对：至少3对连续的对子
	if n >= 6 && n%2 == 0 {
		if result := isStraightPair(cards, counts); result.Valid {
			return result
		}
	}

	// 飞机（不带翅膀）：至少2个连续三条
	if n >= 6 && n%3 == 0 {
		if result := isAirplane(cards, counts); result.Valid {
			return result
		}
	}

	// 飞机带翅膀
	if n >= 8 {
		if result := isAirplaneWithWings(cards, counts); result.Valid {
			return result
		}
	}

	// 三带一（4张）
	if n == 4 {
		if result := isTripleOne(cards, counts); result.Valid {
			return result
		}
	}

	// 三带二（5张）
	if n == 5 {
		if result := isTripleTwo(cards, counts); result.Valid {
			return result
		}
	}

	return PlayResult{Valid: false}
}

// isStraight 判断是否为顺子
func isStraight(cards []Card, counts map[CardValue]int) PlayResult {
	// 顺子要求：所有牌都是单张，连续，不含2和王牌
	if len(counts) != len(cards) {
		return PlayResult{Valid: false} // 有重复牌值，不是顺子
	}

	// 收集所有牌值并排序
	values := make([]CardValue, 0, len(counts))
	for v := range counts {
		if v >= CardValue2 || v == CardValueJokerS || v == CardValueJokerB {
			return PlayResult{Valid: false} // 顺子不能有2和王牌
		}
		values = append(values, v)
	}
	slices.Sort(values)

	// 至少5张
	if len(values) < 5 {
		return PlayResult{Valid: false}
	}

	// 检查连续性
	for i := 1; i < len(values); i++ {
		if values[i] != values[i-1]+1 {
			return PlayResult{Valid: false}
		}
	}

	return PlayResult{
		Valid:   true,
		Pattern: PatternStraight,
		Main:    values[len(values)-1],
		Length:  len(values),
	}
}

// isStraightPair 判断是否为连对
func isStraightPair(cards []Card, counts map[CardValue]int) PlayResult {
	// 连对要求：至少3对连续的对子，不含2和王牌
	pairValues := make([]CardValue, 0)
	for v, c := range counts {
		if c != 2 {
			return PlayResult{Valid: false} // 必须都是对子
		}
		if v >= CardValue2 || v == CardValueJokerS || v == CardValueJokerB {
			return PlayResult{Valid: false} // 不能有2和王牌
		}
		pairValues = append(pairValues, v)
	}

	if len(pairValues) < 3 {
		return PlayResult{Valid: false}
	}

	slices.Sort(pairValues)

	// 检查连续性
	for i := 1; i < len(pairValues); i++ {
		if pairValues[i] != pairValues[i-1]+1 {
			return PlayResult{Valid: false}
		}
	}

	return PlayResult{
		Valid:   true,
		Pattern: PatternStraightPair,
		Main:    pairValues[len(pairValues)-1],
		Length:  len(pairValues) * 2,
	}
}

// isTripleOne 三带一
func isTripleOne(cards []Card, counts map[CardValue]int) PlayResult {
	if len(counts) != 2 {
		return PlayResult{Valid: false}
	}

	var tripleVal, singleVal CardValue
	for v, c := range counts {
		if c == 3 {
			tripleVal = v
		} else if c == 1 {
			singleVal = v
		} else {
			return PlayResult{Valid: false}
		}
	}

	if tripleVal == 0 || singleVal == 0 {
		return PlayResult{Valid: false}
	}
	if tripleVal < CardValue3 || tripleVal > CardValue2 {
		return PlayResult{Valid: false}
	}

	return PlayResult{
		Valid:   true,
		Pattern: PatternTripleOne,
		Main:    tripleVal,
		Length:  4,
	}
}

// isTripleTwo 三带二
func isTripleTwo(cards []Card, counts map[CardValue]int) PlayResult {
	if len(counts) != 2 {
		return PlayResult{Valid: false}
	}

	var tripleVal CardValue
	pairCount := 0
	for v, c := range counts {
		if c == 3 {
			tripleVal = v
		} else if c == 2 {
			pairCount++
		} else {
			return PlayResult{Valid: false}
		}
	}

	if tripleVal == 0 || pairCount != 1 {
		return PlayResult{Valid: false}
	}
	if tripleVal < CardValue3 || tripleVal > CardValue2 {
		return PlayResult{Valid: false}
	}

	return PlayResult{
		Valid:   true,
		Pattern: PatternTripleTwo,
		Main:    tripleVal,
		Length:  5,
	}
}

// isAirplane 飞机（不带翅膀）
func isAirplane(cards []Card, counts map[CardValue]int) PlayResult {
	tripleValues := make([]CardValue, 0)
	for v, c := range counts {
		if c != 3 {
			return PlayResult{Valid: false} // 必须都是三条
		}
		if v < CardValue3 || v > CardValue2 {
			return PlayResult{Valid: false} // 不能有2和王牌
		}
		tripleValues = append(tripleValues, v)
	}

	if len(tripleValues) < 2 {
		return PlayResult{Valid: false}
	}

	slices.Sort(tripleValues)

	// 检查连续性
	for i := 1; i < len(tripleValues); i++ {
		if tripleValues[i] != tripleValues[i-1]+1 {
			return PlayResult{Valid: false}
		}
	}

	return PlayResult{
		Valid:   true,
		Pattern: PatternAirplane,
		Main:    tripleValues[len(tripleValues)-1],
		Length:  len(tripleValues) * 3,
	}
}

// isAirplaneWithWings 飞机带翅膀
func isAirplaneWithWings(cards []Card, counts map[CardValue]int) PlayResult {
	// 找出所有三条
	tripleValues := make([]CardValue, 0)
	wingCards := 0

	for v, c := range counts {
		if c == 3 {
			if v < CardValue3 || v > CardValue2 {
				return PlayResult{Valid: false}
			}
			tripleValues = append(tripleValues, v)
		} else if c == 1 || c == 2 {
			wingCards += c
		} else if c == 4 {
			// 四条可以拆成三条+单牌，或者作为翅膀
			tripleValues = append(tripleValues, v)
			wingCards++ // 多出一张作为翅膀
		} else {
			return PlayResult{Valid: false}
		}
	}

	if len(tripleValues) < 2 {
		return PlayResult{Valid: false}
	}

	slices.Sort(tripleValues)

	// 检查三条的连续性
	for i := 1; i < len(tripleValues); i++ {
		if tripleValues[i] != tripleValues[i-1]+1 {
			return PlayResult{Valid: false}
		}
	}

	// 翅膀数量必须等于三条数量 × (1或2)
	tripleCount := len(tripleValues)
	expectedWingsSingle := tripleCount   // 飞机带单
	expectedWingsPair := tripleCount * 2 // 飞机带对

	if wingCards != expectedWingsSingle && wingCards != expectedWingsPair {
		return PlayResult{Valid: false}
	}

	return PlayResult{
		Valid:   true,
		Pattern: PatternAirplaneWings,
		Main:    tripleValues[len(tripleValues)-1],
		Length:  len(cards),
	}
}

// countValues 统计每种牌值的数量
func countValues(cards []Card) map[CardValue]int {
	counts := make(map[CardValue]int)
	for _, c := range cards {
		counts[c.Value]++
	}
	return counts
}
