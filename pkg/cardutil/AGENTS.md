# pkg/cardutil/ — 斗地主牌型判定库

> 独立可测试的牌型库。**所有牌型识别必须走此包**，禁止业务层重复实现。

## OVERVIEW

提供 `Card`/`CardValue`/`CardSuit`/`CardPattern` 类型，以及：
- `AnalyzePattern(cards []Card) PlayResult` — 牌型分析
- `CompareCards` — 大小比较
- `NewFullDeck` / `DealCards` — 牌堆管理

`pattern_test.go` 469 行覆盖 **30+ 场景**（覆盖率 > 90%）。

## WHERE TO LOOK

| 任务 | 位置 |
|------|------|
| 牌定义 | `card.go`（`CardValue` 3-17、`CardSuit` 1-5、`Card` 结构） |
| 牌型识别 | `pattern.go`（`AnalyzePattern` 入口 + `analyzeSingle/Double/Triple/Four/Multi`） |
| 大小比较 | `compare.go` |
| 牌堆/发牌 | `deck.go`（`NewFullDeck`、`DealCards`） |
| 测试 | `pattern_test.go`（30+ 用例） |

## 牌型枚举（`card.go`）

```go
const (
    PatternUnknown       CardPattern = 0
    PatternSingle        CardPattern = 1   // 单张
    PatternPair          CardPattern = 2   // 对子
    PatternTriple        CardPattern = 3   // 三条
    PatternTripleOne     CardPattern = 4   // 三带一
    PatternTripleTwo     CardPattern = 5   // 三带二
    PatternStraight      CardPattern = 6   // 顺子（≥5）
    PatternStraightPair  CardPattern = 7   // 连对（≥3 对）
    PatternAirplane      CardPattern = 8   // 飞机（≥2 连续三条）
    PatternAirplaneWings CardPattern = 9   // 飞机带翅膀
    PatternFourTwo       CardPattern = 10  // 四带二
    PatternBomb          CardPattern = 11  // 炸弹
    PatternRocket        CardPattern = 12  // 王炸
)
```

## CONVENTIONS

- **牌值**：`CardValue3=3 ... CardValue2=15, CardValueJokerS=16, CardValueJokerB=17`
- **王炸**：`CardValueJokerS` + `CardValueJokerB`，是最大牌型
- **2 和王**不参与顺子/连对/飞机
- **牌字符串**：`Card.String()` 返回 `S3`/`H10`/`BJ` 格式（suit + value）
- **不持有**状态：所有函数纯函数，无全局变量、无 init()
- **排序**：调用方负责，`AnalyzePattern` 内部会拷贝并 `slices.SortFunc`

## 入口函数

```go
func AnalyzePattern(cards []Card) PlayResult
// 返回: {Valid bool, Pattern CardPattern, Main CardValue, Length int}
```

调用链：`analyzeSingle(1) → analyzeDouble(2) → analyzeTriple(3) → analyzeFour(4) → analyzeMulti(5+)`。

## ANTI-PATTERNS

- ❌ **不要**在 AI/room 层写自己的牌型识别（必须用 `AnalyzePattern`）
- ❌ **不要**修改 `CardValue` 整数值（向前兼容）
- ❌ **不要**修改 `PatternUnknown=0` 的默认值（零值语义：`Valid=false`）
- ❌ **不要**跳过测试新增牌型识别（先写测试，违反 TDD）

## 测试

```bash
make test-cardutil
# 等价: go test -v -race -cover ./pkg/cardutil/...
```

要求覆盖率 > 90%。`pattern_test.go` 命名规范：`TestAnalyzePattern_<子场景>`，例：
- `TestAnalyzePattern_Single`
- `TestAnalyzePattern_Bomb`
- `TestAnalyzePattern_Rocket`
- `TestAnalyzePattern_Straight_5Cards`
- `TestDealCards_ThreePlayers`
