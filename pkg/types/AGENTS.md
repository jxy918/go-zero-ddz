# pkg/types/ — 共享类型与常量

> **唯一真理源**。所有模块（app/user、app/game、pkg/cardutil）必须从这里引用，禁止重复定义。

## OVERVIEW

三个文件覆盖：消息 ID、游戏状态枚举、通用结构体。

## 文件划分

| 文件 | 内容 |
|------|------|
| `message.go` | WS 消息 ID 常量（`uint16` 范围 0x00xx-0x05xx） |
| `game.go` | 玩家角色、房间状态、胜利方、AI 难度枚举 + 游戏常量 |
| `types.go` | `PlayerSettlement`、`SettlementResult`、`CallRecord`、`PlayerInfo`、`MatchResult` |

## 消息 ID 范围（`message.go`）

| 范围 | 模块 | 常量 |
|------|------|------|
| `0x00xx` | 系统 | `MsgHeartbeatReq/Resp`, `MsgErrorResponse` |
| `0x01xx` | 认证 | `MsgLoginReq/Resp` |
| `0x02xx` | 房间 | `MsgCreateRoomReq/Resp`, `MsgJoinRoomReq/Resp`, `MsgRoomStateNotify`, `MsgPlayerReadyReq` |
| `0x03xx` | 匹配 | `MsgMatchStartReq`, `MsgMatchCancelReq`, `MsgMatchSuccessNotify` |
| `0x04xx` | 游戏 | `MsgDealCardsNotify`, `MsgCallLandlordReq/Notify`, `MsgPlayCardsReq/Notify`, `MsgPassNotify`, `MsgGameEndNotify`, `MsgTimerNotify`, `MsgCancelAIControlReq` |
| `0x05xx` | 重连 | `MsgReconnectReq/Resp` |

## 枚举（`game.go`）

```go
type PlayerRole int
const (RoleUnknown, RoleLandlord, RolePeasant)

type RoomState int
const (StateWaiting, StateDealing, StateCalling, StatePlaying, StateSettlement)

type WinnerSide int
const (WinnerSideLandlord, WinnerSidePeasant)

type AIDifficulty int
const (AIEasy=1, AINormal=2, AIHard=3)
```

**注意**：`AIDifficulty` 是枚举，但 `AIContext.Difficulty` 字段是字符串（见 ai/AGENTS.md）。

## 游戏常量（`game.go`）

```go
MaxRoomSize       = 3
MinRoomSize       = 2
TotalCards        = 54
LandlordCards     = 20
PeasantCards      = 17
BottomCards       = 3
CallLandlordTimer = 15  // 秒
PlayCardsTimer    = 15  // 秒
MatchTimeout      = 30  // 秒
```

## CONVENTIONS

- **优先引用**: `import "go-zero-ddz/pkg/types"`
- **类型安全**: 强类型枚举（`types.PlayerRole`），不要用 `int` 替代
- **新增类型**: 在对应文件追加，**不**在新文件散布
- **WS 消息 ID**: 新增消息需选正确范围（0x00xx-0x05xx），避免冲突

## ANTI-PATTERNS

- ❌ **重复定义** 角色/状态/消息 ID（违规 #7，代码评审卡住）
- ❌ **在 logic 层定义** `const MsgXxx = 0x0401`（必须从 `types` 引用）
- ❌ **改枚举整数值**（向前兼容：`AIEasy=1, AINormal=2, AIHard=3`，不要重排）
- ❌ **magic number**：`MaxRoomSize=3` 等已定义为常量，引用即可
