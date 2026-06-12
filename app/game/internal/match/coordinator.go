package match

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/zeromicro/go-zero/core/logx"

	"go-zero-ddz/app/game/internal/room"
	"go-zero-ddz/app/game/internal/websocket"
)

// Config 匹配系统配置
type Config struct {
	Enabled        bool
	ScanInterval   int // 扫描间隔（秒）
	RandomTimeout  int // 随机匹配超时
	RankedTimeout  int // 段位匹配超时
	EloRange       int // ELO 容差
	BotFillTimeout int // Bot 填充超时
}

// QueueStats 队列统计
type QueueStats struct {
	RandomCount  int64
	RankedCount  int64
	MatchedCount int64
	TimeoutCount int64
}

// Coordinator 匹配协调器
type Coordinator struct {
	cfg          Config
	queue        *Queue
	roomMgr      *room.Manager
	hub          *websocket.Hub
	rdb          redis.UniversalClient
	ctx          context.Context
	cancel       context.CancelFunc
	matchedCount int64
	timeoutCount int64
}

// NewCoordinator 创建匹配协调器
func NewCoordinator(cfg Config, rdb redis.UniversalClient, roomMgr *room.Manager, hub *websocket.Hub) *Coordinator {
	ctx, cancel := context.WithCancel(context.Background())

	return &Coordinator{
		cfg:     cfg,
		queue:   NewQueue(rdb),
		roomMgr: roomMgr,
		hub:     hub,
		rdb:     rdb,
		ctx:     ctx,
		cancel:  cancel,
	}
}

// Start 启动匹配系统
func (c *Coordinator) Start() {
	if !c.cfg.Enabled {
		logx.Info("Matchmaking disabled")
		return
	}

	logx.Info("Matchmaking started")
	go c.scanLoop()
}

// Stop 停止匹配系统
func (c *Coordinator) Stop() {
	c.cancel()
	logx.Info("Matchmaking stopped")
}

// Enqueue 玩家入队
func (c *Coordinator) Enqueue(ctx context.Context, player *WaitingPlayer) error {
	return c.queue.Enqueue(ctx, player)
}

// Cancel 取消匹配
func (c *Coordinator) Cancel(ctx context.Context, uid string) error {
	return c.queue.Dequeue(ctx, uid)
}

// scanLoop 定期扫描匹配队列
func (c *Coordinator) scanLoop() {
	ticker := time.NewTicker(time.Duration(c.cfg.ScanInterval) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			c.scanRandomQueue()
			c.scanRankedQueue()

		case <-c.ctx.Done():
			return
		}
	}
}

// scanRandomQueue 扫描随机匹配队列
func (c *Coordinator) scanRandomQueue() {
	ctx, cancel := context.WithTimeout(c.ctx, 5*time.Second)
	defer cancel()

	players, err := c.queue.GetRandomQueue(ctx, 100)
	if err != nil {
		logx.Errorf("Failed to get random queue: %v", err)
		return
	}

	if len(players) < 3 {
		return
	}

	// 取前 3 个玩家
	matchGroup := players[:3]

	// 检查是否有超时的
	now := time.Now()
	for _, p := range matchGroup {
		if now.Sub(p.WaitStart) > time.Duration(c.cfg.RandomTimeout)*time.Second {
			// 超时，填充 Bot
			c.fillWithBots(ctx, matchGroup)
			return
		}
	}

	// 创建房间并匹配
	c.createMatchRoom(ctx, matchGroup, false)
}

// scanRankedQueue 扫描段位匹配队列
func (c *Coordinator) scanRankedQueue() {
	ctx, cancel := context.WithTimeout(c.ctx, 5*time.Second)
	defer cancel()

	// 遍历所有段位队列
	for _, tier := range Tiers {
		players, err := c.queue.GetRankedQueue(ctx, tier.Name, tier.Min, tier.Max, 100)
		if err != nil || len(players) < 3 {
			continue
		}

		matchGroup := players[:3]

		// 检查超时
		now := time.Now()
		for _, p := range matchGroup {
			if now.Sub(p.WaitStart) > time.Duration(c.cfg.RankedTimeout)*time.Second {
				c.fillWithBots(ctx, matchGroup)
				return
			}
		}

		c.createMatchRoom(ctx, matchGroup, true)
		break
	}
}

// fillWithBots 用 Bot 填充匹配组
func (c *Coordinator) fillWithBots(ctx context.Context, players []*WaitingPlayer) {
	needed := 3 - len(players)
	for range needed {
		bot := &WaitingPlayer{
			UID:       generateBotUID(),
			ELO:       1000,
			Tier:      "青铜I",
			MatchType: MatchTypeRandom,
		}
		players = append(players, bot)
	}

	c.createMatchRoom(ctx, players, false)
}

// createMatchRoom 创建匹配房间
func (c *Coordinator) createMatchRoom(ctx context.Context, players []*WaitingPlayer, isRanked bool) {
	roomID := room.GenerateID()
	r, err := c.roomMgr.CreateRoom(roomID)
	if err != nil {
		logx.Errorf("Failed to create match room: %v", err)
		return
	}

	// 添加玩家到房间
	for _, p := range players {
		player := &room.Player{
			UID:      p.UID,
			Nickname: "Player_" + p.UID[:min(4, len(p.UID))],
			ELO:      p.ELO,
			Tier:     p.Tier,
			IsBot:    len(p.UID) > 4 && p.UID[:4] == "bot_",
			IsOnline: true,
			IsReady:  true, // 匹配玩家自动准备
		}
		if err := r.AddPlayer(player); err != nil {
			logx.Errorf("Failed to add player %s to room: %v", p.UID, err)
			return
		}
	}

	// 通知玩家匹配成功
	for _, p := range players {
		if p.UID[:min(4, len(p.UID))] == "bot_" {
			continue // Bot 不需要通知
		}

		// 查找玩家连接并发送通知
		// 实际应该通过 Hub 发送
		logx.Infof("Match success: player %s -> room %s", p.UID, roomID)
	}

	// 从队列中移除
	c.queue.RemovePlayers(ctx, players)

	// 自动开始游戏（所有玩家已准备）
	logx.Infof("Match room %s created with %d players (ranked: %v)", roomID, len(players), isRanked)
}

func generateBotUID() string {
	return "bot_" + time.Now().Format("20060102150405")
}

// GetQueueStats 获取队列统计信息
func (c *Coordinator) GetQueueStats() QueueStats {
	ctx, cancel := context.WithTimeout(c.ctx, 5*time.Second)
	defer cancel()

	randomPlayers, _ := c.queue.GetRandomQueue(ctx, 1000)
	randomCount := int64(len(randomPlayers))

	rankedCount := int64(0)
	for _, tier := range Tiers {
		players, _ := c.queue.GetRankedQueue(ctx, tier.Name, tier.Min, tier.Max, 1000)
		rankedCount += int64(len(players))
	}

	return QueueStats{
		RandomCount:  randomCount,
		RankedCount:  rankedCount,
		MatchedCount: c.matchedCount,
		TimeoutCount: c.timeoutCount,
	}
}
