# app/game/internal/room/ — 房间 + 状态机

> 房间生命周期 + 游戏状态机的核心。`manager.go` 808 行（最大），`state.go` 386 行（状态机）。

## OVERVIEW

`Manager` 管理 `map[string]*Room`（roomsMu 保护），持有 AI 引擎、Hub、Redis。
`Room` 持有玩家、手牌、底牌、当前回合、计时器、状态机引用。
`GameStateMachine` 封装发牌/叫地主/出牌规则。

## WHERE TO LOOK

| 任务 | 位置 |
|------|------|
| 创建/销毁房间 | `manager.go`（`CreateRoom`/`RemoveRoom`） |
| 房间配置 | `RoomConfig` 结构（`manager.go`） |
| 玩家结构 | `Player` 结构（`room.go`） |
| 房间回调 | `OnStateChange/OnTimeout/OnBotJoinTimeout/OnBotJoinCountdown`（`room.go`） |
| 发牌 | `GameStateMachine.DealCards`（`state.go`） |
| 叫地主 | `GameStateMachine.CallLandAllCalled` + `app/game/internal/game/call.go` 业务封装 |
| 出牌 | `GameStateMachine.PlayCards` + `app/game/internal/game/play.go` 业务封装 |
| 状态转换 | `state.go` 内部状态机逻辑 |
| 快照持久化 | `manager.go` 的 `snapshotLoop`（默认 30s） |

## 房间状态机

```
StateWaiting → StateDealing → StateCalling → StatePlaying → StateSettlement → 销毁
```

定义在 `pkg/types/game.go` 的 `RoomState` 枚举。

## CONVENTIONS

- **每个 Room 创建后应立刻**调用 `SetOnStartGame` 等回调（由 `service_context.go` 设置）
- **状态机** 通过 `Room.GameState *GameStateMachine` 持有引用（**不**每次 `NewGameStateMachine`）
- **并发**：所有 `Room.Players`/`State` 修改都在 `room.mu` 保护下
- **计时器**：`timer *time.Timer` + `timerLock`（防止 Stop 与 Stop 竞态）
- **Redis 快照**：`snapshotLoop` 异步周期写入（`ddz:room:snapshot:{id}`）
- **断线**：`Player.DisconnectAt time.Time`，超时后 AI 接管

## 关键陷阱

| # | 陷阱 | 说明 |
|---|------|------|
| 6 | **状态机重复创建** | `NewGameStateMachine()` 每次都新建实例，状态丢失。**Room 持有 `GameState` 引用** |
| 8 | **`IsAIControlled` 状态** | 进入出牌阶段前**必须**把非机器人玩家的 `IsAIControlled=false` |
| - | **出牌轮次记录** | `PlayRounds []types.PlayRoundRecord` 用于回放，写入时机：`PlayCards` 成功后 |
| - | **底牌** | `BottomCards` 仅在 `StateCalling → StatePlaying` 转换时公开给地主 |

## 测试

`room_test.go` 342 行覆盖：3 玩家发牌、叫地主流程、AI 托管、并发安全。
```bash
go test ./app/game/internal/room/ -v -race
```
