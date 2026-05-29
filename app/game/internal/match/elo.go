package match

// TierInfo 段位信息
type TierInfo struct {
	Name  string
	Min   int32
	Max   int32
}

// 段位定义
var Tiers = []TierInfo{
	{Name: "青铜I", Min: 0, Max: 299},
	{Name: "青铜II", Min: 300, Max: 599},
	{Name: "青铜III", Min: 600, Max: 899},
	{Name: "白银I", Min: 900, Max: 1199},
	{Name: "白银II", Min: 1200, Max: 1499},
	{Name: "白银III", Min: 1500, Max: 1799},
	{Name: "黄金I", Min: 1800, Max: 2099},
	{Name: "黄金II", Min: 2100, Max: 2399},
	{Name: "黄金III", Min: 2400, Max: 2699},
	{Name: "铂金I", Min: 2700, Max: 2999},
	{Name: "铂金II", Min: 3000, Max: 3299},
	{Name: "铂金III", Min: 3300, Max: 3599},
	{Name: "大师", Min: 3600, Max: 99999},
}

// GetTier 根据 ELO 获取段位
func GetTier(elo int32) string {
	for _, t := range Tiers {
		if elo >= t.Min && elo <= t.Max {
			return t.Name
		}
	}
	return "青铜I"
}

// GetTierIndex 获取段位索引
func GetTierIndex(tier string) int {
	for i, t := range Tiers {
		if t.Name == tier {
			return i
		}
	}
	return 0
}

// ELOChange ELO 变化计算
type ELOChange struct {
	OldELO  int32
	NewELO  int32
	Delta   int32
	OldTier string
	NewTier string
	Promoted bool
	Demoted  bool
}

// CalculateELO 计算 ELO 变化
// isWinner: 是否获胜方
// isLandlord: 是否是地主
// isLandlordWin: 地主是否获胜
// baseScore: 基础分
// multiplier: 倍数
// elo: 当前 ELO
// opponentELO: 对手平均 ELO（用于系数计算）
func CalculateELO(isWinner, isLandlord, isLandlordWin bool, baseScore, multiplier int32, elo, opponentELO int32) ELOChange {
	oldTier := GetTier(elo)

	// 计算系数
	coef := float32(1.0)
	diff := opponentELO - elo
	if diff > 100 {
		coef = 1.2 // 对手更强，赢更多/输更少
	} else if diff < -100 {
		coef = 0.8 // 对手更弱，赢更少/输更多
	}

	// 基础变化
	var delta int32
	if isLandlordWin {
		if isLandlord {
			delta = int32(float32(baseScore*multiplier*2) * coef)
		} else {
			delta = -int32(float32(baseScore*multiplier) * coef)
		}
	} else {
		if isLandlord {
			delta = -int32(float32(baseScore*multiplier*2) * coef)
		} else {
			delta = int32(float32(baseScore*multiplier) * coef)
		}
	}

	newELO := elo + delta
	newTier := GetTier(newELO)

	return ELOChange{
		OldELO:   elo,
		NewELO:   newELO,
		Delta:    delta,
		OldTier:  oldTier,
		NewTier:  newTier,
		Promoted: GetTierIndex(newTier) > GetTierIndex(oldTier),
		Demoted:  GetTierIndex(newTier) < GetTierIndex(oldTier),
	}
}
