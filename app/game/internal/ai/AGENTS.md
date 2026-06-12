# app/game/internal/ai/ — AI 决策引擎

> 斗地主 AI 出牌决策。**项目最大模块**（engine.go 1281 行 + engine_test.go 1160 行）。

## OVERVIEW

三种难度：easy（随机）、normal（最小牌型）、hard（记牌+推理）。
`Engine` 结构持有 `*AIConfig` 和 `*rand.Rand`；`DecidePlay(ctx *AIContext) *PlayDecision` 是入口。

## WHERE TO LOOK

| 文件 | 职责 |
|------|------|
| `engine.go` | 核心决策（DecidePlay、enumerateLegalPlays、PassToTeammate、UseBomb、isHandWeak） |
| `counter.go` | 记牌器（CardCounter）—— 跟踪已出牌，推断剩余牌型 |
| `strategy.go` | 策略辅助（牌型价值、拆牌代价、局势评分） |
| `rules.go` | 牌型判定封装（基于 `pkg/cardutil`） |
| `bot.go` | 机器人管理（AI vs Bot 概念区分） |
| `engine_test.go` | 1160 行测试，覆盖 30+ 场景 |

## CONVENTIONS

- **难度通过 `AIContext.Difficulty` 字符串** 传递（`easy` / `normal` / `hard`），非枚举
- **默认放过队友概率** 在 `NewAIEngine` 中设置：
  - ≤3 张牌 → 80% 放过
  - ≤5 张牌 → 50% 放过
  - ≤7 张牌 → 30% 放过
- **响应延迟**：`DelayMsMin/Max` 模拟人类出牌节奏（300-800ms 经验值）
- **枚举所有合法出牌** → 评估 → 选最优。`enumerateLegalPlays` 必须支持**所有**牌型（顺子、连对、飞机、王炸等），这是 `.trae/specs/ai-strategy-optimization` 的核心改进点

## 数据结构

```go
type AIContext struct {
    MyCards       []cardutil.Card
    LastPlay      *LastPlayInfo
    LastPlayerUID string
    MyRole        types.PlayerRole   // 地主/农民
    Players       map[string]*PlayerInfo
    CardCounter   *CardCounter        // 记牌器（hard 模式使用）
    Difficulty    string              // easy | normal | hard
}
```

## ANTI-PATTERNS

- ❌ **不要**在 `Engine` 持有房间状态（Engine 无状态，房间通过 `AIContext` 注入）
- ❌ **不要**在 AI 决策里 `time.Sleep` 真实等待（用 `ResponseRate + DelayMs` 由调用方控制节奏）
- ❌ **不要**修改 `enumerateLegalPlays` 跳过 `pkg/cardutil`（共享类型，违规 #7）
- ❌ **不要**新增牌型识别逻辑到 AI（必须走 `pkg/cardutil.AnalyzePattern`）

## 测试

`engine_test.go` 1160 行，必须保持覆盖率 > 90%。运行：
```bash
make test-cardutil    # 牌型库
go test ./app/game/internal/ai/ -v
```

修改 AI 策略后必须跑 E2E：3 个机器人对局 1000 局，胜率分布合理。
