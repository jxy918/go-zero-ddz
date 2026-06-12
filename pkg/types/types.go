package types

// PlayerSettlement 玩家结算
type PlayerSettlement struct {
	UID         string
	IsLandlord  bool
	IsBot       bool
	ScoreChange int32
	NewELO      int32
	NewTier     string
	IsPromoted  bool
	IsDemoted   bool
}

// SettlementResult 结算结果
type SettlementResult struct {
	WinnerUID       string
	WinnerSide      WinnerSide
	IsSpring        bool
	IsCounterSpring bool
	Multiplier      int32
	BaseScore       int32
	PlayerResults   map[string]*PlayerSettlement
}

// CallRecord 叫地主记录
type CallRecord struct {
	UID    string
	Action int   // 0=pass, 1=call
	Score  int32 // 1-3
}

// PlayerInfo 玩家信息
type PlayerInfo struct {
	UID            string
	Nickname       string
	IsLandlord     bool
	IsBot          bool
	IsAIControlled bool
	CardCount      int
}

// MatchResult 匹配结果
type MatchResult struct {
	RoomID    string
	PlayerIDs []string
}
