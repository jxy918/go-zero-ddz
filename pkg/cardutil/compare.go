package cardutil

// ComparePlays 比较两手牌的大小
// 返回:
//
//	> 0: play1 大
//	= 0: 一样大（不可能，因为同类型同主牌值不会同时出现）
//	< 0: play2 大
//
// 规则：
// 1. 同类型比较主牌值
// 2. 炸弹 > 非炸弹
// 3. 王炸 > 炸弹
// 4. 不同类型且都不是炸弹 → 无法比较（返回 0）
func ComparePlays(play1, play2 PlayResult) int {
	if !play1.Valid || !play2.Valid {
		return 0
	}

	// 王炸最大
	if play1.Pattern == PatternRocket && play2.Pattern == PatternRocket {
		return 0 // 都是王炸，一样大（实际不会出现）
	}
	if play1.Pattern == PatternRocket {
		return 1
	}
	if play2.Pattern == PatternRocket {
		return -1
	}

	// 炸弹比较
	if play1.Pattern.IsBomb() && play2.Pattern.IsBomb() {
		// 都是炸弹，比较主牌值
		return int(play1.Main) - int(play2.Main)
	}
	if play1.Pattern.IsBomb() {
		return 1 // 炸弹 > 非炸弹
	}
	if play2.Pattern.IsBomb() {
		return -1 // 非炸弹 < 炸弹
	}

	// 特殊牌型比较：三带一可以大过三张，三带二可以大过三带一和三张
	// 规则：带牌类型可以大过不带牌的同类（三带一 > 三张，三带二 > 三带一 > 三张）
	if play1.Pattern == PatternTripleOne && play2.Pattern == PatternTriple {
		return 1 // 三带一大过三张
	}
	if play1.Pattern == PatternTriple && play2.Pattern == PatternTripleOne {
		return -1 // 三张小不过三带一
	}
	if play1.Pattern == PatternTripleTwo && (play2.Pattern == PatternTripleOne || play2.Pattern == PatternTriple) {
		return 1 // 三带二大过三带一和三张
	}
	if (play1.Pattern == PatternTripleOne || play1.Pattern == PatternTriple) && play2.Pattern == PatternTripleTwo {
		return -1 // 三带一和三张小不过三带二
	}

	// 同类型比较
	if play1.Pattern != play2.Pattern {
		return 0 // 不同类型且都不是炸弹，无法比较
	}

	// 同类型比较主牌值
	return int(play1.Main) - int(play2.Main)
}

// CanBeat 判断 play1 是否能大过 play2
// play2 是上一手出的牌，play1 是当前要出的牌
// 规则：
// 1. 炸弹可以大过任何非炸弹牌型
// 2. 王炸可以大过任何牌（包括炸弹）
// 3. 非炸弹牌型必须牌型相同才能比较大小
// 4. 三带一 > 三张，三带二 > 三带一和三张
func CanBeat(play1, play2 PlayResult) bool {
	if !play1.Valid || !play2.Valid {
		return false
	}

	// 王炸最大
	if play1.Pattern == PatternRocket {
		return true
	}

	// 炸弹可以大过非炸弹
	if play1.Pattern.IsBomb() && !play2.Pattern.IsBomb() {
		return true
	}

	// 都是炸弹，比较大小
	if play1.Pattern.IsBomb() && play2.Pattern.IsBomb() {
		return play1.Main > play2.Main
	}

	// 三带一可以大过三张
	if play1.Pattern == PatternTripleOne && play2.Pattern == PatternTriple {
		return play1.Main > play2.Main
	}

	// 三带二可以大过三带一和三张
	if play1.Pattern == PatternTripleTwo {
		if play2.Pattern == PatternTriple || play2.Pattern == PatternTripleOne {
			return play1.Main > play2.Main
		}
	}

	// 非炸弹牌型必须牌型相同才能比较
	if play1.Pattern != play2.Pattern {
		return false
	}

	// 同类型比较主牌值
	return play1.Main > play2.Main
}

// CompareCardValues 比较两张单牌的大小
func CompareCardValues(v1, v2 CardValue) int {
	return int(v1) - int(v2)
}

// IsBigger 判断牌值 v1 是否大于 v2
func IsBigger(v1, v2 CardValue) bool {
	return v1 > v2
}
