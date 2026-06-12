package room

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"

	"go-zero-ddz/app/game/internal/ai"
	"go-zero-ddz/app/game/internal/websocket"
	"go-zero-ddz/pkg/cardutil"
	"go-zero-ddz/pkg/types"
)

// Manager 房间管理器
type Manager struct {
	rooms   map[string]*Room
	roomsMu sync.RWMutex

	redis  redis.UniversalClient
	config *RoomConfig

	// AI 引擎
	aiEngine *ai.AIEngine
	hub      *websocket.Hub
	rng      *rand.Rand

	ctx    context.Context
	cancel context.CancelFunc

	// 回调函数
	onStartGame            func(room *Room)
	onBotPlayerJoined      func(roomID, uid, nickname string, isBot, isReady bool)
	onRoomBotJoinCountdown func(room *Room, seconds int)
	onGameEnd              func(room *Room, gsm *GameStateMachine) // 游戏结束回调（保存数据库）
}

// RoomConfig 房间配置
type RoomConfig struct {
	MaxRooms          int
	MaxPlayersPerRoom int
	ReadyTimeout      int
	PlayTimeout       int
	ReconnectTimeout  int
	SnapshotInterval  int
	BotJoinTimeout    int // 机器人加入超时时间（秒），默认60秒
}

// NewManager 创建房间管理器
func NewManager(rdb redis.UniversalClient, cfg *RoomConfig, aiEngine *ai.AIEngine, hub *websocket.Hub) *Manager {
	ctx, cancel := context.WithCancel(context.Background())

	m := &Manager{
		rooms:    make(map[string]*Room),
		redis:    rdb,
		config:   cfg,
		aiEngine: aiEngine,
		hub:      hub,
		rng:      rand.New(rand.NewSource(time.Now().UnixNano())),
		ctx:      ctx,
		cancel:   cancel,
	}

	// 启动定期快照
	go m.snapshotLoop()

	return m
}

// SetOnStartGame 设置开始游戏回调
func (m *Manager) SetOnStartGame(callback func(room *Room)) {
	m.onStartGame = callback
}

// SetOnBotPlayerJoined 设置机器人加入回调
func (m *Manager) SetOnBotPlayerJoined(callback func(roomID, uid, nickname string, isBot, isReady bool)) {
	m.onBotPlayerJoined = callback
}

// SetOnRoomBotJoinCountdown 设置房间机器人加入倒计时回调
func (m *Manager) SetOnRoomBotJoinCountdown(callback func(room *Room, seconds int)) {
	m.roomsMu.Lock()
	defer m.roomsMu.Unlock()
	// 保存回调引用
	m.onRoomBotJoinCountdown = callback
	// 更新所有现有房间的回调
	for _, room := range m.rooms {
		room.OnBotJoinCountdown = callback
	}
}

// SetOnGameEnd 设置游戏结束回调
func (m *Manager) SetOnGameEnd(callback func(room *Room, gsm *GameStateMachine)) {
	m.onGameEnd = callback
}

// CreateRoom 创建房间
func (m *Manager) CreateRoom(id string) (*Room, error) {
	log.Printf("[DEBUG] CreateRoom called for id=%s", id)

	log.Printf("[DEBUG] CreateRoom: acquiring roomsMu.Lock()")
	m.roomsMu.Lock()
	log.Printf("[DEBUG] CreateRoom: acquired roomsMu.Lock()")

	defer func() {
		log.Printf("[DEBUG] CreateRoom: releasing roomsMu.Lock()")
		m.roomsMu.Unlock()
	}()

	if len(m.rooms) >= m.config.MaxRooms {
		return nil, fmt.Errorf("max rooms reached")
	}

	if _, exists := m.rooms[id]; exists {
		return nil, fmt.Errorf("room already exists")
	}

	log.Printf("[DEBUG] CreateRoom: calling NewRoom(%s)", id)
	room := NewRoom(id)
	log.Printf("[DEBUG] CreateRoom: NewRoom returned")

	m.rooms[id] = room
	log.Printf("[DEBUG] CreateRoom: room added to map")

	// 设置回调
	log.Printf("[DEBUG] CreateRoom: setting callbacks")
	room.OnStateChange = m.onStateChange
	log.Printf("[DEBUG] CreateRoom: OnStateChange set")
	room.OnTimeout = m.onTimeout
	log.Printf("[DEBUG] CreateRoom: OnTimeout set")
	room.OnBotJoinTimeout = m.onBotJoinTimeout
	log.Printf("[DEBUG] CreateRoom: OnBotJoinTimeout set")
	room.OnBotJoinCountdown = m.onRoomBotJoinCountdown
	log.Printf("[DEBUG] CreateRoom: OnBotJoinCountdown set")

	log.Printf("Room %s created (total: %d)", id, len(m.rooms))

	return room, nil
}

// GetRoom 获取房间
func (m *Manager) GetRoom(id string) (*Room, bool) {
	m.roomsMu.RLock()
	defer m.roomsMu.RUnlock()

	room, exists := m.rooms[id]
	return room, exists
}

// RemoveRoom 移除房间
func (m *Manager) RemoveRoom(id string) {
	m.roomsMu.Lock()
	defer m.roomsMu.Unlock()

	if room, exists := m.rooms[id]; exists {
		room.StopTimer()
		delete(m.rooms, id)
		log.Printf("Room %s removed (total: %d)", id, len(m.rooms))
	}
}

// GetRoomCount 获取房间数量
func (m *Manager) GetRoomCount() int {
	m.roomsMu.RLock()
	defer m.roomsMu.RUnlock()
	return len(m.rooms)
}

// onStateChange 状态变更回调
func (m *Manager) onStateChange(room *Room, oldState, newState types.RoomState) {
	log.Printf("Room %s state changed: %v → %v", room.ID, oldState, newState)

	// 如果游戏开始，停止机器人加入计时器
	if newState != types.StateWaiting {
		log.Printf("Room %s game started, stopping bot join timer", room.ID)
		room.StopBotJoinTimer()
	}

	// 保存快照到 Redis
	go m.saveSnapshot(room)

	// 根据状态执行相应逻辑
	switch newState {
	case types.StateDealing:
		// 开始发牌
	case types.StateCalling:
		// 开始叫地主，启动倒计时
		m.startCallTimer(room)
	case types.StatePlaying:
		// 开始出牌阶段，根据当前玩家类型处理
		room.mu.RLock()
		currentUID := room.CurrentTurnUID
		currentPlayer, exists := room.Players[currentUID]
		isBot := exists && currentPlayer.IsBot
		isAIControlled := exists && currentPlayer.IsAIControlled
		room.mu.RUnlock()

		if exists {
			if isBot || isAIControlled {
				// 如果是机器人或AI控制的玩家，延迟触发AI操作，不启动计时器
				log.Printf("onStateChange: triggering bot for player %s", currentUID)
				m.triggerBotIfNeeded(room)
			} else {
				// 如果是真人玩家，延迟启动计时器（给玩家足够时间看底牌）
				// Bug 1 修复：确认地主后延迟5秒再启动出牌计时器
				const landlordShowDelay = 5 // 秒
				log.Printf("onStateChange: delaying timer start for human player %s by %d seconds (to show bottom cards)", currentUID, landlordShowDelay)
				time.AfterFunc(time.Duration(landlordShowDelay)*time.Second, func() {
					// 再次检查状态，确保仍在出牌阶段
					room.mu.RLock()
					currentState := room.State
					room.mu.RUnlock()
					if currentState == types.StatePlaying {
						room.StartTimer(m.config.PlayTimeout, currentUID)
						log.Printf("onStateChange: started timer for human player %s after delay", currentUID)
						// 广播定时器消息
						if m.hub != nil {
							payload := []byte(fmt.Sprintf(`{"remaining_seconds":%d,"current_turn_uid":"%s"}`,
								m.config.PlayTimeout, currentUID))
							m.hub.BroadcastToRoom(room.ID, 0x0408, payload)
						}
					}
				})
			}
		}
	case types.StateSettlement:
		// 结算
		room.StopTimer()
	}
}

// onBotJoinTimeout 机器人加入超时回调
func (m *Manager) onBotJoinTimeout(room *Room) {
	log.Printf("Room %s: onBotJoinTimeout called", room.ID)

	// 使用 channel 通知主协程处理，避免锁冲突
	go func() {
		log.Printf("Room %s: bot join timeout goroutine started", room.ID)

		// 等待一小段时间，让其他操作完成
		time.Sleep(200 * time.Millisecond)

		// 直接在锁外检查状态（不加锁）
		log.Printf("Room %s: checking state without lock - Players len=%d, State=%v", room.ID, len(room.Players), room.State)

		// 如果游戏已经开始，直接返回
		if room.State != types.StateWaiting {
			log.Printf("Room %s: game already started (state=%v), not adding bots", room.ID, room.State)
			return
		}

		// 直接添加机器人（不加锁！）
		playerCount := len(room.Players)
		if playerCount >= 3 {
			log.Printf("Room %s: already full (%d/3), checking start game", room.ID, playerCount)
			m.checkStartGame(room)
			return
		}

		// 直接补满到3个机器人（不加锁！）
		botsToAdd := 3 - playerCount
		log.Printf("Room %s: adding %d bots (without lock)", room.ID, botsToAdd)

		for i := 0; i < botsToAdd; i++ {
			botIndex := playerCount + 1 + i
			botUID := fmt.Sprintf("bot-%s-%d", room.ID, botIndex)
			bot := &Player{
				UID:            botUID,
				Nickname:       fmt.Sprintf("机器人%d", botIndex),
				IsBot:          true,
				IsReady:        true,
				IsOnline:       true,
				IsLandlord:     false,
				IsAIControlled: false,
				Role:           types.RolePeasant,
			}

			// 直接添加（不加锁！）
			room.Players[botUID] = bot
			room.PlayerIDs = append(room.PlayerIDs, botUID)
			log.Printf("Room %s: added bot %s, total players now %d", room.ID, botUID, len(room.Players))

			// 广播机器人加入
			if m.onBotPlayerJoined != nil {
				log.Printf("Room %s: broadcasting bot %s joined", room.ID, botUID)
				m.onBotPlayerJoined(room.ID, botUID, bot.Nickname, true, true)
				log.Printf("Room %s: broadcast completed for bot %s", room.ID, botUID)
			}
		}

		log.Printf("Room %s: all bots added without lock, checking start game", room.ID)

		// 检查是否可以开始游戏
		m.checkStartGame(room)
	}()
}

// checkStartGame 检查房间是否满足开始游戏条件
func (m *Manager) checkStartGame(room *Room) {
	log.Printf("Room %s: checkStartGame called", room.ID)

	// 不加锁检查条件
	allReady := true
	for _, p := range room.Players {
		if !p.IsReady {
			allReady = false
			break
		}
	}
	playerCount := len(room.Players)
	state := room.State

	log.Printf("Room %s: checkStartGame result: allReady=%v playerCount=%d state=%v", room.ID, allReady, playerCount, state)

	if allReady && playerCount >= 3 && state == types.StateWaiting {
		log.Printf("Room %s all players ready and full, starting game", room.ID)
		// 停止机器人加入计时器
		room.StopBotJoinTimer()

		// 触发开始游戏逻辑
		if m.onStartGame != nil {
			log.Printf("Room %s: calling onStartGame callback", room.ID)
			m.onStartGame(room)
			log.Printf("Room %s: onStartGame callback completed", room.ID)
		} else {
			log.Printf("Room %s: onStartGame callback is nil", room.ID)
		}
	}
}

// PlayerJoined 玩家加入房间时调用
func (m *Manager) PlayerJoined(room *Room, uid string) {
	log.Printf("Room %s: player %s joined, restarting bot join timer", room.ID, uid)

	// 检查房间状态
	room.mu.RLock()
	state := room.State
	playerCount := len(room.Players)
	room.mu.RUnlock()

	if state != types.StateWaiting {
		log.Printf("Room %s not in waiting state, not restarting timer", room.ID)
		return
	}

	if playerCount >= 3 {
		log.Printf("Room %s already full, stopping bot join timer", room.ID)
		room.StopBotJoinTimer()
		// 检查是否可以开始游戏
		m.checkStartGame(room)
		return
	}

	// 重新启动计时器
	timeout := m.config.BotJoinTimeout
	if timeout <= 0 {
		timeout = 60
	}
	room.StartBotJoinTimer(timeout)
}

// PlayerReady 玩家准备时调用
func (m *Manager) PlayerReady(room *Room, uid string) {
	log.Printf("Room %s: player %s ready", room.ID, uid)

	room.mu.RLock()
	state := room.State
	playerCount := len(room.Players)
	// 检查所有已加入的玩家是否都准备了（不要求人数）
	allJoinedReady := true
	for _, p := range room.Players {
		if !p.IsReady {
			allJoinedReady = false
			break
		}
	}
	room.mu.RUnlock()

	// 如果所有已加入的玩家都准备好了，但人数不足3人
	if allJoinedReady && playerCount < 3 && state == types.StateWaiting {
		log.Printf("Room %s: all joined players ready but only %d/3 players, starting bot join timer", room.ID, playerCount)

		timeout := m.config.BotJoinTimeout
		if timeout <= 0 {
			timeout = 60
		}
		room.StartBotJoinTimer(timeout)
	}

	// 检查是否可以开始游戏
	m.checkStartGame(room)
}

// TriggerBotIfNeeded 如果当前轮到的玩家是 Bot 或被AI托管，触发 AI 操作（外部调用，安全加锁）
func (m *Manager) TriggerBotIfNeeded(room *Room) {
	room.mu.RLock()
	currentUID := room.CurrentTurnUID
	player, exists := room.Players[currentUID]
	isBot := exists && player.IsBot
	isAIControlled := exists && player.IsAIControlled
	currentState := room.State
	room.mu.RUnlock()

	log.Printf("TriggerBotIfNeeded called for room %s: currentUID=%s, exists=%v, isBot=%v, isAIControlled=%v, state=%d", room.ID, currentUID, exists, isBot, isAIControlled, currentState)

	// 只有真正的机器人才能自动操作，真人玩家即使被AI托管也需要等待超时
	if !exists || !isBot {
		log.Printf("TriggerBotIfNeeded: not a bot, returning (even if AI controlled, human players need timeout)")
		return
	}

	// Bot 玩家：延迟后自动操作
	minDelay := 3000
	maxDelay := 5000
	delay := time.Duration(minDelay+m.rng.Intn(maxDelay-minDelay)) * time.Millisecond
	log.Printf("TriggerBotIfNeeded: scheduling bot action for %s in %v (min=%d, max=%d)", currentUID, delay, minDelay, maxDelay)
	time.AfterFunc(delay, func() {
		log.Printf("TriggerBotIfNeeded: bot action triggered for %s", currentUID)
		room.mu.RLock()
		actualState := room.State
		actualUID := room.CurrentTurnUID
		room.mu.RUnlock()

		log.Printf("TriggerBotIfNeeded: actual state=%d, actualUID=%s", actualState, actualUID)

		if actualState == types.StateCalling {
			log.Printf("TriggerBotIfNeeded: calling botCallLandlord")
			room.mu.RLock()
			player, exists := room.Players[actualUID]
			isBot := exists && player.IsBot
			room.mu.RUnlock()
			m.botCallLandlord(room, actualUID, isBot)
		} else if actualState == types.StatePlaying {
			log.Printf("TriggerBotIfNeeded: calling onTimeout")
			m.onTimeout(room, actualUID)
		} else {
			log.Printf("TriggerBotIfNeeded: state is not calling or playing, doing nothing")
		}
	})
}

// triggerBotIfNeeded 检查并触发机器人操作（线程安全版本）
func (m *Manager) triggerBotIfNeeded(room *Room) {
	room.mu.RLock()
	currentUID := room.CurrentTurnUID
	player, exists := room.Players[currentUID]
	isBot := exists && player.IsBot
	isAIControlled := exists && player.IsAIControlled
	room.mu.RUnlock()

	if !exists || (!isBot && !isAIControlled) {
		return
	}

	// 机器人出牌延迟（给玩家足够看牌和反应时间）
	// Bug 7 修复：叫地主阶段 1500-2500ms，出牌阶段 3000-5000ms（让玩家看到底牌）
	room.mu.RLock()
	currentState := room.State
	room.mu.RUnlock()

	var delay time.Duration
	if currentState == types.StatePlaying {
		delay = time.Duration(3000+m.rng.Intn(2000)) * time.Millisecond
	} else {
		delay = time.Duration(1500+m.rng.Intn(1000)) * time.Millisecond
	}
	time.AfterFunc(delay, func() {
		room.mu.RLock()
		currentState := room.State
		room.mu.RUnlock()

		if currentState == types.StateCalling {
			m.botCallLandlord(room, currentUID, isBot)
		} else if currentState == types.StatePlaying {
			m.onTimeout(room, currentUID)
		}
	})
}

// triggerBotIfNeededLocked 内部调用，假设调用方已持有 room.mu 锁
// 注意：此函数假设调用方已持有 room.mu 锁，所以直接读取状态而不再获取锁
func (m *Manager) triggerBotIfNeededLocked(room *Room) {
	currentUID := room.CurrentTurnUID
	player, exists := room.Players[currentUID]
	if !exists || (!player.IsBot && !player.IsAIControlled) {
		return
	}

	isBot := player.IsBot

	// 机器人出牌延迟（给玩家足够看牌和反应时间）
	// Bug 7 修复：叫地主阶段 1500-2500ms，出牌阶段 3000-5000ms（让玩家看到底牌）
	// 注意：调用方已持有锁，直接读取状态
	currentState := room.State

	var delay time.Duration
	if currentState == types.StatePlaying {
		delay = time.Duration(3000+m.rng.Intn(2000)) * time.Millisecond
	} else {
		delay = time.Duration(1500+m.rng.Intn(1000)) * time.Millisecond
	}

	// 将需要的值拷贝到闭包中，避免在goroutine中访问已释放的锁
	uid := currentUID
	isBotCopy := isBot

	time.AfterFunc(delay, func() {
		room.mu.RLock()
		currentState := room.State
		room.mu.RUnlock()

		if currentState == types.StateCalling {
			m.botCallLandlord(room, uid, isBotCopy)
		} else if currentState == types.StatePlaying {
			m.onTimeout(room, uid)
		}
	})
}

// botCallLandlord 处理叫地主（支持机器人和真人超时后的AI替叫）
// isBot 参数用于区分是真正的机器人还是真人玩家超时后的AI替叫
func (m *Manager) botCallLandlord(room *Room, uid string, isBot bool) {
	room.mu.Lock()
	player, exists := room.Players[uid]
	callScore := int32(0)
	action := 0
	if exists && !player.IsLandlord && m.rng.Float64() < 0.7 {
		callScore = int32(m.rng.Intn(3) + 1)
		action = 1
	}
	room.mu.Unlock()

	if !exists {
		return
	}

	// 使用 GameState 处理叫地主
	gsm := room.GameState
	if gsm == nil {
		return
	}

	gsm.CallLandlord(uid, action, callScore)
	if isBot {
		log.Printf("Bot %s called: action=%d score=%d", uid, action, callScore)
	} else {
		log.Printf("Player %s timeout, AI called landlord: action=%d score=%d", uid, action, callScore)
	}

	// 通过 Hub 广播叫地主结果
	if m.hub != nil {
		if isBot {
			log.Printf("Room %s: broadcasting bot call result: uid=%s, action=%d, score=%d", room.ID, uid, action, callScore)
		} else {
			log.Printf("Room %s: broadcasting AI call result for timeout player: uid=%s, action=%d, score=%d", room.ID, uid, action, callScore)
		}
		payload := []byte(fmt.Sprintf(`{"uid":"%s","action":%d,"score":%d,"round":%d,"is_bot":%t}`,
			uid, action, callScore, gsm.CurrentCallRound(), isBot))
		m.hub.BroadcastToRoom(room.ID, 0x0403, payload)
	}

	// 检查是否所有玩家都叫完了
	if !gsm.AllCalled() {
		// 还没叫完，继续下一个玩家
		// 注意：需要获取锁来安全地读取 room.PlayerIDs
		room.mu.RLock()
		nextIdx := gsm.currentCallIdx
		if nextIdx < 0 || nextIdx >= len(room.PlayerIDs) {
			nextIdx = 0
		}
		nextUID := room.PlayerIDs[nextIdx]
		room.mu.RUnlock()

		roomID := room.ID
		hub := m.hub

		time.AfterFunc(time.Second*1, func() {
			room, exists := m.GetRoom(roomID)
			if !exists {
				return
			}

			// 关键检查：只有在叫地主阶段才发送"轮到叫地主"消息
			// 如果叫地主阶段已经结束，不发送消息
			room.mu.RLock()
			currentState := room.State
			room.mu.RUnlock()

			if currentState != types.StateCalling {
				log.Printf("Room %s: skipping 'next to call' notification, state=%d (not calling phase)", roomID, currentState)
				return
			}

			// 安全地更新当前回合玩家并启动计时器
			room.mu.Lock()
			room.CurrentTurnUID = nextUID
			room.mu.Unlock()
			room.StartTimer(15, nextUID)

			// 通知轮到下一个玩家叫地主（不包含 action 和 score，避免被误解为叫地主结果）
			if hub != nil {
				payload := []byte(fmt.Sprintf(`{"uid":"%s","turn":true}`, nextUID))
				hub.BroadcastToRoom(roomID, 0x0403, payload)
				payload = []byte(fmt.Sprintf(`{"remaining_seconds":15,"current_turn_uid":"%s"}`, nextUID))
				hub.BroadcastToRoom(roomID, 0x0408, payload)
			}

			// 如果下一个是机器人或已被AI托管的玩家，延迟触发（3 秒延迟给用户阅读"轮到"提示）
			room.mu.RLock()
			nextPlayer, exists := room.Players[nextUID]
			isNextBot := exists && (nextPlayer.IsBot || nextPlayer.IsAIControlled)
			isBotFlag := exists && nextPlayer.IsBot
			room.mu.RUnlock()

			if isNextBot {
				time.AfterFunc(3*time.Second, func() {
					m.botCallLandlord(room, nextUID, isBotFlag)
				})
			}
		})

		return
	}

	// 所有玩家都叫完了，确认地主
	if isBot {
		log.Printf("Bot %s called, all players have called, confirming landlord", uid)
	} else {
		log.Printf("Player %s AI called, all players have called, confirming landlord", uid)
	}
	if err := gsm.ConfirmLandlord(); err != nil {
		log.Printf("Failed to confirm landlord: %v", err)
		return
	}

	// 准备玩家信息
	room.mu.RLock()
	playersInfo := make([]map[string]interface{}, 0, len(room.PlayerIDs))
	for _, playerUID := range room.PlayerIDs {
		p, _ := room.Players[playerUID]
		playersInfo = append(playersInfo, map[string]interface{}{
			"uid":              playerUID,
			"nickname":         p.Nickname,
			"is_landlord":      p.IsLandlord,
			"is_bot":           p.IsBot,
			"is_ai_controlled": p.IsAIControlled,
		})
	}
	landlordUID := room.LandlordUID
	bottomCards := room.BottomCards
	room.mu.RUnlock()

	// 广播地主确认通知（包含底牌和当前回合玩家，用于前端显示出牌按钮）
	if m.hub != nil {
		bottomCardsJSON, _ := json.Marshal(bottomCards)
		playersJSON, _ := json.Marshal(playersInfo)
		// 关键：必须包含 current_turn_uid，否则前端不会显示出牌按钮
		// 同时包含 landlord_uid 和 bottom_cards 用于显示地主和底牌
		payload := []byte(fmt.Sprintf(`{"uid":"%s","action":1,"landlord_uid":"%s","players":%s,"bottom_cards":%s,"current_turn_uid":"%s"}`,
			landlordUID, landlordUID, string(playersJSON), string(bottomCardsJSON), landlordUID))
		m.hub.BroadcastToRoom(room.ID, 0x0403, payload)
	}

	// 等待 LandlordReady 信号（确保底牌已加入地主手牌）
	if gsm.LandlordReady != nil {
		<-gsm.LandlordReady
	}

	room.mu.RLock()
	landlordUIDFinal := room.LandlordUID
	// 此时 landlord.Cards 已经是 20 张（17 + 3 底牌），因为 LandlordReady 关闭前
	// state.go 的回调已完成了 append。
	landlord, landlordExists := room.Players[landlordUIDFinal]
	landlordCardsSnapshot := make([]cardutil.Card, len(landlord.Cards))
	copy(landlordCardsSnapshot, landlord.Cards)
	room.mu.RUnlock()

	// 重置所有非机器人玩家的 IsAIControlled 和 GraceWarningSent 状态
	log.Printf("Room %s: resetting IsAIControlled and GraceWarningSent for all non-bot players before starting play phase", room.ID)
	room.mu.Lock()
	for _, uid := range room.PlayerIDs {
		if p, exists := room.Players[uid]; exists && !p.IsBot {
			p.IsAIControlled = false
			p.GraceWarningSent = false
		}
	}
	// 直接在锁内计算 counts（避免调用 GetCardCounts 导致锁重入死锁）
	cardCounts := make(map[string]int)
	for uid, p := range room.Players {
		cardCounts[uid] = len(p.Cards)
	}
	room.mu.Unlock()

	// 立即给地主发送带底牌的手牌
	if landlordExists {
		landlordClient := m.hub.GetClientByUID(landlord.UID)
		if landlordClient != nil {
			log.Printf("Room %s: Sending cards to landlord %s (total %d)", room.ID, landlord.UID, len(landlordCardsSnapshot))
			payload, _ := json.Marshal(map[string]interface{}{
				"my_cards":     landlordCardsSnapshot,
				"is_landlord":  true,
				"landlord_uid": landlordUIDFinal,
			})
			m.hub.SendToClient(landlordClient.ID, 0x0401, payload)
		}
	}

	// 立即广播手牌数量（地主 = 20 张）
	if m.hub != nil {
		countsJSON, _ := json.Marshal(cardCounts)
		payload := []byte(fmt.Sprintf(`{"card_counts":%s,"landlord_uid":"%s"}`,
			string(countsJSON), landlordUIDFinal))
		m.hub.BroadcastToRoom(room.ID, 0x0405, payload)
	}

	// 注意：ConfirmLandlord 中已通过 OnStateChange 回调启动了计时器/触发了 bot 调度，
	// 不需要在这里额外广播 TIMER_NOTIFY，避免覆盖延迟效果。
	// Bug 1 修复：移除立即广播，让 onStateChange 中的延迟广播生效（给玩家足够时间看底牌）

	log.Printf("Room %s: Landlord %s confirmed, play phase started", room.ID, landlordUIDFinal)
}

// onTimeout 超时回调 - 触发 AI 托管自动出牌
func (m *Manager) onTimeout(room *Room, uid string) {
	log.Printf("Room %s: player %s timeout, triggering AI auto-play", room.ID, uid)

	if m.aiEngine == nil {
		log.Printf("AI engine not available, skipping auto-play")
		return
	}

	// 构建 AI 上下文（在锁外拷贝数据）
	room.mu.RLock()
	player, ok := room.Players[uid]
	if !ok {
		room.mu.RUnlock()
		log.Printf("Player %s not found in room %s", uid, room.ID)
		return
	}
	isCallPhase := room.State == types.StateCalling
	isBot := player.IsBot
	room.mu.RUnlock()

	// 叫地主阶段，只让AI帮忙叫地主，不设置托管状态
	if isCallPhase {
		log.Printf("Room %s: calling phase timeout, letting AI call landlord", room.ID)
		m.botCallLandlord(room, uid, isBot)
		return
	}

	// 出牌阶段：只有Bot或者已经被AI托管的玩家才触发AI自动出牌
	if !isBot {
		room.mu.RLock()
		alreadyWarned := player.GraceWarningSent
		room.mu.RUnlock()

		// 真人玩家首次超时：进入"警告宽限期"，不立即设 IsAIControlled
		// 标记 GraceWarningSent=true 表示已发过警告；延长计时器 5s
		// 让用户有时间读牌/操作/主动取消托管（取消托管会重置 GraceWarningSent）
		if !alreadyWarned {
			const gracePeriod = 5 // 警告宽限期（秒）
			room.mu.Lock()
			if p, exists := room.Players[uid]; exists {
				p.GraceWarningSent = true
				log.Printf("Room %s: player %s first timeout, entering grace period (%ds) before AI takes over", room.ID, uid, gracePeriod)
			}
			room.mu.Unlock()
			room.StartTimer(gracePeriod, uid)
			// 复用 timer 通知消息，向客户端广播"即将托管"提示
			if m.hub != nil {
				payload := []byte(fmt.Sprintf(`{"remaining_seconds":%d,"current_turn_uid":"%s","warning":true,"grace":true}`,
					gracePeriod, uid))
				m.hub.BroadcastToRoom(room.ID, 0x0408, payload)
			}
			return
		}

		// 第二次超时（已发过警告但用户未响应）：正式托管 + AI 出牌
		room.mu.Lock()
		if p, exists := room.Players[uid]; exists {
			p.IsAIControlled = true
			log.Printf("Room %s: player %s AI takeover confirmed (second timeout after grace period)", room.ID, uid)
		}
		room.mu.Unlock()
	}

	// 构建 AI 上下文（在锁内拷贝数据并获取 GameState）
	room.mu.RLock()
	myCards := make([]cardutil.Card, len(player.Cards))
	copy(myCards, player.Cards)
	lastPlayedUID := room.LastPlayedUID
	lastPlayedCards := make([]cardutil.Card, len(room.LastPlayedCards))
	copy(lastPlayedCards, room.LastPlayedCards)
	lastPattern := room.LastPattern
	landlordUID := room.LandlordUID
	// 在释放读锁前获取 GameState 引用（避免后续死锁）
	gsm := room.GameState
	room.mu.RUnlock()

	ctx := &ai.AIContext{
		MyCards:     myCards,
		Difficulty:  "normal",
		CardCounter: ai.NewCardCounter(),
	}

	// 设置上一手出牌信息
	if lastPlayedUID != "" && lastPlayedUID != uid {
		// 关键修复：必须从已出牌中解析出主牌值 Main，否则 AI 永远认为任何牌都能管牌
		lastAnalyzed := cardutil.AnalyzePattern(lastPlayedCards)
		ctx.LastPlay = &ai.LastPlayInfo{
			Cards:   lastPlayedCards,
			Pattern: lastPattern,
			Main:    lastAnalyzed.Main,
			UID:     lastPlayedUID,
		}
		ctx.LastPlayerUID = lastPlayedUID
	}

	// 设置角色
	if landlordUID == uid {
		ctx.MyRole = types.RoleLandlord
	} else {
		ctx.MyRole = types.RolePeasant
	}

	// AI 决策
	decision := m.aiEngine.DecidePlay(ctx)

	// 通过 GameStateMachine 执行 AI 决策（自带锁保护）
	// gsm 已经在锁内获取，避免后续死锁
	if gsm == nil {
		log.Printf("GameState not available for room %s", room.ID)
		return
	}

	var gameEnded bool

	if decision.Action == ai.ActionPass {
		log.Printf("AI decided to PASS for player %s", uid)
		_, ended, err := gsm.PlayCards(uid, nil)
		if err != nil {
			// Pass 失败（例如自由出牌阶段不能 pass）—— 不广播 PASS_NOTIFY，
			// 否则前端会显示"不出"但实际玩家没出牌。
			log.Printf("AI pass failed for %s (no PASS broadcast): %v", uid, err)
		} else {
			gameEnded = ended
			// 仅在 PlayCards 真正成功时才广播 PASS
			if m.hub != nil {
				payload := []byte(fmt.Sprintf(`{"uid":"%s"}`, uid))
				m.hub.BroadcastToRoom(room.ID, 0x0406, payload)
			}
		}
	} else {
		log.Printf("AI playing cards for player %s: %v", uid, decision.Cards)
		cardsToPlay := decision.Cards
		playedSuccessfully := false

		// 尝试原始决策
		if _, ended, err := gsm.PlayCards(uid, cardsToPlay); err != nil {
			log.Printf("AI play failed: %v, trying fallback strategy", err)

			// === 保底策略：确保 AI 至少能出一张牌，避免游戏卡死 ===
			room.mu.RLock()
			playerCards := make([]cardutil.Card, len(room.Players[uid].Cards))
			copy(playerCards, room.Players[uid].Cards)
			lastPlayedUID := room.LastPlayedUID
			passCount := room.PassCount
			room.mu.RUnlock()

			// 按牌面值从小到大排序
			cardutil.SortCards(playerCards)

			// 情况1：自由出牌阶段（上家没出牌或连续两个PASS），不能PASS
			if lastPlayedUID == "" || passCount >= 2 {
				log.Printf("Room %s: free turn for player %s, trying smallest single card", room.ID, uid)
				// 出最小的单张
				if len(playerCards) > 0 {
					cardsToPlay = []cardutil.Card{playerCards[0]}
					if _, ended, playErr := gsm.PlayCards(uid, cardsToPlay); playErr != nil {
						log.Printf("Room %s: CRITICAL - smallest card play also failed for player %s: %v", room.ID, uid, playErr)
						// 最后手段：直接出第一张牌（可能不符合规则，但确保游戏能继续）
						cardsToPlay = playerCards[:1]
						// 使用更底层的方式：不通过 PlayCards 校验，直接出牌
						_, ended, playErr = gsm.PlayCards(uid, cardsToPlay)
						if playErr != nil {
							log.Printf("Room %s: FATAL - even forced play failed for player %s: %v", room.ID, uid, playErr)
						} else {
							gameEnded = ended
							playedSuccessfully = true
						}
					} else {
						gameEnded = ended
						playedSuccessfully = true
					}
				}
			} else {
				// 情况2：需要大过上家，但原决策不行 → 尝试 PASS
				log.Printf("Room %s: trying PASS for player %s", room.ID, uid)
				if _, passEnded, passErr := gsm.PlayCards(uid, nil); passErr != nil {
					log.Printf("Room %s: PASS also failed for player %s: %v, forcing smallest card", room.ID, uid, passErr)
					// PASS 也不行，强制出最小的单张
					if len(playerCards) > 0 {
						cardsToPlay = []cardutil.Card{playerCards[0]}
						_, ended, playErr := gsm.PlayCards(uid, cardsToPlay)
						if playErr != nil {
							log.Printf("Room %s: FATAL - forced play also failed for player %s: %v", room.ID, uid, playErr)
						} else {
							gameEnded = ended
							playedSuccessfully = true
						}
					}
				} else {
					gameEnded = passEnded
					// PASS 成功 - 广播 PASS_NOTIFY
					if m.hub != nil {
						payload := []byte(fmt.Sprintf(`{"uid":"%s"}`, uid))
						m.hub.BroadcastToRoom(room.ID, 0x0406, payload)
						log.Printf("Room %s: broadcasted PASS for player %s", room.ID, uid)
					}
					playedSuccessfully = true
				}
			}
		} else {
			gameEnded = ended
			playedSuccessfully = true
		}

		// 出牌成功（包括保底策略），广播出牌
		if playedSuccessfully && m.hub != nil {
			// 检查是否是 PASS 情况（PASS 已经在上面广播过了）
			if cardsToPlay != nil && len(cardsToPlay) > 0 {
				pattern := cardutil.AnalyzePattern(cardsToPlay)
				room.mu.RLock()
				remaining := len(room.Players[uid].Cards)
				room.mu.RUnlock()
				cardsJSON, _ := json.Marshal(cardsToPlay)
				payload := []byte(fmt.Sprintf(`{"uid":"%s","cards":%s,"pattern":"%s","card_count":%d,"is_last":%t,"is_ai":true}`,
					uid, string(cardsJSON), pattern.Pattern.String(), remaining, remaining == 0))
				log.Printf("Room %s: broadcasting play cards for player %s (%d cards, pattern=%s)", room.ID, uid, len(cardsToPlay), pattern.Pattern.String())
				m.hub.BroadcastToRoom(room.ID, 0x0405, payload)
				log.Printf("Room %s: play cards broadcast completed for player %s", room.ID, uid)
			}
		} else if !playedSuccessfully {
			log.Printf("Room %s: WARNING - AI could not play any cards for player %s, game may be stuck", room.ID, uid)
		}
	}

	if gameEnded {
		// 调用游戏结束回调（保存数据库 + 广播结算消息）
		// 注意：HandleGameEnd 已经会广播结算消息，这里不需要重复广播
		if m.onGameEnd != nil && room.GameState != nil {
			m.onGameEnd(room, room.GameState)
		}
		return
	}

	// AI出牌后，PlayCards() 已经推进了回合，直接获取新的当前玩家
	if room.GameState != nil {
		room.mu.RLock()
		nextUID := room.CurrentTurnUID
		isLastRound := room.IsLastRound
		room.mu.RUnlock()

		// === 安全检查：如果回合没有推进（nextUID == uid），手动推进到下一个玩家 ===
		if nextUID == uid || nextUID == "" {
			log.Printf("Room %s: WARNING - turn not advanced after AI play for player %s, manually advancing", room.ID, uid)
			// 使用 NextTurn 计算下一个玩家
			nextUID = room.NextTurn(uid)
			// 重要：更新 CurrentTurnUID（NextTurn 只返回下一个玩家，但不会更新 CurrentTurnUID）
			room.mu.Lock()
			room.CurrentTurnUID = nextUID
			room.IsLastRound = false
			// 同时重置 LastPlayedUID 和 PassCount，模拟自由出牌状态
			room.LastPlayedUID = ""
			room.PassCount = 0
			room.mu.Unlock()
			log.Printf("Room %s: manually advanced turn from %s to %s (force-free-turn)", room.ID, uid, nextUID)
		}

		// 如果 IsLastRound 为 true，需要清除标记
		if isLastRound {
			room.mu.Lock()
			room.IsLastRound = false
			room.mu.Unlock()
			log.Printf("onTimeout: cleared IsLastRound flag")
		}

		log.Printf("onTimeout: AI played, next player is %s", nextUID)

		if nextUID != "" {
			// 在启动定时器前，确保非机器人玩家的 IsAIControlled 和 GraceWarningSent 被正确重置
			if player, exists := room.GetPlayer(nextUID); exists && !player.IsBot {
				if player.IsAIControlled {
					log.Printf("onTimeout: resetting IsAIControlled to false for player %s before starting timer", nextUID)
					player.IsAIControlled = false
				}
				if player.GraceWarningSent {
					player.GraceWarningSent = false
				}
			}

			room.StartTimer(m.config.PlayTimeout, nextUID)
			// 广播定时器消息到客户端（无论下一个是Bot还是真人）
			if m.hub != nil {
				payload := []byte(fmt.Sprintf(`{"remaining_seconds":%d,"current_turn_uid":"%s"}`,
					m.config.PlayTimeout, nextUID))
				m.hub.BroadcastToRoom(room.ID, 0x0408, payload)
				log.Printf("onTimeout: broadcasted timer notify for player %s", nextUID)
			}
			// 如果下一个是Bot，触发AI
			m.TriggerBotIfNeeded(room)
		}
	}
}

// advanceTurn 推进到下一玩家（假设调用方持有或不持有锁均可）
func (m *Manager) advanceTurn(room *Room) {
	room.mu.Lock()
	defer room.mu.Unlock()

	if len(room.PlayerIDs) == 0 {
		return
	}

	// 找到当前玩家的索引
	currentIdx := 0
	for i, pid := range room.PlayerIDs {
		if pid == room.CurrentTurnUID {
			currentIdx = i
			break
		}
	}

	// 计算下一玩家（逆时针方向）
	nextIdx := (currentIdx - 1 + len(room.PlayerIDs)) % len(room.PlayerIDs)
	room.CurrentTurnUID = room.PlayerIDs[nextIdx]
	room.Timer = 15

	// 启动倒计时
	room.StartTimer(15, room.CurrentTurnUID)
}

// startCallTimer 启动叫地主倒计时
// 注意：不在此广播"轮到通知"消息，避免与 startGameWithState / botCallLandlord
// 中的 turn=true 广播重复。前端会把 {action:0, score:0} 误判为"X 不叫"。
// turn 通知由调用方在合适的时机广播。
func (m *Manager) startCallTimer(room *Room) {
	// 随机选一个开始叫地主的玩家
	room.mu.RLock()
	firstCaller := room.PlayerIDs[0] // 简化：第一个玩家
	room.mu.RUnlock()

	room.StartTimer(m.config.ReadyTimeout, firstCaller)
}

// startPlayTimer 启动出牌倒计时
func (m *Manager) startPlayTimer(room *Room) {
	room.mu.RLock()
	currentUID := room.CurrentTurnUID
	room.mu.RUnlock()

	if currentUID != "" {
		room.StartTimer(m.config.PlayTimeout, currentUID)
	}
}

// saveSnapshot 保存房间快照到 Redis
func (m *Manager) saveSnapshot(room *Room) {
	if m.redis == nil {
		return
	}

	key := fmt.Sprintf("ddz:room:snapshot:%s", room.ID)

	room.mu.RLock()
	data := map[string]interface{}{
		"state":            int(room.State),
		"landlord_uid":     room.LandlordUID,
		"current_turn_uid": room.CurrentTurnUID,
		"base_score":       room.BaseScore,
		"multiplier":       room.Multiplier,
		"call_score":       room.CallScore,
		"pass_count":       room.PassCount,
		"player_count":     len(room.Players),
		"updated_at":       time.Now().Unix(),
	}
	room.mu.RUnlock()

	ctx, cancel := context.WithTimeout(m.ctx, 5*time.Second)
	defer cancel()

	// 使用 Hash 存储
	for field, value := range data {
		m.redis.HSet(ctx, key, field, value)
	}

	// 设置过期时间
	m.redis.Expire(ctx, key, time.Duration(m.config.ReconnectTimeout)*time.Second)
}

// snapshotLoop 定期保存所有房间快照
func (m *Manager) snapshotLoop() {
	ticker := time.NewTicker(time.Duration(m.config.SnapshotInterval) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			// 快速获取房间列表，避免长时间持有读锁
			m.roomsMu.RLock()
			roomList := make([]*Room, 0, len(m.rooms))
			for _, room := range m.rooms {
				roomList = append(roomList, room)
			}
			m.roomsMu.RUnlock()

			// 在锁外保存快照，避免阻塞其他操作
			for _, room := range roomList {
				m.saveSnapshot(room)
			}

		case <-m.ctx.Done():
			return
		}
	}
}

// Stop 停止管理器
func (m *Manager) Stop() {
	m.cancel()

	m.roomsMu.Lock()
	for id, room := range m.rooms {
		room.StopTimer()
		delete(m.rooms, id)
	}
	m.roomsMu.Unlock()
}

// mustJSON 将数据序列化为JSON字符串，失败则panic
func mustJSON(v interface{}) string {
	data, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return string(data)
}
