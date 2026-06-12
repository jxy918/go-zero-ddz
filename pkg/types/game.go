package types

// PlayerRole 玩家角色
type PlayerRole int

const (
	RoleUnknown  PlayerRole = 0
	RoleLandlord PlayerRole = 1 // 地主
	RolePeasant  PlayerRole = 2 // 农民
)

// RoomState 房间状态
type RoomState int

const (
	StateWaiting    RoomState = 0 // 等待中
	StateDealing    RoomState = 1 // 发牌中
	StateCalling    RoomState = 2 // 叫地主中
	StatePlaying    RoomState = 3 // 出牌中
	StateSettlement RoomState = 4 // 结算中
)

// WinnerSide 胜利方
type WinnerSide int

const (
	WinnerSideLandlord WinnerSide = 0
	WinnerSidePeasant  WinnerSide = 1
)

// AIDifficulty AI难度等级
type AIDifficulty int

const (
	AIEasy   AIDifficulty = 1 // 简单
	AINormal AIDifficulty = 2 // 普通
	AIHard   AIDifficulty = 3 // 困难
)

// 游戏常量
const (
	MaxRoomSize       = 3  // 房间最大人数
	MinRoomSize       = 2  // 房间最小人数（可以开始游戏）
	TotalCards        = 54 // 总牌数
	LandlordCards     = 20 // 地主牌数
	PeasantCards      = 17 // 农民牌数
	BottomCards       = 3  // 底牌数
	CallLandlordTimer = 15 // 叫地主超时时间（秒）
	PlayCardsTimer    = 15 // 出牌超时时间（秒）
	MatchTimeout      = 30 // 匹配超时时间（秒）
)

// PlayAction 出牌动作类型
type PlayAction string

const (
	PlayActionPlay  PlayAction = "play"  // 出牌
	PlayActionPass  PlayAction = "pass"  // 跳过（Pass）
)

// PlayRoundRecord 打牌轮次记录
type PlayRoundRecord struct {
	RoundIndex int          `json:"round_index"` // 轮次索引
	PlayerUID  string       `json:"player_uid"`  // 出牌玩家UID
	Action     PlayAction   `json:"action"`      // 动作类型
	Cards      []int        `json:"cards"`       // 牌面（牌值数组）
	Pattern    string       `json:"pattern"`     // 牌型
	Timestamp  int64        `json:"timestamp"`   // 时间戳
}
