# app/game/ — game-service (WebSocket, 全手写)

> 斗地主游戏核心服务。WebSocket 协议 8080。**完全手写，不用 goctl**。

## OVERVIEW

职责：房间生命周期、叫地主、出牌、结算、匹配、AI 托管、跨实例路由。

入口：`cmd/server/main.go` → `internal/svc/service_context.go` 装配所有组件。

## WHERE TO LOOK

| 任务 | 位置 |
|------|------|
| 改配置加载 | `internal/config/config.go`（**自定义 `LoadConfig`，不**用 `conf.MustLoad`） |
| 改启动流程 | `internal/svc/service_context.go`（依赖装配 + Start/Stop 生命周期） |
| 改 WS 消息路由 | `internal/handler/handler.go`（HandlerManager） |
| 改房间/出牌/叫地主 | `internal/room/`（state machine 在 `state.go`） |
| 改 AI 出牌 | `internal/ai/`（注意陷阱 #8） |
| 改匹配/段位 | `internal/match/`（ELO 算法在 `elo.go`） |
| 改 WS 帧协议 | `internal/websocket/codec.go` |
| 改 WS 连接管理 | `internal/websocket/hub.go`（gorilla/websocket） |
| 改集群模式 | `internal/cluster/`（registry/router/message_bus） |
| 改错误码 | `internal/errors/errors.go` |

## 目录树

```
app/game/
├── cmd/server/main.go         # 入口（49 行）
├── etc/                        # YAML 配置
│   ├── game-local.yaml         # 本地
│   ├── game-prod.yaml          # 生产集群
│   └── ai.yaml                 # AI 难度
├── proto/                       # 生成的 pb.go（当前为空）
└── internal/
    ├── config/                 # 配置结构体 + LoadConfig
    ├── svc/                    # ServiceContext 装配（398 行）
    ├── handler/                # WS 消息处理器（5 个文件）
    ├── websocket/              # Hub + Client + Codec
    ├── room/                   # 房间 + 状态机
    ├── match/                  # 匹配 + ELO
    ├── ai/                     # AI 决策引擎
    ├── cluster/                # 集群组件
    ├── game/                   # 游戏核心逻辑（call/play/settlement）
    └── errors/                 # 错误码 + GameError
```

## 数据流

```
Client → /ws (HTTP upgrade)
       → websocket.Hub (注册/路由/统计)
       → handler.HandlerManager (按 msgID 分发)
       → game.GameLogic / room.Manager
       → redis (玩家路由 + 房间快照 + 匹配队列 + 跨实例 Pub/Sub)
       → mysql (用户/对局，可选)
```

## CONVENTIONS

- **配置加载**: `config.LoadConfig(path)` 加载 YAML + 环境变量覆盖（`GAME_*` 前缀）+ 校验
- **日志**: `logx`（go-zero 标准）
- **并发**: 共享状态用 `sync.RWMutex`（房间状态、Hub 客户端表）
- **JSON 字段**: snake_case
- **错误**: 优先用 `internal/errors/` 的预定义 `*GameError`，不要裸 `fmt.Errorf`
- **goroutine 启动**: 入口 `service_context.go` 启动；goroutine 内不接受新连接、不阻塞
- **HTTP 端点**: 同 `http.Server` 复用 8080（WS + `/health` + `/ready` + 静态文件 + 可选 `/metrics`）

## ANTI-PATTERNS

| # | 陷阱 | 说明 |
|---|------|------|
| 5 | **用 `conf.MustLoad`** | game-service 用自定义 `LoadConfig`，保留环境变量覆盖与校验 |
| 6 | **状态机重复创建** | `NewGameStateMachine()` 每次新实例，应在 `Room` 中**持有引用**（`Room.GameState`） |
| 8 | **`IsAIControlled` 未重置** | 进入出牌阶段前**必须**重置非机器人玩家的 `IsAIControlled=false`，否则用户被莫名托管 |
| 9 | **Dockerfile Go 版本** | 当前 `golang:1.22-alpine`，需升级为 `1.25-alpine`（go.mod 已 1.25.1） |

## 健康检查

| 端点 | 用途 | 返回 |
|------|------|------|
| `/health` | 存活 | 200 OK |
| `/ready` | 就绪 | 200 READY |
| `/metrics` | Prometheus（需 `Metrics.Enabled`） | text/plain |

## 集群模式

`Cluster.Enabled=false` 退化为单机模式（最常用）。启用时 `internal/cluster/` 三件套：
- `Registry` — 实例注册 + 30s TTL 心跳
- `Router` — 玩家 UID → (instance_id, room_id, conn_id) 路由表（5min TTL）
- `MessageBus` — Redis Pub/Sub 跨实例消息（`MsgTypePlay/Control/Broadcast/Reconnect`）
