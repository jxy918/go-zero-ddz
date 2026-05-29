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

// CreateRoom 创建房间
func (m *Manager) CreateRoom(id string) (*Room, error) {
	m.roomsMu.Lock()
	defer m.roomsMu.Unlock()

	if len(m.rooms) >= m.config.MaxRooms {
		return nil, fmt.Errorf("max rooms reached")
	}

	if _, exists := m.rooms[id]; exists {
		return nil, fmt.Errorf("room already exists")
	}

	room := NewRoom(id)
	m.rooms[id] = room

	// 设置回调
	room.OnStateChange = m.onStateChange
	room.OnTimeout = m.onTimeout
	room.OnBotJoinTimeout = m.onBotJoinTimeout
	room.OnBotJoinCountdown = m.onRoomBotJoinCountdown

	log.Printf("Room %s created (total: %d)", id, len(m.rooms))

	// 不立即启动机器人加入计时器，等待玩家准备后再启动

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
		// 开始出牌，启动倒计时
		m.startPlayTimer(room)
	case types.StateSettlement:
		// 结算
		room.StopTimer()
	}

	// 如果当前轮到 Bot，立即触发 AI 操作（已持有锁）
	m.triggerBotIfNeededLocked(room)

	// 如果当前轮到真人玩家，启动计时器
	currentPlayer, exists := room.Players[room.CurrentTurnUID]
	if exists && !currentPlayer.IsBot && !currentPlayer.IsAIControlled {
		room.StartTimer(15, room.CurrentTurnUID)
		log.Printf("onStateChange: started timer for human player %s", room.CurrentTurnUID)
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
			m.botCallLandlord(room, currentUID)
		} else if actualState == types.StatePlaying {
			log.Printf("TriggerBotIfNeeded: calling onTimeout")
			m.onTimeout(room, currentUID)
		} else {
			log.Printf("TriggerBotIfNeeded: state is not calling or playing, doing nothing")
		}
	})
}

// triggerBotIfNeededLocked 内部调用，假设调用方已持有 room.mu 锁
func (m *Manager) triggerBotIfNeededLocked(room *Room) {
	currentUID := room.CurrentTurnUID
	player, exists := room.Players[currentUID]
	if !exists || (!player.IsBot && !player.IsAIControlled) {
		return
	}

	// 机器人出牌延迟（给玩家足够看牌和反应时间）
	delay := time.Duration(1500+m.rng.Intn(1000)) * time.Millisecond
	time.AfterFunc(delay, func() {
		room.mu.RLock()
		currentState := room.State
		room.mu.RUnlock()

		if currentState == types.StateCalling {
			m.botCallLandlord(room, currentUID)
		} else if currentState == types.StatePlaying {
			m.onTimeout(room, currentUID)
		}
	})
}

// botCallLandlord Bot 叫地主
func (m *Manager) botCallLandlord(room *Room, uid string) {
	room.mu.Lock()
	player, exists := room.Players[uid]
	callScore := int32(0)
	action := 0
	if exists && !player.IsLandlord && m.rng.Float64() < 0.3 {
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
	log.Printf("Bot %s called: action=%d score=%d", uid, action, callScore)

	// 通过 Hub 广播机器人叫地主结果
	if m.hub != nil {
		log.Printf("Room %s: broadcasting bot call result: uid=%s, action=%d, score=%d", room.ID, uid, action, callScore)
		payload := []byte(fmt.Sprintf(`{"uid":"%s","action":%d,"score":%d,"round":%d}`,
			uid, action, callScore, gsm.CurrentCallRound()))
		m.hub.BroadcastToRoom(room.ID, 0x0403, payload)
	}

	// 检查是否所有玩家都叫完了
	if !gsm.AllCalled() {
		// 还没叫完，继续下一个玩家
		nextIdx := gsm.currentCallIdx
		if nextIdx < 0 || nextIdx >= len(room.PlayerIDs) {
			nextIdx = 0
		}
		nextUID := room.PlayerIDs[nextIdx]

		roomID := room.ID
		hub := m.hub

		time.AfterFunc(time.Second*1, func() {
			room, exists := m.GetRoom(roomID)
			if !exists {
				return
			}

			room.CurrentTurnUID = nextUID
			room.StartTimer(15, nextUID)

			// 通知轮到下一个玩家叫地主（不包含 action 和 score，避免被误解为叫地主结果）
			if hub != nil {
				payload := []byte(fmt.Sprintf(`{"uid":"%s","turn":true}`, nextUID))
				hub.BroadcastToRoom(roomID, 0x0403, payload)
				payload = []byte(fmt.Sprintf(`{"remaining_seconds":15,"current_turn_uid":"%s"}`, nextUID))
				hub.BroadcastToRoom(roomID, 0x0408, payload)
			}

			// 如果下一个是机器人，延迟触发
			if nextPlayer, exists := room.Players[nextUID]; exists && (nextPlayer.IsBot || nextPlayer.IsAIControlled) {
				time.AfterFunc(1500*time.Millisecond, func() {
					m.botCallLandlord(room, nextUID)
				})
			}
		})

		return
	}

	// 所有玩家都叫完了，确认地主
	log.Printf("Bot %s called, all players have called, confirming landlord", uid)
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

	// 广播地主确认通知（包含底牌，翻开展示5秒）
	if m.hub != nil {
		bottomCardsJSON, _ := json.Marshal(bottomCards)
		payload := []byte(fmt.Sprintf(`{"uid":"%s","action":1,"landlord_uid":"%s","players":%s,"bottom_cards":%s}`,
			landlordUID, landlordUID, mustJSON(playersInfo), string(bottomCardsJSON)))
		m.hub.BroadcastToRoom(room.ID, 0x0403, payload)
	}

	// 延迟5秒后发送底牌给地主并开始出牌阶段
	time.AfterFunc(5*time.Second, func() {
		room.mu.RLock()
		landlordUID := room.LandlordUID
		bottomCards := room.BottomCards
		cardCounts := room.GetCardCounts()
		room.mu.RUnlock()

		// 重置所有非机器人玩家的 IsAIControlled 状态
		log.Printf("Room %s: resetting IsAIControlled for all non-bot players before starting play phase", room.ID)
		room.mu.Lock()
		for _, uid := range room.PlayerIDs {
			if p, exists := room.Players[uid]; exists && !p.IsBot {
				log.Printf("Room %s: player %s IsAIControlled before reset: %v", room.ID, uid, p.IsAIControlled)
				p.IsAIControlled = false
				log.Printf("Room %s: player %s reset IsAIControlled to false", room.ID, uid)
			}
		}
		room.mu.Unlock()

		// 给地主发送带底牌的手牌
		if landlord, exists := room.GetPlayer(landlordUID); exists {
			landlordClient := m.hub.GetClientByUID(landlord.UID)
			if landlordClient != nil {
				log.Printf("Room %s: Sending cards to landlord %s", room.ID, landlord.UID)
				payload, _ := json.Marshal(map[string]interface{}{
					"my_cards":     landlord.Cards,
					"bottom_cards": bottomCards,
				})
				m.hub.SendToClient(landlordClient.ID, 0x0401, payload)
			}
		}

		// 广播手牌数量
		if m.hub != nil {
			countsJSON, _ := json.Marshal(cardCounts)
			payload := []byte(fmt.Sprintf(`{"card_counts":%s,"landlord_uid":"%s"}`,
				string(countsJSON), landlordUID))
			m.hub.BroadcastToRoom(room.ID, 0x0405, payload)
		}

		// 启动地主的出牌计时器
		room.SetState(types.StatePlaying)
		room.CurrentTurnUID = landlordUID
		room.StartTimer(m.config.PlayTimeout, landlordUID)

		if m.hub != nil {
			payload := []byte(fmt.Sprintf(`{"remaining_seconds":%d,"current_turn_uid":"%s"}`,
				m.config.PlayTimeout, landlordUID))
			m.hub.BroadcastToRoom(room.ID, 0x0408, payload)
		}

		// 如果地主是机器人，自动出牌
		if landlord, exists := room.GetPlayer(landlordUID); exists && (landlord.IsBot || landlord.IsAIControlled) {
			time.AfterFunc(1500*time.Millisecond, func() {
				m.onTimeout(room, landlordUID)
			})
		}
	})
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
		m.botCallLandlord(room, uid)
		return
	}

	// 出牌阶段：只有Bot或者已经被AI托管的玩家才触发AI自动出牌
	if !isBot {
		// 设置玩家为AI托管状态
		room.mu.Lock()
		if p, exists := room.Players[uid]; exists {
			p.IsAIControlled = true
			log.Printf("Room %s: player %s set to AI controlled", room.ID, uid)
		}
		room.mu.Unlock()
	}

	// 构建 AI 上下文（在锁外拷贝数据）
	room.mu.RLock()
	myCards := make([]cardutil.Card, len(player.Cards))
	copy(myCards, player.Cards)
	lastPlayedUID := room.LastPlayedUID
	lastPlayedCards := make([]cardutil.Card, len(room.LastPlayedCards))
	copy(lastPlayedCards, room.LastPlayedCards)
	lastPattern := room.LastPattern
	landlordUID := room.LandlordUID
	room.mu.RUnlock()

	ctx := &ai.AIContext{
		MyCards:     myCards,
		Difficulty:  "normal",
		CardCounter: ai.NewCardCounter(),
	}

	// 设置上一手出牌信息
	if lastPlayedUID != "" && lastPlayedUID != uid {
		ctx.LastPlay = &ai.LastPlayInfo{
			Cards:   lastPlayedCards,
			Pattern: lastPattern,
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
	gsm := room.GameState
	if gsm == nil {
		log.Printf("GameState not available for room %s", room.ID)
		return
	}

	var gameEnded bool

	if decision.Action == ai.ActionPass {
		log.Printf("AI decided to PASS for player %s", uid)
		if _, ended, err := gsm.PlayCards(uid, nil); err != nil {
			log.Printf("AI pass failed: %v", err)
		} else {
			gameEnded = ended
		}
		// 广播 PASS
		if m.hub != nil {
			payload := []byte(fmt.Sprintf(`{"uid":"%s"}`, uid))
			m.hub.BroadcastToRoom(room.ID, 0x0406, payload)
		}
	} else {
		log.Printf("AI playing cards for player %s: %v", uid, decision.Cards)
		if _, ended, err := gsm.PlayCards(uid, decision.Cards); err != nil {
			log.Printf("AI play failed: %v, switching to PASS", err)
			// 出牌失败，自动 PASS
			if _, passEnded, passErr := gsm.PlayCards(uid, nil); passErr != nil {
				log.Printf("AI pass failed after play failure: %v", passErr)
			} else {
				gameEnded = passEnded
			}
			// 广播 PASS
			if m.hub != nil {
				payload := []byte(fmt.Sprintf(`{"uid":"%s"}`, uid))
				m.hub.BroadcastToRoom(room.ID, 0x0406, payload)
			}
		} else {
			gameEnded = ended
			// 出牌成功，广播出牌
			if m.hub != nil {
				pattern := cardutil.AnalyzePattern(decision.Cards)
				room.mu.RLock()
				remaining := len(room.Players[uid].Cards)
				room.mu.RUnlock()
				cardsJSON, _ := json.Marshal(decision.Cards)
				payload := []byte(fmt.Sprintf(`{"uid":"%s","cards":%s,"pattern":"%s","card_count":%d,"is_last":%t,"is_ai":true}`,
					uid, string(cardsJSON), pattern.Pattern.String(), remaining, remaining == 0))
				m.hub.BroadcastToRoom(room.ID, 0x0405, payload)
			}
		}
	}

	if gameEnded {
		// 广播游戏结束
		if m.hub != nil {
			gsm := room.GameState
			if gsm != nil {
				result := gsm.CalculateSettlement()
				winnerSide := "landlord"
				if result.WinnerSide == types.WinnerSidePeasant {
					winnerSide = "peasant"
				}
				payload := []byte(fmt.Sprintf(`{"winner_uid":"%s","winner_side":"%s","base_score":%d,"multiplier":%d}`,
					result.WinnerUID, winnerSide, result.BaseScore, result.Multiplier))
				m.hub.BroadcastToRoom(room.ID, 0x0407, payload)
			}
		}
		return
	}

	// AI出牌后，推进到下一个玩家
	if room.GameState != nil {
		// 检查是否是连续 PASS 后的情况（IsLastRound 标记）
		room.mu.RLock()
		isLastRound := room.IsLastRound
		currentTurn := room.CurrentTurnUID
		room.mu.RUnlock()

		var nextUID string
		// 如果 IsLastRound 为 true，说明是连续 PASS 的情况
		// CurrentTurnUID 已经是最后出牌的人，不需要再调用 NextTurnAfterPlay()
		if isLastRound {
			room.mu.Lock()
			room.IsLastRound = false
			room.mu.Unlock()
			log.Printf("onTimeout: IsLastRound=true, CurrentTurnUID=%s, using existing turn", currentTurn)
			nextUID = currentTurn
		} else {
			// 正常情况，推进到下一个玩家
			nextUID = room.GameState.NextTurnAfterPlay()
			log.Printf("onTimeout: AI played, next player is %s", nextUID)
		}

		if nextUID != "" {
			// 在启动定时器前，确保非机器人玩家的 IsAIControlled 被正确重置
			if player, exists := room.GetPlayer(nextUID); exists && !player.IsBot && player.IsAIControlled {
				log.Printf("onTimeout: resetting IsAIControlled to false for player %s before starting timer", nextUID)
				player.IsAIControlled = false
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
func (m *Manager) startCallTimer(room *Room) {
	// 随机选一个开始叫地主的玩家
	room.mu.RLock()
	firstCaller := room.PlayerIDs[0] // 简化：第一个玩家
	room.mu.RUnlock()

	room.StartTimer(m.config.ReadyTimeout, firstCaller)

	// 通知客户端轮到叫地主
	if m.hub != nil {
		payload := []byte(fmt.Sprintf(`{"uid":"%s","action":0,"score":0}`, firstCaller))
		m.hub.BroadcastToRoom(room.ID, 0x0403, payload)
	}
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
			m.roomsMu.RLock()
			for _, room := range m.rooms {
				m.saveSnapshot(room)
			}
			m.roomsMu.RUnlock()

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
