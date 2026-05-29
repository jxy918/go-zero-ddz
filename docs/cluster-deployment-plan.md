# 斗地主多实例集群部署方案

## 1. 架构概述

### 1.1 组件拓扑

```
┌─────────────────────────────────────────────────────────────────┐
│                        客户端层 (Clients)                        │
│              Cocos Creator (Web / H5 / 微信小游戏)               │
└────────────────────────────┬────────────────────────────────────┘
                             │ WebSocket (wss://)
                             ▼
┌─────────────────────────────────────────────────────────────────┐
│                      负载均衡层 (Load Balancer)                   │
│                  Nginx / HAProxy / 云厂商 LB                      │
│         策略: 最少连接 (Least Connections) 或 Round Robin         │
│         注意: 不需要 Sticky Session（玩家路由由 Redis 管理）        │
└────────────────────────────┬────────────────────────────────────┘
                             │
              ┌──────────────┼──────────────┐
              ▼              ▼              ▼
        ┌──────────┐  ┌──────────┐  ┌──────────┐
        │ Game-01  │  │ Game-02  │  │ Game-03  │  ← 可水平扩展
        │ :8080    │  │ :8080    │  │ :8080    │
        │          │  │          │  │          │
        │ ┌──────┐ │  │ ┌──────┐ │  │ ┌──────┐ │
        │ │内存  │ │  │ │内存  │ │  │ │内存  │ │
        │ │房间  │ │  │ │房间  │ │  │ │房间  │ │
        │ └──────┘ │  │ └──────┘ │  │ └──────┘ │
        │          │  │          │  │          │
        │ ┌──────┐ │  │ ┌──────┐ │  │ ┌──────┐ │
        │ │Redis │ │  │ │Redis │ │  │ │Redis │ │
        │ │Client│ │  │ │Client│ │  │ │Client│ │
        │ └──────┘ │  │ └──────┘ │  │ └──────┘ │
        └────┬─────┘  └────┬─────┘  └────┬─────┘
             │              │              │
             └──────────────┼──────────────┘
                            │
              ┌─────────────┴─────────────┐
              │       Redis (单节点/       │
              │       Cluster/Sentinel)    │
              │                           │
              │  • Player Route Table     │
              │  • Room Snapshots         │
              │  • Pub/Sub 消息总线        │
              │  • 匹配队列                │
              │  • 分布式锁                │
              └─────────────┬─────────────┘
                            │
              ┌─────────────┴─────────────┐
              │       MySQL (主从/         │
              │       云数据库)             │
              │                           │
              │  • User Accounts          │
              │  • Game History           │
              │  • ELO / Leaderboard      │
              └───────────────────────────┘
```

### 1.2 数据流向

```
【同实例通信】（房间内所有玩家在同一实例）
  Player A (Game-01) ──出牌──► Room 101 (内存) ──广播──► Player B (Game-01)
  延迟: < 1ms

【跨实例通信】（房间内玩家分布在不同实例）
  Player A (Game-01) ──出牌──► Room 101 (Game-01)
                              │
                              ├──► 本地 Player B (Game-01)
                              │
                              └──► Redis Pub/Sub: channel="room:101"
                                              │
                                    Game-02 订阅者收到消息
                                              │
                                              └──► Player C (Game-02)
  延迟: < 5ms (含 Redis 往返)
```

---

## 2. 核心机制设计

### 2.1 玩家路由表 (Player Route Table)

```
Redis Key:    ddz:player:route:{uid}
Value:        { "instance_id": "game-01", "room_id": 101, "ws_conn_id": "conn-xxx" }
TTL:          玩家在线期间有效（心跳续期）
Set:          玩家连接 WS 时写入
Delete:       玩家断开连接时删除
```

### 2.2 房间归属与跨实例消息

```
【房间归属】
  谁创建房间，房间就归属谁
  Redis Key: ddz:room:owner:{room_id} → "game-01"
  
  归属实例 = 房间的"权威实例"
  - 游戏状态机运行在归属实例的内存中
  - 所有游戏逻辑（出牌校验、胜负判定）在归属实例执行

【跨实例玩家加入】
  Player C 在 Game-02 加入 Room 101（归属 Game-01）:
  1. Game-02 收到"加入房间"请求
  2. Game-02 查询 Redis: ddz:room:owner:101 → "game-01"
  3. Game-02 通过 Redis Pub/Sub 发送加入请求到 channel="room:101:control"
  4. Game-01 收到请求，执行加入逻辑
  5. Game-01 通过 Pub/Sub 广播房间状态更新
  6. Game-02 收到广播，更新本地玩家列表
```

### 2.3 Redis Pub/Sub Channel 设计

```
room:{room_id}:broadcast    → 出牌结果、游戏状态变更、系统消息
room:{room_id}:control      → 加入/离开房间、准备、叫地主
room:{room_id}:play         → 玩家出牌请求
instance:{instance_id}:msg  → 管理命令、状态同步
global:events               → 实例上线/下线、全局广播
```

### 2.4 实例注册与发现

```
【实例注册】
  启动时写入 Redis: ddz:instance:{instance_id}
  TTL: 30s（每 10s 续期）
  
  优雅关闭:
    1. 接收 SIGTERM
    2. 状态改为 "draining"
    3. 等待房间自然结束
    4. 注销实例 + 玩家路由
    5. 退出进程
```

---

## 3. 配置设计

### 3.1 配置文件结构 (YAML)

```yaml
# etc/game.yaml
Name: game-service
Host: 0.0.0.0
Port: 8080
InstanceId: ""

Cluster:
  Enabled: false
  InstanceId: ""
  Host: "127.0.0.1"
  Port: 8080

Redis:
  Host: localhost:6379
  Type: node
  Pass: ""
  Tls: false
  ClusterNodes: []
  SentinelMaster: ""
  SentinelNodes: []

WebSocket:
  ReadBufferSize: 4096
  WriteBufferSize: 4096
  HandshakeTimeout: 10
  PingPeriod: 30
  PongWait: 60
  MaxMessageSize: 65536
  WriteWait: 10

Room:
  MaxRooms: 10000
  MaxPlayersPerRoom: 3
  ReadyTimeout: 60
  PlayTimeout: 15
  ReconnectTimeout: 300
  SnapshotInterval: 30

Match:
  Enabled: true
  ScanInterval: 2           # 匹配扫描间隔（秒）
  RandomTimeout: 15         # 随机匹配超时（秒）
  RankedTimeout: 30         # 段位匹配超时（秒）
  EloRange: 100             # 段位匹配 ELO 容差
  BotFillTimeout: 30        # 超时后填充 AI 机器人

AI:
  Enabled: true
  DefaultDifficulty: "normal"  # easy | normal | hard
  AutoEnableTimeout: 30        # 超时后自动开启托管（秒）
  PlayDelayMin: 500            # AI 出牌最小延迟（ms）
  PlayDelayMax: 2000           # AI 出牌最大延迟（ms）

Log:
  Mode: file
  Path: logs/
  Level: info
  Compress: true
  KeepDays: 7

Metrics:
  Enabled: true
  Path: /metrics
```

### 3.2 单机 vs 集群配置对比

**单机开发模式** (`etc/game-local.yaml`):
```yaml
Cluster:
  Enabled: false
Redis:
  Host: localhost:6379
  Type: node
Log:
  Mode: console
  Level: debug
Metrics:
  Enabled: false
```

**生产集群模式** (`etc/game-prod.yaml`):
```yaml
Cluster:
  Enabled: true
  Host: "${POD_IP}"
Redis:
  Type: cluster
  Pass: "${REDIS_PASSWORD}"
  ClusterNodes:
    - redis-cluster-0:6379
    - redis-cluster-1:6379
    - redis-cluster-2:6379
Room:
  SnapshotInterval: 10
Log:
  Mode: file
  Level: info
Metrics:
  Enabled: true
```

---

## 4. 关键代码设计

### 4.1 实例注册与心跳

```go
// internal/cluster/registry.go
package cluster

import (
    "context"
    "encoding/json"
    "fmt"
    "os"
    "time"

    "github.com/redis/go-redis/v9"
)

type InstanceInfo struct {
    InstanceID  string `json:"instance_id"`
    Host        string `json:"host"`
    Port        int    `json:"port"`
    Status      string `json:"status"`
    Connections int    `json:"connections"`
    Rooms       int    `json:"rooms"`
    StartedAt   string `json:"started_at"`
}

type Registry struct {
    rdb        *redis.Client
    instanceID string
    info       InstanceInfo
    ctx        context.Context
    cancel     context.CancelFunc
    ttl        time.Duration
}

func NewRegistry(rdb *redis.Client, host string, port int) *Registry {
    instanceID := generateInstanceID()
    ctx, cancel := context.WithCancel(context.Background())
    
    return &Registry{
        rdb:        rdb,
        instanceID: instanceID,
        info: InstanceInfo{
            InstanceID:  instanceID,
            Host:        host,
            Port:        port,
            Status:      "active",
            StartedAt:   time.Now().UTC().Format(time.RFC3339),
        },
        ctx:    ctx,
        cancel: cancel,
        ttl:    30 * time.Second,
    }
}

func generateInstanceID() string {
    hostname, _ := os.Hostname()
    return fmt.Sprintf("%s-%d", hostname, os.Getpid())
}

func (r *Registry) Register() error {
    key := fmt.Sprintf("ddz:instance:%s", r.instanceID)
    val, _ := json.Marshal(r.info)
    _, err := r.rdb.SetEX(r.ctx, key, string(val), r.ttl).Result()
    if err != nil {
        return fmt.Errorf("register instance: %w", err)
    }
    go r.heartbeatLoop()
    r.broadcastEvent("instance_joined", r.info)
    return nil
}

func (r *Registry) heartbeatLoop() {
    ticker := time.NewTicker(r.ttl / 3)
    defer ticker.Stop()
    for {
        select {
        case <-ticker.C:
            r.refreshHeartbeat()
        case <-r.ctx.Done():
            return
        }
    }
}

func (r *Registry) refreshHeartbeat() {
    key := fmt.Sprintf("ddz:instance:%s", r.instanceID)
    r.info.Connections = getGlobalConnectionCount()
    r.info.Rooms = getGlobalRoomCount()
    val, _ := json.Marshal(r.info)
    r.rdb.SetEX(r.ctx, key, string(val), r.ttl)
}

func (r *Registry) Unregister() error {
    r.cancel()
    r.info.Status = "offline"
    key := fmt.Sprintf("ddz:instance:%s", r.instanceID)
    val, _ := json.Marshal(r.info)
    r.rdb.Set(r.ctx, key, string(val), 60*time.Second)
    r.broadcastEvent("instance_left", r.info)
    return nil
}

func (r *Registry) GetInstanceID() string {
    return r.instanceID
}

func (r *Registry) UpdateStats(connections, rooms int) {
    r.info.Connections = connections
    r.info.Rooms = rooms
}

func (r *Registry) broadcastEvent(eventType string, info InstanceInfo) {
    event := map[string]interface{}{
        "type":      eventType,
        "instance":  info,
        "timestamp": time.Now().UnixMilli(),
    }
    payload, _ := json.Marshal(event)
    r.rdb.Publish(r.ctx, "global:events", string(payload))
}

// 以下为占位函数，实际由 Hub 和 RoomManager 提供
func getGlobalConnectionCount() int { return 0 }
func getGlobalRoomCount() int       { return 0 }
```

### 4.2 玩家路由管理

```go
// internal/cluster/router.go
package cluster

import (
    "context"
    "encoding/json"
    "fmt"
    "time"

    "github.com/redis/go-redis/v9"
)

type PlayerRoute struct {
    InstanceID string `json:"instance_id"`
    RoomID     string `json:"room_id"`
    ConnID     string `json:"conn_id"`
}

type Router struct {
    rdb        *redis.Client
    instanceID string
    ttl        time.Duration
}

func NewRouter(rdb *redis.Client, instanceID string) *Router {
    return &Router{
        rdb:        rdb,
        instanceID: instanceID,
        ttl:        5 * time.Minute,
    }
}

func (r *Router) RegisterPlayer(ctx context.Context, uid string, roomID, connID string) error {
    key := fmt.Sprintf("ddz:player:route:%s", uid)
    route := PlayerRoute{
        InstanceID: r.instanceID,
        RoomID:     roomID,
        ConnID:     connID,
    }
    val, _ := json.Marshal(route)
    _, err := r.rdb.SetEX(ctx, key, string(val), r.ttl).Result()
    return err
}

func (r *Router) GetPlayerRoute(ctx context.Context, uid string) (*PlayerRoute, error) {
    key := fmt.Sprintf("ddz:player:route:%s", uid)
    val, err := r.rdb.Get(ctx, key).Result()
    if err != nil {
        return nil, err
    }
    var route PlayerRoute
    if err := json.Unmarshal([]byte(val), &route); err != nil {
        return nil, err
    }
    return &route, nil
}

func (r *Router) IsLocalPlayer(ctx context.Context, uid string) bool {
    route, err := r.GetPlayerRoute(ctx, uid)
    if err != nil {
        return false
    }
    return route.InstanceID == r.instanceID
}

func (r *Router) UnregisterPlayer(ctx context.Context, uid string) error {
    key := fmt.Sprintf("ddz:player:route:%s", uid)
    return r.rdb.Del(ctx, key).Err()
}

func (r *Router) RenewPlayerRoute(ctx context.Context, uid string) error {
    key := fmt.Sprintf("ddz:player:route:%s", uid)
    return r.rdb.Expire(ctx, key, r.ttl).Err()
}
```

### 4.3 跨实例消息总线

```go
// internal/cluster/message_bus.go
package cluster

import (
    "context"
    "encoding/json"
    "fmt"
    "log"
    "time"

    "github.com/redis/go-redis/v9"
)

type MessageType string

const (
    MsgTypePlay      MessageType = "play"
    MsgTypeControl   MessageType = "control"
    MsgTypeBroadcast MessageType = "broadcast"
    MsgTypeReconnect MessageType = "reconnect"
)

type ClusterMessage struct {
    Type      MessageType     `json:"type"`
    RoomID    string          `json:"room_id"`
    SenderUID string          `json:"sender_uid,omitempty"`
    Payload   json.RawMessage `json:"payload"`
    Timestamp int64           `json:"timestamp"`
}

type MessageHandler func(msg *ClusterMessage)

type MessageBus struct {
    rdb        *redis.Client
    instanceID string
    handlers   map[string]MessageHandler
    pubsub     *redis.PubSub
    ctx        context.Context
}

func NewMessageBus(rdb *redis.Client, instanceID string) *MessageBus {
    return &MessageBus{
        rdb:        rdb,
        instanceID: instanceID,
        handlers:   make(map[string]MessageHandler),
    }
}

func (mb *MessageBus) Start(ctx context.Context) {
    mb.ctx = ctx
    mb.pubsub = mb.rdb.Subscribe(ctx)
    go mb.listenLoop(ctx)
}

func (mb *MessageBus) Subscribe(ctx context.Context, channel string, handler MessageHandler) error {
    mb.handlers[channel] = handler
    return mb.pubsub.Subscribe(ctx, channel)
}

func (mb *MessageBus) SubscribeRoom(ctx context.Context, roomID string) error {
    channels := []string{
        fmt.Sprintf("room:%s:broadcast", roomID),
        fmt.Sprintf("room:%s:control", roomID),
        fmt.Sprintf("room:%s:play", roomID),
    }
    for _, ch := range channels {
        if err := mb.Subscribe(ctx, ch, mb.getRoomHandler(roomID, ch)); err != nil {
            return err
        }
    }
    return nil
}

func (mb *MessageBus) Publish(ctx context.Context, channel string, msg *ClusterMessage) error {
    msg.Timestamp = time.Now().UnixMilli()
    payload, err := json.Marshal(msg)
    if err != nil {
        return err
    }
    return mb.rdb.Publish(ctx, channel, string(payload)).Err()
}

func (mb *MessageBus) PublishToRoom(ctx context.Context, roomID string, msgType MessageType, payload interface{}) error {
    channel := getChannelForType(roomID, msgType)
    msg := &ClusterMessage{
        Type:    msgType,
        RoomID:  roomID,
        Payload: mustMarshal(payload),
    }
    return mb.Publish(ctx, channel, msg)
}

func (mb *MessageBus) listenLoop(ctx context.Context) {
    ch := mb.pubsub.Channel()
    for {
        select {
        case <-ctx.Done():
            return
        case msg, ok := <-ch:
            if !ok {
                return
            }
            handler, exists := mb.handlers[msg.Channel]
            if !exists {
                continue
            }
            var clusterMsg ClusterMessage
            if err := json.Unmarshal([]byte(msg.Payload), &clusterMsg); err != nil {
                log.Printf("failed to unmarshal cluster message: %v", err)
                continue
            }
            handler(&clusterMsg)
        }
    }
}

func (mb *MessageBus) Close() error {
    if mb.pubsub != nil {
        return mb.pubsub.Close()
    }
    return nil
}

func getChannelForType(roomID string, msgType MessageType) string {
    switch msgType {
    case MsgTypePlay:
        return fmt.Sprintf("room:%s:play", roomID)
    case MsgTypeControl:
        return fmt.Sprintf("room:%s:control", roomID)
    default:
        return fmt.Sprintf("room:%s:broadcast", roomID)
    }
}

func mustMarshal(v interface{}) json.RawMessage {
    b, _ := json.Marshal(v)
    return b
}

func (mb *MessageBus) getRoomHandler(roomID, channel string) MessageHandler {
    return func(msg *ClusterMessage) {
        switch {
        case isBroadcastChannel(channel):
            mb.handleBroadcast(roomID, msg)
        case isControlChannel(channel):
            if mb.isRoomOwner(roomID) {
                mb.handleControl(roomID, msg)
            }
        case isPlayChannel(channel):
            if mb.isRoomOwner(roomID) {
                mb.handlePlay(roomID, msg)
            }
        }
    }
}

func isBroadcastChannel(ch string) bool { return len(ch) > 9 && ch[len(ch)-9:] == "broadcast" }
func isControlChannel(ch string) bool   { return len(ch) > 7 && ch[len(ch)-7:] == "control" }
func isPlayChannel(ch string) bool      { return len(ch) > 4 && ch[len(ch)-4:] == "play" }

func (mb *MessageBus) isRoomOwner(roomID string) bool { return true } // 实际查 Redis
func (mb *MessageBus) handleBroadcast(roomID string, msg *ClusterMessage) {}
func (mb *MessageBus) handleControl(roomID string, msg *ClusterMessage)   {}
func (mb *MessageBus) handlePlay(roomID string, msg *ClusterMessage)      {}
```

---

## 5. 匹配系统设计

### 5.1 匹配模式

```
┌─────────────────────────────────────────────────────┐
│                   匹配系统入口                       │
├──────────────────┬──────────────────────────────────┤
│   快速匹配        │         段位匹配                  │
│  (Random Match)  │      (Ranked Match)               │
├──────────────────┼──────────────────────────────────┤
│ • 随机分配对手    │ • 按 ELO 积分匹配                 │
│ • 无积分变化      │ • 积分变化影响段位                │
│ • 适合练习        │ • 适合竞技                        │
│ • 秒进            │ • 可能需要等待                    │
└──────────────────┴──────────────────────────────────┘
```

### 5.2 段位体系

| 段位 | 名称 | ELO 区间 |
|------|------|----------|
| D1-D3 | 青铜 I-III | 0 - 899 |
| C1-C3 | 白银 I-III | 900 - 1799 |
| B1-B3 | 黄金 I-III | 1800 - 2699 |
| A1-A3 | 铂金 I-III | 2700 - 3599 |
| S  | 大师   | 3600+ |

### 5.3 ELO 积分规则

```
【胜负积分变化】
地主胜利: 地主 +30×倍数×系数, 农民 -15×倍数×系数
农民胜利: 地主 -20×倍数×系数, 农民 +15×倍数×系数

系数:
  对手积分 > 自己 100+: 1.2
  对手积分 < 自己 100+: 0.8
  其他: 1.0

倍数:
  基础: 1, 炸弹: ×2, 春天: ×2, 反春: ×2

保护机制:
  - 新玩家前 10 局输局扣分减半
  - 连输 3 局后扣分减半
  - 降段后 3 局内不会再降段
```

### 5.4 匹配算法

```go
// 匹配策略（优先级从高到低）
// 1. 同实例 + 同段位 ±50 分 → 立即匹配
// 2. 同实例 + 同段位 ±100 分 → 等待 5s 后匹配
// 3. 跨实例 + 同段位 ±100 分 → 等待 10s 后匹配
// 4. 任意实例 + 任意段位 → 等待 15s 后兜底匹配
// 5. 超时 30s → 加入 AI 机器人补齐
```

### 5.5 匹配系统架构

```
Redis Sorted Sets:
  ddz:match:queue:random
    Score: timestamp (等待时间)
    Member: {instance_id}:{uid}
    
  ddz:match:queue:ranked:{tier}
    Score: ELO
    Member: {instance_id}:{uid}
    
  ddz:match:lock:{uid}
    防止重复入队 (分布式锁)

匹配流程:
  1. 玩家点击"开始匹配" → 实例写入 Redis 队列
  2. MatchCoordinator 定时扫描队列 (每 2s)
  3. 找到 3 个满足条件的玩家
  4. 选择 ELO 最高的玩家所在实例作为房间归属
  5. 通知各实例创建房间
  6. 玩家收到"匹配成功"消息，自动加入房间
```

### 5.6 AI 机器人

```
使用场景:
  1. 匹配超时 30s 无人 → 补充 AI 机器人
  2. 玩家断线超过重连窗口 → AI 接管
  3. 玩家主动选择"单人练习" → 2 个 AI 对手

难度等级:
  简单: 随机出合法牌，不记牌
  普通: 优先出小牌，会管牌，不记牌
  困难: 记已出大牌，会算牌，保留炸弹

标识: UID 以 "bot_" 开头，不记录 ELO
```

---

## 6. AI 自动出牌系统

### 6.1 触发条件

```
1. 玩家出牌超时 (15s 无操作)
   → 首次超时: 弹出"是否托管"确认
   → 再次超时: 自动开启托管

2. 玩家主动点击"托管"按钮 → 立即开启

3. 玩家断线重连后选择"继续托管" → 保持

4. 匹配补齐的 AI 机器人 → 全程 AI 控制
```

### 6.2 AI 出牌引擎

```
Input:
  - 当前手牌
  - 上一手牌
  - 已出牌历史 (记牌)
  - 队友/对手身份
  - 剩余牌数

决策流程:
  1. 枚举所有合法出牌组合
  2. 过滤不符合规则的出牌
  3. 对每个合法出牌进行评分
  4. 选择最高分出牌 (或不出)
```

### 6.3 AI 核心代码

```go
// internal/ai/engine.go
package ai

import (
    "math/rand"
    "time"
)

type AIContext struct {
    MyCards      []*Card
    LastPlay     *PlayRecord      // 上一手出牌
    LastPlayerUID string          // 上一手出牌玩家
    MyRole       PlayerRole       // 地主/农民
    Players      map[string]*PlayerInfo
    CardCounter  *CardCounter     // 记牌器
    Difficulty   string           // easy | normal | hard
}

type PlayDecision struct {
    Action PlayAction
    Cards  []*Card
}

type PlayAction int

const (
    ActionPass PlayAction = iota
    ActionPlay
)

type AIEngine struct {
    config *AIConfig
    rng    *rand.Rand
}

func NewAIEngine(config *AIConfig) *AIEngine {
    return &AIEngine{
        config: config,
        rng:    rand.New(rand.NewSource(time.Now().UnixNano())),
    }
}

func (ai *AIEngine) DecidePlay(ctx *AIContext) *PlayDecision {
    legalPlays := ai.enumerateLegalPlays(ctx)
    
    if len(legalPlays) == 0 {
        return &PlayDecision{Action: ActionPass}
    }
    
    // 自由出牌（没人出牌或上家 Pass）
    if ctx.LastPlay == nil {
        return ai.decideFreePlay(ctx, legalPlays)
    }
    
    // 需要管牌
    return ai.decideResponsePlay(ctx, legalPlays)
}

func (ai *AIEngine) decideFreePlay(ctx *AIContext, plays []CardCombo) *PlayDecision {
    // 检查是否能一手出完
    if len(ctx.MyCards) <= 5 {
        if ai.canPlayAllAtOnce(ctx.MyCards) {
            return &PlayDecision{Action: ActionPlay, Cards: ctx.MyCards}
        }
    }
    
    // 出最小的单张
    smallest := ai.findSmallestSingle(ctx.MyCards)
    if smallest != nil {
        return &PlayDecision{Action: ActionPlay, Cards: []*Card{smallest}}
    }
    
    // 出最小的对子
    smallestPair := ai.findSmallestPair(ctx.MyCards)
    if smallestPair != nil {
        return &PlayDecision{Action: ActionPlay, Cards: smallestPair}
    }
    
    return &PlayDecision{Action: ActionPlay, Cards: []*Card{ai.findSmallestCard(ctx.MyCards)}}
}

func (ai *AIEngine) decideResponsePlay(ctx *AIContext, plays []CardCombo) *PlayDecision {
    validPlays := ai.filterBeatingPlays(plays, ctx.LastPlay)
    if len(validPlays) == 0 {
        return &PlayDecision{Action: ActionPass}
    }
    
    // 上家是队友 → 一般不管
    if ai.shouldPassToTeammate(ctx) {
        if ai.isHandWeak(ctx.MyCards) {
            return &PlayDecision{Action: ActionPass}
        }
    }
    
    // 选择最小的能管上的牌
    bestPlay := ai.findMinimumBeatingPlay(validPlays)
    
    // 是否用炸弹？
    if ai.shouldUseBomb(ctx, bestPlay) {
        bombPlay := ai.findSmallestBomb(ctx.MyCards)
        if bombPlay != nil {
            return &PlayDecision{Action: ActionPlay, Cards: bombPlay}
        }
    }
    
    return &PlayDecision{Action: ActionPlay, Cards: bestPlay}
}

func (ai *AIEngine) shouldPassToTeammate(ctx *AIContext) bool {
    if ctx.MyRole == RoleLandlord {
        return false
    }
    lastPlayerRole := ctx.getPlayerRole(ctx.LastPlayerUID)
    if lastPlayerRole != RoleLandlord && ctx.MyRole != RoleLandlord {
        teammateCards := ctx.getCardCount(ctx.LastPlayerUID)
        if teammateCards <= 3 {
            return true
        }
    }
    return false
}

// 以下为占位方法，实际在 pkg/cardutil 和内部逻辑中实现
func (ai *AIEngine) enumerateLegalPlays(ctx *AIContext) []CardCombo { return nil }
func (ai *AIEngine) canPlayAllAtOnce(cards []*Card) bool            { return false }
func (ai *AIEngine) findSmallestSingle(cards []*Card) *Card         { return nil }
func (ai *AIEngine) findSmallestPair(cards []*Card) []*Card         { return nil }
func (ai *AIEngine) findSmallestCard(cards []*Card) *Card           { return nil }
func (ai *AIEngine) filterBeatingPlays(plays []CardCombo, last *PlayRecord) []CardCombo { return nil }
func (ai *AIEngine) isHandWeak(cards []*Card) bool                  { return false }
func (ai *AIEngine) findMinimumBeatingPlay(plays []CardCombo) []*Card { return nil }
func (ai *AIEngine) shouldUseBomb(ctx *AIContext, play *CardCombo) bool { return false }
func (ai *AIEngine) findSmallestBomb(cards []*Card) []*Card         { return nil }
```

### 6.4 记牌器

```go
// internal/ai/counter.go
package ai

type CardCounter struct {
    playedCards    map[CardValue]int
    remainingCards map[string]int
    bigCardHistory []BigCardRecord
}

type BigCardRecord struct {
    CardValue CardValue
    PlayerUID string
    Timestamp int64
}

func NewCardCounter() *CardCounter {
    return &CardCounter{
        playedCards:    make(map[CardValue]int),
        remainingCards: make(map[string]int),
    }
}

func (cc *CardCounter) RecordPlayed(cards []*Card) {
    for _, c := range cards {
        cc.playedCards[c.Value]++
    }
}

func (cc *CardCounter) Remaining() map[CardValue]int {
    remaining := make(map[CardValue]int)
    fullDeck := cc.fullDeck()
    for val, total := range fullDeck {
        played := cc.playedCards[val]
        remaining[val] = total - played
    }
    return remaining
}

func (cc *CardCounter) fullDeck() map[CardValue]int {
    deck := make(map[CardValue]int)
    for v := CardValue3; v <= CardValue2; v++ {
        deck[v] = 4
    }
    deck[CardValueJokerSmall] = 1
    deck[CardValueJokerBig] = 1
    return deck
}
```

---

## 7. Protobuf 协议设计（纯 Message 风格）

### 7.1 消息帧格式

```
┌─────────────────────────────────────────────────┐
│  Message Length (4 bytes, big-endian)           │
├─────────────────────────────────────────────────┤
│  Message ID (2 bytes, big-endian)               │
├─────────────────────────────────────────────────┤
│  Protobuf Payload (variable length)             │
└─────────────────────────────────────────────────┘
```

### 7.2 消息 ID 分配

```
系统消息:     0x0000 - 0x00FF
  0x0001      HeartbeatReq        0x0002      HeartbeatResp
  0x0003      ErrorResponse

认证消息:     0x0100 - 0x01FF
  0x0101      LoginReq            0x0102      LoginResp
  0x0103      LogoutReq

房间消息:     0x0200 - 0x02FF
  0x0201      CreateRoomReq       0x0202      CreateRoomResp
  0x0203      JoinRoomReq         0x0204      JoinRoomResp
  0x0205      LeaveRoomReq        0x0206      RoomStateNotify
  0x0207      PlayerReadyReq      0x0208      PlayerReadyNotify

匹配消息:     0x0300 - 0x03FF
  0x0301      MatchStartReq       0x0302      MatchCancelReq
  0x0303      MatchSuccessNotify  0x0304      MatchTimeoutNotify

游戏消息:     0x0400 - 0x04FF
  0x0401      DealCardsNotify     0x0402      CallLandlordReq
  0x0403      CallLandlordNotify  0x0404      PlayCardsReq
  0x0405      PlayCardsNotify     0x0406      PassNotify
  0x0407      GameEndNotify       0x0408      TimerNotify

断线重连:     0x0500 - 0x05FF
  0x0501      ReconnectReq        0x0502      ReconnectResp

聊天消息:     0x0600 - 0x06FF
  0x0601      ChatReq             0x0602      ChatNotify
```

### 7.3 Protobuf 定义

```protobuf
syntax = "proto3";
package ddz.proto;
option go_package = "go-zero-ddz/proto/ddzpb";

// ============================================
// 公共类型
// ============================================

enum CardValue {
    CARD_VALUE_UNKNOWN = 0;
    CARD_VALUE_3 = 3;
    CARD_VALUE_4 = 4;
    CARD_VALUE_5 = 5;
    CARD_VALUE_6 = 6;
    CARD_VALUE_7 = 7;
    CARD_VALUE_8 = 8;
    CARD_VALUE_9 = 9;
    CARD_VALUE_10 = 10;
    CARD_VALUE_J = 11;
    CARD_VALUE_Q = 12;
    CARD_VALUE_K = 13;
    CARD_VALUE_A = 14;
    CARD_VALUE_2 = 15;
    CARD_VALUE_JOKER_SMALL = 16;
    CARD_VALUE_JOKER_BIG = 17;
}

enum CardSuit {
    CARD_SUIT_UNKNOWN = 0;
    CARD_SUIT_SPADE = 1;
    CARD_SUIT_HEART = 2;
    CARD_SUIT_CLUB = 3;
    CARD_SUIT_DIAMOND = 4;
    CARD_SUIT_JOKER = 5;
}

message Card {
    CardValue value = 1;
    CardSuit suit = 2;
}

enum CardPattern {
    PATTERN_UNKNOWN = 0;
    PATTERN_SINGLE = 1;
    PATTERN_PAIR = 2;
    PATTERN_TRIPLE = 3;
    PATTERN_TRIPLE_ONE = 4;
    PATTERN_TRIPLE_TWO = 5;
    PATTERN_STRAIGHT = 6;
    PATTERN_STRAIGHT_PAIR = 7;
    PATTERN_AIRPLANE = 8;
    PATTERN_AIRPLANE_WINGS = 9;
    PATTERN_FOUR_TWO = 10;
    PATTERN_BOMB = 11;
    PATTERN_ROCKET = 12;
}

message PlayerInfo {
    string uid = 1;
    string nickname = 2;
    uint32 avatar_id = 3;
    int32 elo = 4;
    string tier = 5;
    bool is_bot = 6;
    bool is_online = 7;
    bool is_ready = 8;
    bool is_landlord = 9;
    repeated Card cards = 10;
    int32 card_count = 11;
    int32 score = 12;
    bool is_ai_controlled = 13;
}

enum RoomState {
    ROOM_STATE_WAITING = 0;
    ROOM_STATE_DEALING = 1;
    ROOM_STATE_CALLING = 2;
    ROOM_STATE_PLAYING = 3;
    ROOM_STATE_SETTLEMENT = 4;
}

message GameState {
    RoomState state = 1;
    string room_id = 2;
    repeated PlayerInfo players = 3;
    string landlord_uid = 4;
    int32 current_turn_uid = 5;
    int32 timer = 6;
    int32 base_score = 7;
    int32 multiplier = 8;
    repeated Card last_played_cards = 9;
    string last_played_uid = 10;
    CardPattern last_pattern = 11;
    int32 pass_count = 12;
    repeated Card bottom_cards = 13;
}

// ============================================
// 系统消息 (0x0000 - 0x00FF)
// ============================================

message HeartbeatReq {
    int64 client_timestamp = 1;
}

message HeartbeatResp {
    int64 server_timestamp = 1;
    int32 ping = 2;
}

message ErrorResponse {
    int32 code = 1;
    string message = 2;
    int32 msg_id = 3;
}

// ============================================
// 认证消息 (0x0100 - 0x01FF)
// ============================================

message LoginReq {
    string token = 1;
    string device_id = 2;
    string client_version = 3;
}

message LoginResp {
    bool success = 1;
    string uid = 2;
    string nickname = 3;
    uint32 avatar_id = 4;
    int32 elo = 5;
    string tier = 6;
    int32 gold = 7;
    string session_key = 8;
}

message LogoutReq {
    string reason = 1;
}

// ============================================
// 房间消息 (0x0200 - 0x02FF)
// ============================================

message CreateRoomReq {
    bool is_private = 1;
    string password = 2;
}

message CreateRoomResp {
    bool success = 1;
    string room_id = 2;
    string error = 3;
}

message JoinRoomReq {
    string room_id = 1;
    string password = 2;
}

message JoinRoomResp {
    bool success = 1;
    GameState game_state = 2;
    string error = 3;
}

message LeaveRoomReq {
    string reason = 1;
}

message RoomStateNotify {
    GameState game_state = 1;
    string event = 2;
    PlayerInfo player = 3;
}

message PlayerReadyReq {}

message PlayerReadyNotify {
    string uid = 1;
    bool is_ready = 2;
}

// ============================================
// 匹配消息 (0x0300 - 0x03FF)
// ============================================

message MatchStartReq {
    enum MatchType {
        MATCH_TYPE_RANDOM = 0;
        MATCH_TYPE_RANKED = 1;
    }
    MatchType match_type = 1;
}

message MatchCancelReq {}

message MatchSuccessNotify {
    string room_id = 1;
    repeated PlayerInfo players = 2;
    bool is_ranked = 3;
    int32 estimated_wait = 4;
}

message MatchTimeoutNotify {
    int32 waited_seconds = 1;
    string suggestion = 2;
}

// ============================================
// 游戏消息 (0x0400 - 0x04FF)
// ============================================

message DealCardsNotify {
    repeated Card my_cards = 1;
    repeated Card bottom_cards = 2;
    int32 first_caller_index = 3;
}

message CallLandlordReq {
    enum CallAction {
        CALL_ACTION_PASS = 0;
        CALL_ACTION_CALL = 1;
    }
    CallAction action = 1;
    int32 score = 2;
}

message CallLandlordNotify {
    string uid = 1;
    CallLandlordReq.CallAction action = 2;
    int32 score = 3;
    string landlord_uid = 4;
    repeated Card landlord_cards = 5;
}

message PlayCardsReq {
    repeated Card cards = 1;
}

message PlayCardsNotify {
    string uid = 1;
    repeated Card cards = 2;
    CardPattern pattern = 3;
    int32 card_count = 4;
    bool is_last = 5;
}

message PassNotify {
    string uid = 1;
}

message GameEndNotify {
    string winner_uid = 1;
    enum WinnerSide {
        WINNER_SIDE_LANDLORD = 0;
        WINNER_SIDE_PEASANT = 1;
    }
    WinnerSide winner_side = 2;
    repeated PlayerResult results = 3;
    int32 base_score = 4;
    int32 multiplier = 5;
    bool is_spring = 6;
    bool is_counter_spring = 7;
}

message PlayerResult {
    string uid = 1;
    bool is_landlord = 2;
    int32 score_change = 3;
    int32 new_elo = 4;
    string new_tier = 5;
    bool is_promoted = 6;
    bool is_demoted = 7;
}

message TimerNotify {
    int32 remaining_seconds = 1;
    string current_turn_uid = 2;
}

// ============================================
// 断线重连 (0x0500 - 0x05FF)
// ============================================

message ReconnectReq {
    string session_key = 1;
    string room_id = 2;
}

message ReconnectResp {
    bool success = 1;
    GameState game_state = 2;
    repeated Card my_cards = 3;
    string error = 4;
}

// ============================================
// 聊天消息 (0x0600 - 0x06FF)
// ============================================

message ChatReq {
    enum ChatType {
        CHAT_TYPE_TEXT = 0;
        CHAT_TYPE_EMOJI = 1;
        CHAT_TYPE_VOICE = 2;
    }
    ChatType chat_type = 1;
    string content = 2;
}

message ChatNotify {
    string uid = 1;
    ChatReq.ChatType chat_type = 2;
    string content = 3;
}
```

### 7.4 消息编解码器 (Go)

```go
// internal/websocket/codec.go
package websocket

import (
    "encoding/binary"
    "fmt"
    "io"
)

const (
    HeaderSize     = 6
    MaxMessageSize = 64 * 1024
)

func Encode(msgID uint16, payload []byte) ([]byte, error) {
    if len(payload) > MaxMessageSize {
        return nil, fmt.Errorf("payload too large: %d bytes", len(payload))
    }
    totalLen := 2 + len(payload)
    buf := make([]byte, HeaderSize+len(payload))
    binary.BigEndian.PutUint32(buf[0:4], uint32(totalLen))
    binary.BigEndian.PutUint16(buf[4:6], msgID)
    copy(buf[6:], payload)
    return buf, nil
}

func Decode(reader io.Reader) (uint16, []byte, error) {
    var lengthBuf [4]byte
    if _, err := io.ReadFull(reader, lengthBuf[:]); err != nil {
        return 0, nil, fmt.Errorf("read length: %w", err)
    }
    length := binary.BigEndian.Uint32(lengthBuf[:])
    if length > MaxMessageSize {
        return 0, nil, fmt.Errorf("message too large: %d bytes", length)
    }
    frameData := make([]byte, length)
    if _, err := io.ReadFull(reader, frameData); err != nil {
        return 0, nil, fmt.Errorf("read frame: %w", err)
    }
    msgID := binary.BigEndian.Uint16(frameData[0:2])
    payload := frameData[2:]
    return msgID, payload, nil
}

func DecodeFromBytes(data []byte) (uint16, []byte, error) {
    if len(data) < 4 {
        return 0, nil, fmt.Errorf("data too short: %d bytes", len(data))
    }
    length := binary.BigEndian.Uint32(data[0:4])
    if int(length) != len(data)-4 {
        return 0, nil, fmt.Errorf("length mismatch: header says %d, got %d", length, len(data)-4)
    }
    if len(data) < HeaderSize {
        return 0, nil, fmt.Errorf("frame too short for header: %d bytes", len(data))
    }
    msgID := binary.BigEndian.Uint16(data[4:6])
    payload := data[6:]
    return msgID, payload, nil
}
```

### 7.5 消息编解码器 (TypeScript - Cocos)

```typescript
// assets/scripts/network/MessageCodec.ts
export class MessageCodec {
    encode(msgId: number, payload: Uint8Array): ArrayBuffer {
        const totalLength = 2 + payload.byteLength;
        const buffer = new ArrayBuffer(4 + totalLength);
        const view = new DataView(buffer);
        view.setUint32(0, totalLength, false);
        view.setUint16(4, msgId, false);
        const payloadView = new Uint8Array(buffer, 6);
        payloadView.set(payload);
        return buffer;
    }

    decode(data: Uint8Array): { msgId: number; payload: Uint8Array } {
        const view = new DataView(data.buffer, data.byteOffset, data.byteLength);
        const length = view.getUint32(0, false);
        if (length !== data.byteLength - 4) {
            throw new Error(`Length mismatch: header says ${length}, got ${data.byteLength - 4}`);
        }
        const msgId = view.getUint16(4, false);
        const payload = data.slice(6);
        return { msgId, payload };
    }
}
```

---

## 8. 部署方案

### 8.1 Docker Compose（本地开发）

```yaml
version: '3.8'

services:
  redis:
    image: redis:7-alpine
    ports:
      - "6379:6379"
    command: redis-server --appendonly yes
    volumes:
      - redis-data:/data
    healthcheck:
      test: ["CMD", "redis-cli", "ping"]
      interval: 5s
      timeout: 3s
      retries: 5

  mysql:
    image: mysql:8.0
    environment:
      MYSQL_ROOT_PASSWORD: root123
      MYSQL_DATABASE: ddz
      MYSQL_USER: ddz
      MYSQL_PASSWORD: ddz123
    ports:
      - "3306:3306"
    volumes:
      - mysql-data:/var/lib/mysql
      - ./sql/init.sql:/docker-entrypoint-initdb.d/init.sql
    healthcheck:
      test: ["CMD", "mysqladmin", "ping", "-h", "localhost"]
      interval: 10s
      timeout: 5s
      retries: 5

  game-1:
    build:
      context: .
      dockerfile: Dockerfile
    ports:
      - "8081:8080"
    environment:
      - GAME_CONFIG=etc/game-local.yaml
      - REDIS_HOST=redis:6379
      - MYSQL_DSN=ddz:ddz123@tcp(mysql:3306)/ddz?parseTime=true
    depends_on:
      redis:
        condition: service_healthy
      mysql:
        condition: service_healthy

  game-2:
    build:
      context: .
      dockerfile: Dockerfile
    ports:
      - "8082:8080"
    environment:
      - GAME_CONFIG=etc/game-local.yaml
      - REDIS_HOST=redis:6379
      - MYSQL_DSN=ddz:ddz123@tcp(mysql:3306)/ddz?parseTime=true
    depends_on:
      redis:
        condition: service_healthy
      mysql:
        condition: service_healthy

  nginx:
    image: nginx:alpine
    ports:
      - "80:80"
      - "443:443"
    volumes:
      - ./nginx/nginx.conf:/etc/nginx/nginx.conf:ro
    depends_on:
      - game-1
      - game-2

volumes:
  redis-data:
  mysql-data:
```

### 8.2 Nginx 负载均衡配置

```nginx
worker_processes auto;

events {
    worker_connections 4096;
}

http {
    upstream game_backend {
        least_conn;
        server game-1:8080 max_fails=3 fail_timeout=30s;
        server game-2:8080 max_fails=3 fail_timeout=30s;
    }

    server {
        listen 80;
        server_name ws.ddz.example.com;

        location /ws {
            proxy_pass http://game_backend;
            proxy_http_version 1.1;
            proxy_set_header Upgrade $http_upgrade;
            proxy_set_header Connection "upgrade";
            proxy_read_timeout 86400s;
            proxy_send_timeout 86400s;
            proxy_connect_timeout 10s;
            proxy_buffering off;
            proxy_set_header X-Real-IP $remote_addr;
            proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        }

        location /health {
            proxy_pass http://game_backend/health;
        }
    }
}
```

### 8.3 Kubernetes 部署（生产环境）

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: ddz-game-service
  namespace: ddz
spec:
  replicas: 3
  selector:
    matchLabels:
      app: ddz-game
  template:
    metadata:
      labels:
        app: ddz-game
    spec:
      containers:
      - name: game
        image: registry.example.com/ddz-game:latest
        ports:
        - containerPort: 8080
          name: ws
          protocol: TCP
        - containerPort: 9090
          name: metrics
          protocol: TCP
        env:
        - name: GAME_CONFIG
          value: "etc/game-prod.yaml"
        - name: POD_IP
          valueFrom:
            fieldRef:
              fieldPath: status.podIP
        - name: REDIS_PASSWORD
          valueFrom:
            secretKeyRef:
              name: redis-secret
              key: password
        - name: MYSQL_DSN
          valueFrom:
            secretKeyRef:
              name: mysql-secret
              key: dsn
        resources:
          requests:
            cpu: "500m"
            memory: "512Mi"
          limits:
            cpu: "2000m"
            memory: "2Gi"
        livenessProbe:
          httpGet:
            path: /health
            port: 8080
          initialDelaySeconds: 10
          periodSeconds: 30
          timeoutSeconds: 5
        readinessProbe:
          httpGet:
            path: /ready
            port: 8080
          initialDelaySeconds: 5
          periodSeconds: 10
          timeoutSeconds: 3
        lifecycle:
          preStop:
            exec:
              command: ["/bin/sh", "-c", "kill -SIGTERM 1 && sleep 15"]
---
apiVersion: v1
kind: Service
metadata:
  name: ddz-game-service
  namespace: ddz
spec:
  selector:
    app: ddz-game
  ports:
  - name: ws
    port: 8080
    targetPort: 8080
    protocol: TCP
  - name: metrics
    port: 9090
    targetPort: 9090
    protocol: TCP
  type: ClusterIP
---
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata:
  name: ddz-game-hpa
  namespace: ddz
spec:
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: ddz-game-service
  minReplicas: 2
  maxReplicas: 20
  metrics:
  - type: Resource
    resource:
      name: cpu
      target:
        type: Utilization
        averageUtilization: 70
  - type: Resource
    resource:
      name: memory
      target:
        type: Utilization
        averageUtilization: 80
  - type: Pods
    pods:
      metric:
        name: active_connections
      target:
        type: AverageValue
        averageValue: "5000"
```

### 8.4 Dockerfile

```dockerfile
FROM golang:1.22-alpine AS builder
WORKDIR /build
RUN apk add --no-cache git make
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG TARGETARCH
RUN CGO_ENABLED=0 GOOS=linux GOARCH=${TARGETARCH} \
    go build -ldflags="-s -w" -o /build/bin/game-service ./app/game/cmd/server

FROM alpine:3.19
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app
COPY --from=builder /build/bin/game-service /app/game-service
COPY --from=builder /build/etc /app/etc
EXPOSE 8080
ENTRYPOINT ["/app/game-service"]
CMD ["-f", "/app/etc/game.yaml"]
```

---

## 9. 扩展与运维

### 9.1 容量规划

```
单实例资源需求:
  CPU:    0.5 - 2 core
  Memory: 512MB - 2GB   (1GB 可承载 ~15,000 连接)
  Network: 100Mbps

5000 房间 (15,000 玩家) 需求:
  实例数: 2-3 台
  Redis:  单节点足够，建议 Sentinel 或 Cluster
  MySQL:  单节点足够

50,000 房间 (150,000 玩家) 需求:
  实例数: 15-20 台
  Redis:  Redis Cluster (6 节点)
  MySQL:  主从复制 + 读写分离
  LB:     云厂商负载均衡器
```

### 9.2 优雅关闭流程

```
1. 接收 SIGTERM 信号
2. 停止接收新连接
3. 更新 Redis 实例状态为 "draining"
4. 等待现有房间结束 (或迁移)
5. 注销玩家路由 (批量 DEL)
6. 注销实例 (DEL ddz:instance:{id})
7. 关闭 Redis 连接池
8. 退出进程
```

### 9.3 监控指标

```go
var (
    ActiveConnections = prometheus.NewGaugeVec(
        prometheus.GaugeOpts{Name: "ddz_active_connections"},
        []string{"instance_id"},
    )
    ActiveRooms = prometheus.NewGaugeVec(
        prometheus.GaugeOpts{Name: "ddz_active_rooms"},
        []string{"instance_id"},
    )
    MessagesProcessed = prometheus.NewCounterVec(
        prometheus.CounterOpts{Name: "ddz_messages_processed_total"},
        []string{"instance_id", "message_type"},
    )
    MessageLatency = prometheus.NewHistogramVec(
        prometheus.HistogramOpts{Name: "ddz_message_latency_ms"},
        []string{"instance_id"},
    )
    CrossInstanceMessages = prometheus.NewCounterVec(
        prometheus.CounterOpts{Name: "ddz_cross_instance_messages_total"},
        []string{"instance_id", "direction"},
    )
)
```

### 9.4 故障场景处理

| 故障场景 | 影响 | 恢复机制 |
|----------|------|----------|
| 单实例宕机 | 该实例上所有玩家断线 | 客户端自动重连 → LB 路由到新实例 → 从 Redis 恢复 |
| Redis 宕机 | 无法路由玩家、无法跨实例通信 | Redis Sentinel 自动故障转移 |
| 网络分区 | 部分实例间无法通信 | 房间归属实例继续运行；跨实例玩家超时后重连 |
| 玩家断线 | 玩家离开房间 | 300s 内重连可恢复；超时则视为离开 |

---

## 10. 配置切换指南

```bash
# 单机开发
$ ./game-service -f etc/game-local.yaml

# 集群模式 - 实例 1
$ ./game-service -f etc/game-prod.yaml \
    --cluster.enabled=true \
    --cluster.host=10.0.1.10

# 集群模式 - 实例 2  
$ ./game-service -f etc/game-prod.yaml \
    --cluster.enabled=true \
    --cluster.host=10.0.1.11

# Docker Compose 一键启动集群
$ docker-compose up -d

# Kubernetes 自动扩缩
$ kubectl apply -f k8s/
$ kubectl scale deployment ddz-game-service --replicas=5
```

**核心原则**：`Cluster.Enabled = false` 时，所有跨实例逻辑跳过，退化为纯单机模式。

---

## 11. 更新后的项目结构

```
go-zero-ddz/
├── proto/
│   ├── common.proto                # 公共类型（Card, PlayerInfo, GameState）
│   └── messages.proto              # 所有消息定义（纯 message 风格）
├── app/
│   ├── user/                       # 用户服务 (go-zero api)
│   ├── user-rpc/                   # 用户 RPC 服务 (go-zero rpc)
│   └── game/                       # 游戏服务（独立 WS 服务）
│       ├── etc/
│       │   ├── game-local.yaml     # 单机开发配置
│       │   ├── game-prod.yaml      # 生产集群配置
│       │   └── ai.yaml             # AI 难度配置
│       ├── cmd/server/main.go
│       ├── internal/
│       │   ├── config/
│       │   ├── websocket/          # WS 连接管理
│       │   │   ├── client.go
│       │   │   ├── hub.go
│       │   │   ├── codec.go        # 消息帧编解码
│       │   │   └── handler.go      # 消息路由与处理
│       │   ├── cluster/            # 集群组件
│       │   │   ├── registry.go     # 实例注册与发现
│       │   │   ├── router.go       # 玩家路由管理
│       │   │   └── message_bus.go  # 跨实例消息总线
│       │   ├── room/               # 房间管理
│       │   │   ├── manager.go
│       │   │   ├── room.go
│       │   │   └── state.go
│       │   ├── match/              # 匹配系统
│       │   │   ├── coordinator.go  # 匹配协调器
│       │   │   ├── queue.go        # 匹配队列
│       │   │   └── elo.go          # ELO 积分计算
│       │   ├── ai/                 # AI 系统
│       │   │   ├── engine.go       # AI 决策引擎
│       │   │   ├── rules.go        # 规则引擎
│       │   │   ├── strategy.go     # 策略引擎
│       │   │   ├── counter.go      # 记牌器
│       │   │   └── bot.go          # AI 机器人
│       │   ├── game/               # 游戏核心逻辑
│       │   │   ├── cards.go
│       │   │   ├── deck.go
│       │   │   ├── pattern.go      # 牌型判定
│       │   │   ├── compare.go      # 大小比较
│       │   │   └── rules.go
│       │   └── svc/
│       │       └── service_context.go
│       └── proto/                  # 生成的 pb.go
├── pkg/
│   ├── cardutil/                   # 牌型工具（可独立测试）
│   └── redisutil/
├── docs/
│   └── cluster-deployment-plan.md  # 本文档
├── docker-compose.yml
├── Dockerfile
├── Makefile
└── go.mod
```

---

## 12. 完整开发计划

| 阶段 | 内容 | 可交付物 | 预估 |
|------|------|----------|------|
| **P0** | Protobuf 协议定义 | `proto/*.proto` 可编译 | 1天 |
| **P1** | 牌型判定库 + 测试 | `pkg/cardutil` 100% 覆盖 | 2天 |
| **P2** | go-zero user api + rpc | 登录/注册/Token | 1天 |
| **P3** | WS Gateway + 消息编解码 | 连接、心跳、消息路由 | 2天 |
| **P4** | 集群组件（注册、路由、消息总线） | 多实例通信 | 2天 |
| **P5** | RoomManager + 状态机 | 创建/加入/离开 | 2天 |
| **P6** | 匹配系统（随机 + 段位 + ELO） | 匹配队列 + 协调器 | 2天 |
| **P7** | AI 出牌引擎 + 机器人 | 自动出牌 + 记牌 | 3天 |
| **P8** | 发牌 → 叫地主 → 出牌 → 结算 | 完整一局 | 3天 |
| **P9** | 断线重连 + 超时托管 | 异常场景 | 2天 |
| **P10** | Cocos 前端对接 | WS 客户端 + UI | 3天 |
| **P11** | 联调 + Docker 部署 | 可运行 Demo | 2天 |

**总计：约 25 个工作日（单人）**
