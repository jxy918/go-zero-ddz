# AGENTS.md — go-zero-ddz 开发规范手册

> 斗地主联机游戏后端。Go 1.25 + go-zero 1.10.1 + WebSocket + Redis + MySQL。

---

## 一、项目概览

### 服务拓扑

| 服务 | 入口 | 端口 | 协议 | 代码生成 | 描述 |
|------|------|------|------|----------|------|
| **user-api** | `app/user/user.go` | 8888 | HTTP/JSON | `goctl api go` | 用户注册/登录/JWT |
| **game-service** | `app/game/cmd/server/main.go` | 8080 | WebSocket | 手写 | 房间/匹配/出牌/AI |

### 共享库

| 包 | 路径 | 说明 |
|----|------|------|
| `pkg/cardutil/` | 牌型库 | 30+ 测试用例，独立可测试 |
| `pkg/types/` | 类型定义 | 共享常量、枚举、结构体定义 |

### 数据流

```
Client → Nginx (LB/WS) → Game-Instance → Redis (Pub/Sub + 快照 + 路由)
                                          → MySQL (用户/对局)
```

---

## 二、环境与快速开始

### 2.1 前置要求

- Go 1.25+（go.mod: `go 1.25.1`）
- Redis 6+（必选）
- MySQL 8+（可选，支持**内存模式**）
- `protoc + protoc-gen-go`（仅 proto 修改时需要）
- `goctl v1.10.1`（仅 user-api 修改时需要）

### 2.2 常用命令

```bash
# ═══════════════ 构建 ═══════════════
go build ./app/user/                # user-api
go build ./app/game/cmd/server      # game-service
go build ./...                      # 全量构建

# ═══════════════ 运行 ═══════════════
go run ./app/user/ -f app/user/etc/user-api.yaml
go run ./app/game/cmd/server/ -f app/game/etc/game-local.yaml

# ═══════════════ 测试 ═══════════════
go test ./pkg/cardutil/... -v -cover   # 牌型库测试
go test ./... -v                        # 全量测试
go test -race ./...                     # 竞态检测

# ═══════════════ 基础设施 ═══════════════
make docker-up                # 启动 Redis + MySQL
make docker-down              # 停止

# ═══════════════ 代码生成 ═══════════════
cd app/user && goctl api go -api user.api -dir . -style gozero

# ═══════════════ 质量保障 ═══════════════
make lint                     # golangci-lint
make fmt                      # go fmt

# ═══════════════ Proto ═══════════════
make proto                    # protoc 编译
```

### 2.3 内存模式

MySQL DSN 为空或不可达时，**user-api** 自动降级为内存模式：
- 使用硬编码模拟用户（仅 `test/123` 可用）
- 数据不持久化
- 日志标注 `[Memory Mode]`

---

## 三、目录结构与职责

```
go-zero-ddz/
│
├── app/                                  # 应用服务
│   ├── user/                             # ── user-api（goctl 生成骨架）
│   │   ├── user.api                      #    API 定义（唯一真理源）
│   │   ├── user.go                       #   入口（goctl 生成，禁止编辑）
│   │   ├── etc/user-api.yaml             #   配置
│   │   └── internal/
│   │       ├── config/config.go          #   配置结构体（嵌入 rest.RestConf）
│   │       ├── handler/                  #   HTTP 处理器（goctl 生成，禁止编辑）
│   │       │   ├── routes.go             #   ← 唯一需手动调整的生成文件
│   │       │   ├── loginhandler.go
│   │       │   ├── registerhandler.go
│   │       │   └── userinfohandler.go
│   │       ├── logic/                    #   业务逻辑（在此填写代码）
│   │       │   ├── loginlogic.go
│   │       │   ├── registerlogic.go
│   │       │   └── userinfologic.go
│   │       ├── svc/servicecontext.go     #   服务上下文（可编辑）
│   │       └── types/types.go            #   请求/响应类型（goctl 生成，禁止编辑）
│   │
│   ├── game/                             # ── game-service（手写，无代码生成）
│   │   ├── cmd/server/main.go            #   入口
│   │   ├── etc/
│   │   │   ├── game-local.yaml           #   本地开发配置
│   │   │   ├── game-prod.yaml            #   生产集群配置
│   │   │   └── ai.yaml                   #   AI 难度配置
│   │   ├── proto/                        #   生成的 pb.go
│   │   └── internal/
│   │       ├── config/config.go          #   配置结构体（自定义 LoadConfig）
│   │       ├── ai/                       #   AI 出牌引擎
│   │       │   ├── engine.go             #   决策引擎
│   │       │   ├── counter.go            #   记牌器
│   │       │   ├── strategy.go           #   策略
│   │       │   ├── rules.go              #   规则
│   │       │   └── bot.go                #   机器人管理
│   │       ├── cluster/                  #   多实例集群
│   │       │   ├── registry.go           #   实例注册/心跳
│   │       │   ├── router.go             #   玩家路由
│   │       │   └── message_bus.go        #   Redis Pub/Sub
│   │       ├── game/                     #   游戏核心逻辑
│   │       │   ├── call.go               #   叫地主逻辑
│   │       │   └── settlement.go         #   结算逻辑
│   │       ├── handler/handler.go         #   WS 消息处理器
│   │       ├── match/                    #   匹配系统
│   │       │   ├── coordinator.go
│   │       │   ├── queue.go
│   │       │   └── elo.go
│   │       ├── room/                     #   房间管理
│   │       │   ├── manager.go
│   │       │   ├── room.go
│   │       │   └── state.go
│   │       ├── svc/service_context.go    #   服务上下文
│   │       └── websocket/                #   WS 连接管理
│   │           ├── hub.go                #   连接中心
│   │           ├── client.go             #   客户端
│   │           └── codec.go              #   消息编解码
│   │
│   └── user-rpc/                         # 预留 gRPC 服务
│
├── pkg/                                  # 共享库
│   ├── cardutil/                         # 牌型判定（独立可测试）
│   │   ├── card.go                       #   牌定义
│   │   ├── pattern.go                    #   牌型分析
│   │   ├── compare.go                    #   牌型比较
│   │   ├── deck.go                       #   牌组管理
│   │   └── pattern_test.go               #   单元测试（30+ 用例）
│   └── types/                            # 类型定义（共享常量与结构体）
│       ├── message.go                    #   消息ID常量
│       ├── game.go                       #   游戏状态、角色、常量
│       └── types.go                      #   通用结构体
│
├── proto/                                # Protobuf 定义
│   ├── common.proto                      # 公共类型
│   ├── messages.proto                    # 消息定义
│   └── ddzpb/                            # 生成的 Go 代码
│
├── sql/init.sql                          # 数据库初始化
│
├── docs/                                 # 设计文档
│   ├── go-zero-api-development-spec.md   # API 开发规范
│   └── cluster-deployment-plan.md        # 集群部署方案
│
├── nginx/nginx.conf                      # Nginx 负载均衡配置
├── docker-compose.yml                    # Docker 编排
├── Dockerfile                            # 游戏服务镜像
├── k8s/                                  # Kubernetes 部署
├── Makefile                              # 构建脚本
├── AGENTS.md                             # ← 当前文件（开发规范总纲）
└── README.md                             # 项目介绍
```

### 文件所有权矩阵

| 目录 | 所有权 | 说明 |
|------|--------|------|
| `app/user/internal/handler/` | ❌ goctl 生成 | 禁止编辑，重新生成会覆盖 |
| `app/user/internal/logic/` | ✅ 开发者 | 骨架由 goctl 生成，逻辑自己填 |
| `app/user/internal/types/` | ❌ goctl 生成 | 禁止编辑 |
| `app/user/internal/config/` | ⚠️ 手动维护 | goctl 会跳过已有文件 |
| `app/user/internal/svc/` | ⚠️ 手动维护 | goctl 会跳过已有文件 |
| `app/user/internal/handler/routes.go` | ⚠️ 生成后需手动 | 重新生成后需恢复路由分离 |
| `app/game/internal/` | ✅ 全部手写 | 无代码生成 |
| `pkg/cardutil/` | ✅ 手动 | 独立可测试库 |
| `pkg/types/` | ✅ 手动 | 共享类型定义，所有模块引用 |

---

## 四、编码规范

### 4.1 通用 Go 规范

- **Go 版本**: 1.25（使用 `go 1.25.1`）
- **格式化**: `go fmt`（强制），不允许 `goimports` 以外的格式化器
- **导入顺序**: 标准库 → 第三方库 → 内部包（空行分隔）
- **命名**:
  - 接口: `-er` 后缀（`Reader`、`Handler`）
  - 错误: 以 `Err` 前缀（`var ErrNotFound = errors.New(...)`）
  - 包名: 全小写，单数，无下划线（`cardutil`、`types`）
- **错误处理**: 永远不要 `_ =` 忽略错误；业务错误用 `errors.New()`，系统错误用 `fmt.Errorf("context: %w", err)`

### 4.2 项目特有约定

| 类别 | 规则 | 示例 |
|------|------|------|
| **包名** | 全小写，单数。避免 `utils`、`common`、`lib` | `cardutil` ✅，`card_util` ❌ |
| **文件名** | 蛇形命名 | `service_context.go` |
| **结构体名** | 驼峰，全称不缩写 | `ServiceContext` ✅，`SvcCtx` ❌ |
| **接口名** | `-er` 结尾 | `PlayerRouter` |
| **错误变量** | `Err` 前缀 | `ErrPlayerNotFound` |
| **常量** | 驼峰 | `MaxRoomSize = 3` |
| **测试文件** | `_test.go` 后缀，与被测文件同目录 | `pattern_test.go` |
| **Mock** | 同目录下 `mock_xxx.go` | `mock_cardutil.go` |

### 4.3 user-api 规范

**严格遵循 goctl 工作流**：

```
编辑 .api 文件 → goctl 生成代码 → 填写 logic → 调整 routes.go → 编译
```

关键点：
1. **`routes.go`** 每次重新生成后必须手动分离公开路由和 JWT 路由
2. **`ServiceContext`** 签名必须为 `NewServiceContext(c config.Config) *ServiceContext`（不能返回 error）
3. **配置标签** `json:",default=..."` 和 `json:",optional"` 是 go-zero 特有语法，LSP 报 "unknown JSON option" 是**误报**，忽略
4. **Handler 层禁止修改**，所有业务逻辑在 logic 层
5. **错误返回** 使用 `errors.New("友好提示")`，不要暴露内部错误

### 4.4 game-service 规范

- 不使用 goctl，全部手写
- 配置使用自定义 `config.LoadConfig()`，而非 go-zero 的 `conf.MustLoad()`
- 房间状态使用 `sync.RWMutex` 保护
- AI 引擎必须实现 `easy/normal/hard` 三种难度
- 超时机制：15 秒不出牌 → AI 自动托管

### 4.5 pkg/types 使用规范

- **优先引用**: 所有模块应从 `pkg/types` 引用共享常量和结构体
- **禁止重复定义**: 禁止在各模块中重复定义相同的常量或结构体
- **类型安全**: 使用强类型定义，避免使用 `int` 替代枚举类型
- **新增类型**: 新增的共享类型应添加到 `pkg/types/` 目录下的对应文件

### 4.6 日志规范

- **user-api**: 使用 `logx.Logger`（嵌入 Logic 结构体）
- **game-service**: 使用 `log` 标准库（后续迁移到 `logx`）
- 日志级别：开发 `debug`，生产 `info`
- 关键位置必须记录日志：
  - 玩家登录/登出
  - 房间创建/销毁
  - 游戏状态变更
  - AI 托管触发
  - 异常/错误

### 4.7 错误码体系

| 范围 | 模块 | HTTP 状态码 |
|------|------|------------|
| `0x00xx` | 系统错误 | 500 |
| `0x01xx` | 认证错误 | 401 |
| `0x02xx` | 房间错误 | 400/404 |
| `0x03xx` | 匹配错误 | 503 |
| `0x04xx` | 游戏错误 | 400 |
| `0x05xx` | 重连错误 | 400 |

---

## 五、关键设计决策

### 5.1 房间状态管理

```
内存为主 + 异步 Redis 快照
- 读写: Go map + sync.RWMutex（微秒级）
- 持久化: goroutine 定期保存到 Redis Hash（秒级）
- 重连: Redis 快照 + 内存恢复
```

### 5.2 玩家路由

```
Redis Hash: ddz:player:route:{uid}
  → {instance_id, room_id, conn_id}
- 写入: WS 连接时
- 删除: 断线时
- 续期: 心跳（5 分钟 TTL）
```

### 5.3 匹配系统

```
Redis ZSet: ddz:match:queue:*
- 扫描间隔: 2 秒
- 快速匹配：先到先得
- 段位匹配：ELO ±100
- 超时 30 秒 → 填充 AI 机器人
```

### 5.4 AI 托管

| 触发条件 | 行为 |
|----------|------|
| 15s 不出牌 | 自动 AI 出牌 |
| 主动点击托管 | 立即开启 |
| 断线超时 | AI 接管 |
| 匹配补齐 | 全程 AI |

### 5.5 密码安全

- **开发环境**: SHA256（仅用于本地开发）
- **生产环境**: **必须使用 bcrypt**（项目预留，待切换）
- 密码哈希函数在 `svc/servicecontext.go` 中

### 5.6 集群模式

- `Cluster.Enabled = false` → 退化为纯单机模式
- 跨实例通信通过 Redis Pub/Sub
- 实例注册 TTL 30 秒，每 10 秒续期
- 优雅关闭流程：SIGTERM → draining → 等待房间结束 → 注销

---

## 六、版本控制规范

### 6.1 Commit Message 格式

```
<type>(<scope>): <subject>

<body>
```

**类型 (type)**:
- `feat`: 新功能
- `fix`: 修复
- `refactor`: 重构
- `test`: 测试
- `docs`: 文档
- `style`: 格式化
- `chore`: 构建/CI
- `perf`: 性能优化

**范围 (scope)**:
- `user-api`
- `game`
- `cardutil`
- `types`
- `cluster`
- `match`
- `room`
- `ai`
- `docs`
- `deploy`

**示例**:
```
feat(match): 实现段位匹配 ELO 算法

- 支持地主/农民分别计分
- 引入分差系数
- 连输保护机制
```

### 6.2 分支策略

```
main        ← 生产就绪代码
dev         ← 最新开发代码
feat/*      ← 功能分支（从 dev 拉取）
fix/*       ← 修复分支
docs/*      ← 文档分支（可绕过 dev 直接合入 main）
```

- 禁止直接推送 `main` 和 `dev`
- 合并前必须通过 lint + test
- 使用 squash merge 保持历史整洁

---

## 七、代码评审检查清单

### 7.1 通用检查

- [ ] 编译通过（`go build ./...`）
- [ ] 测试通过（`go test ./... -v -race`）
- [ ] lint 无报错（`make lint`）
- [ ] 无 `fmt.Println` / `log.Printf`（使用 `logx`）
- [ ] 无魔法数字（定义为常量）
- [ ] 无 `context.Background()` 在非入口函数中
- [ ] 无裸露的 goroutine（使用 errgroup / waitgroup）
- [ ] 锁使用正确（无嵌套锁、无 unlock 遗漏）
- [ ] 错误已处理（不忽略 error 返回值）

### 7.2 user-api 专项

- [ ] `.api` 文件已同步更新
- [ ] `routes.go` 路由分离正确（公开 vs JWT）
- [ ] `config.go` 嵌入了 `rest.RestConf`
- [ ] `servicecontext.go` 返回 `*ServiceContext`（无 error）
- [ ] logic 中不是 `// todo:` 占位

### 7.3 game-service 专项

- [ ] 房间状态机状态转换正确
- [ ] 并发安全（RWMutex 保护共享状态）
- [ ] WebSocket 连接正确注册/注销
- [ ] AI 托管不会导致死循环
- [ ] 跨实例消息不会重复处理
- [ ] 正确引用 `pkg/types` 中的常量和结构体

### 7.4 pkg/types 专项

- [ ] 无重复定义（检查是否已有相同类型）
- [ ] 类型命名清晰、语义明确
- [ ] 常量值合理、无冲突
- [ ] 结构体字段命名规范

### 7.5 安全审查

- [ ] 用户输入已校验（SQL 注入、XSS）
- [ ] JWT Secret 未硬编码在生产配置中
- [ ] 密码未明文存储/日志
- [ ] 敏感信息（密码、Token）不在 URL 参数中传递

---

## 八、测试规范

### 8.1 测试策略

| 层级 | 工具 | 覆盖目标 |
|------|------|---------|
| 单元测试 | `go test` | `pkg/cardutil`（30+ 用例） |
| 集成测试 | `go test` + redis | 数据库操作 |
| 端到端 | 手动/Postman | WS 消息流程 |

### 8.2 测试命名

```go
func TestAnalyzePattern(t *testing.T)           // 函数名测试
func TestAnalyzePattern_Single(t *testing.T)     // 子用例
func TestDealCards_ThreePlayers(t *testing.T)    // 场景描述
```

### 8.3 要求

- 公共 API 必须测试
- 牌型判定覆盖率 > 90%（当前 30 个测试用例）
- 并发代码必须通过 `-race` 检测

---

## 九、部署运维

### 9.1 环境配置

```bash
# 单机开发
game-service -f etc/game-local.yaml

# 集群模式 - 实例 1
game-service -f etc/game-prod.yaml --cluster.enabled=true --cluster.host=10.0.1.10

# 一键启动
docker-compose up -d
```

### 9.2 Docker

```bash
docker build -t ddz-game:latest .
```

- Dockerfile 使用 `golang:1.22-alpine`（**需要更新到 1.25-alpine**）
- 多阶段构建（builder → alpine）
- 仅构建 game-service（user-api 尚未容器化）

### 9.3 关键指标

| 指标 | 说明 |
|------|------|
| 单实例并发 | 15,000+ WS 连接 / 5,000+ 房间 |
| 内存占用 | ~1GB/实例 |
| 消息延迟 | <1ms（同实例），<5ms（跨实例） |
| 牌型判定 | <10μs/次 |

### 9.4 健康检查

- `/health` — 存活检查（返回 200 OK）
- `/ready` — 就绪检查（返回 200 READY）
- `/metrics` — Prometheus 指标（预留）

### 9.5 优雅关闭流程

```
SIGTERM → 停止接收新连接 → 状态改为 draining
  → 等待现有房间结束 → 注销玩家路由
  → 注销实例 → 关闭 Redis 连接池 → 退出
```

---

## 十、常见陷阱（必读）

| # | 陷阱 | 说明 |
|---|------|------|
| 1 | **routes.go 被覆盖** | 重新生成 user-api 后，需手动恢复公开/JWT 路由分离 |
| 2 | **go-zero 配置标签** | `json:",default=..."` 和 `json:",optional"` LSP 会报 "unknown JSON option" — 忽略即可 |
| 3 | **ServiceContext 签名** | goctl 期望 `NewServiceContext(c) *ServiceContext`，不能返回 `(*ServiceContext, error)` |
| 4 | **MySQL DSN 格式** | `user:pass@tcp(host:port)/dbname?parseTime=true&loc=Local` |
| 5 | **game-service 配置** | 使用自定义 `config.LoadConfig()`，不是 go-zero 的 `conf.MustLoad()` |
| 6 | **状态机重复创建** | `NewGameStateMachine()` 每次调用都创建新实例，应在 Room 中持有引用 |
| 7 | **类型重复定义** | 应从 `pkg/types` 引用，不要在各模块中重复定义 |
| 8 | **IsAIControlled 状态** | 进入出牌阶段前必须重置非机器人玩家的 `IsAIControlled` 为 `false` |
| 9 | **Dockerfile Go 版本** | 当前使用 `golang:1.22-alpine`，需更新为 `go.mod` 的 1.25.1 |