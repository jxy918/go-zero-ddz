package room

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"go-zero-ddz/pkg/cardutil"
)

// GenerateID 生成房间 ID
func GenerateID() string {
	return fmt.Sprintf("room-%d", time.Now().UnixNano())
}

// Player 玩家
type Player struct {
	UID            string
	Nickname       string
	AvatarID       uint32
	ELO            int32
	Tier           string
	IsBot          bool
	IsReady        bool
	IsOnline       bool
	IsLandlord     bool
	IsAIControlled bool
	Cards          []cardutil.Card
	Role           PlayerRole
	DisconnectAt   time.Time // 断线时间
}

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

// Room 房间
type Room struct {
	ID        string
	State     RoomState
	Players   map[string]*Player // UID → Player
	PlayerIDs []string           // 玩家顺序（用于回合计算）

	// 游戏数据
	LandlordUID    string
	CurrentTurnUID string
	Timer          int
	BaseScore      int32
	Multiplier     int32
	CallScore      int32

	// 出牌记录
	LastPlayedCards []cardutil.Card
	LastPlayedUID   string
	LastPattern     cardutil.CardPattern
	PassCount       int
	IsLastRound     bool // 连续 PASS 后标记，表示需要回到最后出牌的人

	// 底牌
	BottomCards []cardutil.Card

	// 计时器
	timer         *time.Timer
	timerLock     sync.Mutex
	botJoinTimer  *time.Timer
	botJoinCancel context.CancelFunc

	// 回调
	OnStateChange      func(room *Room, oldState, newState RoomState)
	OnTimeout          func(room *Room, uid string)
	OnBotJoinTimeout   func(room *Room)              // 机器人加入超时回调
	OnBotJoinCountdown func(room *Room, seconds int) // 机器人加入倒计时回调

	// 游戏状态机（游戏开始后创建）
	GameState *GameStateMachine

	mu sync.RWMutex
}

// NewRoom 创建新房间
func NewRoom(id string) *Room {
	return &Room{
		ID:         id,
		State:      StateWaiting,
		Players:    make(map[string]*Player),
		PlayerIDs:  make([]string, 0, 3),
		BaseScore:  1,
		Multiplier: 1,
	}
}

// InitGameState 初始化游戏状态机（开始游戏时调用）
func (r *Room) InitGameState() *GameStateMachine {
	// 重置所有玩家的AI托管状态
	r.mu.Lock()
	for _, player := range r.Players {
		if !player.IsBot {
			player.IsAIControlled = false
			log.Printf("Room %s: InitGameState reset player %s IsAIControlled to false", r.ID, player.UID)
		}
	}
	r.mu.Unlock()

	gsm := NewGameStateMachine(r)
	r.mu.Lock()
	r.GameState = gsm
	r.mu.Unlock()
	return gsm
}

// GetGameState 获取游戏状态机
func (r *Room) GetGameState() *GameStateMachine {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.GameState
}

// AddPlayer 添加玩家
func (r *Room) AddPlayer(player *Player) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if len(r.Players) >= 3 {
		return fmt.Errorf("room is full")
	}

	if _, exists := r.Players[player.UID]; exists {
		return fmt.Errorf("player already in room")
	}

	r.Players[player.UID] = player
	r.PlayerIDs = append(r.PlayerIDs, player.UID)

	return nil
}

// RemovePlayer 移除玩家
func (r *Room) RemovePlayer(uid string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.Players[uid]; !exists {
		return fmt.Errorf("player not in room")
	}

	delete(r.Players, uid)
	r.PlayerIDs = removeString(r.PlayerIDs, uid)

	return nil
}

// GetPlayer 获取玩家
func (r *Room) GetPlayer(uid string) (*Player, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	p, exists := r.Players[uid]
	return p, exists
}

// IsFull 房间是否满员
func (r *Room) IsFull() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.Players) >= 3
}

// AllReady 所有玩家是否都准备
func (r *Room) AllReady() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if len(r.Players) < 3 {
		return false
	}

	for _, p := range r.Players {
		if !p.IsReady {
			return false
		}
	}
	return true
}

// ResetPlayersState 重置所有玩家的状态
func (r *Room) ResetPlayersState() {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, player := range r.Players {
		player.IsLandlord = false
		player.IsAIControlled = false
		player.Cards = nil
		player.Role = RoleUnknown
	}
	log.Printf("Room %s: all players state reset", r.ID)
}

// SetReady 设置玩家准备状态
func (r *Room) SetReady(uid string, ready bool) error {
	log.Printf("Room %s: SetReady acquiring write lock", r.ID)
	r.mu.Lock()
	log.Printf("Room %s: SetReady acquired write lock", r.ID)
	defer func() {
		log.Printf("Room %s: SetReady releasing write lock", r.ID)
		r.mu.Unlock()
	}()

	player, exists := r.Players[uid]
	if !exists {
		log.Printf("SetReady: player %s not found in room", uid)
		return fmt.Errorf("player not found")
	}

	oldReady := player.IsReady
	player.IsReady = ready
	log.Printf("SetReady: player %s changed from %v to %v", uid, oldReady, ready)
	return nil
}

// SetOnline 设置玩家在线状态
func (r *Room) SetOnline(uid string, online bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if p, exists := r.Players[uid]; exists {
		p.IsOnline = online
		if !online {
			p.DisconnectAt = time.Now()
		} else {
			p.DisconnectAt = time.Time{}
		}
	}
}

// StartTimer 启动倒计时
func (r *Room) StartTimer(seconds int, uid string) {
	r.mu.Lock()
	r.Timer = seconds
	r.CurrentTurnUID = uid
	r.mu.Unlock()

	r.timerLock.Lock()
	defer r.timerLock.Unlock()

	if r.timer != nil {
		r.timer.Stop()
	}

	r.timer = time.AfterFunc(time.Duration(seconds)*time.Second, func() {
		if r.OnTimeout != nil {
			r.OnTimeout(r, uid)
		}
	})
}

// StopTimer 停止倒计时
func (r *Room) StopTimer() {
	r.timerLock.Lock()
	defer r.timerLock.Unlock()

	if r.timer != nil {
		r.timer.Stop()
		r.timer = nil
	}
}

// StartBotJoinTimer 启动机器人加入计时器
func (r *Room) StartBotJoinTimer(timeout int) {
	r.timerLock.Lock()
	log.Printf("Room %s: starting bot join timer with %d seconds timeout", r.ID, timeout)

	// 如果已有计时器，先停止
	if r.botJoinTimer != nil {
		r.botJoinTimer.Stop()
	}
	if r.botJoinCancel != nil {
		r.botJoinCancel()
	}

	ctx, cancel := context.WithCancel(context.Background())
	r.botJoinCancel = cancel

	// 保存回调引用，防止在 goroutine 中被修改
	onCountdown := r.OnBotJoinCountdown
	onTimeout := r.OnBotJoinTimeout
	roomID := r.ID
	room := r // 保存 room 引用

	// 解锁！不要在锁内启动 goroutine！
	r.timerLock.Unlock()

	// 启动一个统一的计时器来处理倒计时和超时
	go func(roomID string, countdownFunc func(*Room, int), timeoutFunc func(*Room), roomRef *Room) {
		// 先发送初始倒计时
		if countdownFunc != nil {
			log.Printf("Room %s: calling countdown callback for %d seconds", roomID, timeout)
			countdownFunc(roomRef, timeout)
			log.Printf("Room %s: countdown callback completed for %d seconds", roomID, timeout)
		}

		// 每秒倒计时
		for i := timeout - 1; i > 0; i-- {
			select {
			case <-ctx.Done():
				log.Printf("Room %s: bot join timer canceled", roomID)
				return
			case <-time.After(time.Second):
				log.Printf("Room %s: bot join countdown: %d seconds left", roomID, i)
				// 调用倒计时回调
				if countdownFunc != nil {
					log.Printf("Room %s: calling countdown callback for %d seconds", roomID, i)
					countdownFunc(roomRef, i)
					log.Printf("Room %s: countdown callback completed for %d seconds", roomID, i)
				}
			}
		}

		// 等待最后一秒
		select {
		case <-ctx.Done():
			log.Printf("Room %s: bot join timer canceled", roomID)
			return
		case <-time.After(time.Second):
			log.Printf("Room %s: bot join timer expired, about to call timeout callback", roomID)
			// 超时了，添加机器人
			if timeoutFunc != nil {
				log.Printf("Room %s: calling timeout callback", roomID)
				timeoutFunc(roomRef)
				log.Printf("Room %s: timeout callback completed", roomID)
			} else {
				log.Printf("Room %s: timeout callback is nil!", roomID)
			}
		}
	}(roomID, onCountdown, onTimeout, room)

	// 设置一个空的timer（我们自己用goroutine处理）
	r.timerLock.Lock()
	r.botJoinTimer = time.AfterFunc(24*time.Hour, func() {})
	r.timerLock.Unlock()
}

// StopBotJoinTimer 停止机器人加入计时器
func (r *Room) StopBotJoinTimer() {
	r.timerLock.Lock()
	defer r.timerLock.Unlock()

	if r.botJoinTimer != nil {
		r.botJoinTimer.Stop()
		r.botJoinTimer = nil
	}
	if r.botJoinCancel != nil {
		r.botJoinCancel()
		r.botJoinCancel = nil
	}
}

// SetState 设置房间状态（外部调用，需要加锁）
func (r *Room) SetState(newState RoomState) {
	r.mu.Lock()
	oldState := r.State
	r.State = newState
	r.mu.Unlock()

	if r.OnStateChange != nil {
		r.OnStateChange(r, oldState, newState)
	}
}

// setStateLocked 设置房间状态（内部调用，假设已持有锁）
func (r *Room) setStateLocked(newState RoomState) RoomState {
	oldState := r.State
	r.State = newState
	return oldState
}

// NextTurn 获取下一个回合玩家（逆时针方向）
func (r *Room) NextTurn(currentUID string) string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for i, uid := range r.PlayerIDs {
		if uid == currentUID {
			// 逆时针方向：i-1，处理边界情况
			nextIdx := (i - 1 + len(r.PlayerIDs)) % len(r.PlayerIDs)
			return r.PlayerIDs[nextIdx]
		}
	}
	return ""
}

// GetPlayerOrder 获取玩家顺序索引
func (r *Room) GetPlayerOrder(uid string) int {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for i, id := range r.PlayerIDs {
		if id == uid {
			return i
		}
	}
	return -1
}

// GetLandlordOrder 获取地主的顺序索引
func (r *Room) GetLandlordOrder() int {
	return r.GetPlayerOrder(r.LandlordUID)
}

// IsPeasant 判断是否为农民
func (r *Room) IsPeasant(uid string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	p, exists := r.Players[uid]
	return exists && !p.IsLandlord
}

// GetOtherPlayer 获取另一个农民（当前玩家是农民时）
func (r *Room) GetOtherPeasant(currentUID string) (string, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for uid, p := range r.Players {
		if uid != currentUID && !p.IsLandlord {
			return uid, true
		}
	}
	return "", false
}

// Count 获取玩家数量
func (r *Room) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.Players)
}

// GetCardCounts 获取各玩家的手牌数量
func (r *Room) GetCardCounts() map[string]int {
	r.mu.RLock()
	defer r.mu.RUnlock()

	counts := make(map[string]int)
	for uid, p := range r.Players {
		counts[uid] = len(p.Cards)
	}
	return counts
}

// removeString 从切片中移除元素
func removeString(slice []string, item string) []string {
	result := make([]string, 0, len(slice))
	for _, s := range slice {
		if s != item {
			result = append(result, s)
		}
	}
	return result
}
