# go-zero-ddz 斗地主联机游戏


一款基于 go-zero 后端 + Web 前端的经典三人斗地主在线卡牌游戏。

## 项目概述

- **游戏类型**: 经典三人斗地主（支持地主/农民角色、叫地主、炸弹、春天规则）
- **后端技术**: go-zero (Go 微服务框架)、WebSocket 长连接、JSON 协议
- **前端技术**: Web (HTML5/CSS/JavaScript，浏览器运行)
- **数据存储**: Redis (房间状态、用户会话、匹配队列)、MySQL (用户数据、对局记录)

## 系统架构

```
                        ┌─────────────────────────────┐
                        │         Nginx / HAProxy       │
                        │     (WebSocket 负载均衡)      │
                        └──────────┬──────────────────┘
                                   │
              ┌────────────────────┼────────────────────┐
              ▼                    ▼                    ▼
        ┌──────────┐        ┌──────────┐        ┌──────────┐
        │ Game-01  │        │ Game-02  │        │ Game-03  │
        │ :8080    │        │ :8080    │        │ :8080    │
        └────┬─────┘        └────┬─────┘        └────┬─────┘
             │                   │                   │
             └───────────────────┼───────────────────┘
                                 │
                    ┌────────────┴────────────┐
                    │       Redis 集群         │
                    │  • 玩家路由表            │
                    │  • 房间快照              │
                    │  • Pub/Sub 消息总线      │
                    │  • 匹配队列              │
                    └──────────────────────────┘
```

### 服务说明

| 服务 | 端口 | 协议 | 说明 |
|------|------|------|------|
| `user-api` | 8888 | HTTP/JSON | 用户服务（登录、注册、JWT 认证） |
| `game-service` | 8080 | WebSocket | 游戏服务（房间、匹配、出牌、AI 托管） |

## 项目结构

```
go-zero-ddz/
├── app/
│   ├── user/                       # 用户 API 服务
│   │   ├── etc/user-api.yaml       # 配置文件
│   │   ├── user.go                 # 入口文件
│   │   ├── user.api                # API 定义
│   │   └── internal/
│   │       ├── config/             # 配置结构体
│   │       ├── handler/            # HTTP 处理器（goctl 生成）
│   │       ├── logic/              # 业务逻辑
│   │       ├── svc/                # 服务上下文
│   │       └── types/              # 类型定义
│   └── game/                       # 游戏 WebSocket 服务
│       ├── cmd/server/main.go      # 入口文件
│       ├── etc/
│       │   ├── game-local.yaml     # 本地配置
│       │   └── game-prod.yaml     # 生产配置
│       └── internal/
│           ├── ai/                 # AI 出牌引擎
│           ├── cluster/            # 集群组件
│           ├── config/             # 配置定义
│           ├── game/               # 核心游戏逻辑
│           │   ├── play.go         # 出牌逻辑
│           │   ├── call.go         # 叫地主逻辑
│           │   └── settlement.go   # 结算逻辑
│           ├── handler/            # WebSocket 处理器
│           ├── match/              # 匹配系统
│           ├── room/               # 房间管理
│           ├── svc/                # 服务上下文
│           └── websocket/          # WebSocket 连接管理
├── pkg/
│   ├── cardutil/                   # 牌型判定库
│   └── types/                      # 共享类型定义
├── proto/                          # Protobuf 定义
├── client-web/                     # Web 前端
├── images/                         # 项目截图
├── sql/                            # 数据库初始化脚本
├── docker-compose.yml              # Docker 编排
├── Dockerfile                      # 游戏服务镜像
├── nginx/                          # Nginx 配置
├── k8s/                            # Kubernetes 配置
├── Makefile                        # 构建脚本
└── AGENTS.md                       # 开发规范手册
```

## 游戏流程截图

### 1. 登录界面

![登录](images/login.png)

玩家输入用户名和密码进行登录，支持注册新账号。

### 2. 准备界面

![准备](images/ready.png)

登录成功后进入准备界面，点击"开始匹配"进入匹配队列。

### 3. 等待匹配

![等待](images/room.png)

匹配成功后进入房间，等待其他玩家准备或 AI 机器人加入。

### 4. 叫地主阶段

![叫地主](images/call.png)

发牌后进入叫地主阶段，玩家可以选择"不叫"或叫 1/2/3 分，决定谁是地主并获得底牌。

### 5. 出牌阶段

![出牌](images/play.png)

确认地主后进入出牌阶段，玩家轮流出牌，支持单张、对子、三条、顺子、连对、飞机、炸弹、王炸等多种牌型。

### 6. 游戏结算

![结算](images/win.png)

有玩家出完手牌后游戏结束，根据胜负和倍数计算积分，支持 ELO 天梯系统。

## 核心功能

### 支持牌型

斗地主标准牌型全支持：

| 牌型 | 说明 | 示例 |
|------|------|------|
| 单张 | 任意单张牌 | 3 |
| 对子 | 两张相同点数的牌 | 33 |
| 三条 | 三张相同点数的牌 | 333 |
| 三带一 | 三张 + 单张 | 333+4 |
| 三带二 | 三张 + 对子 | 333+44 |
| 顺子 | 5 张及以上连续单张（不含 2） | 34567 |
| 连对 | 3 对及以上连续对子 | 334455 |
| 飞机 | 2 个及以上连续三条 | 333444+57 |
| 四带二 | 四张 + 两张单牌 | 3333+45 |
| 炸弹 | 四张相同点数的牌 | 3333 |
| 王炸 | 大王 + 小王 | 🃏🃏 |

### 匹配系统

- **快速匹配**: 先到先得，超时 30 秒后填充 AI 机器人
- **排位匹配**: 基于 ELO 分数，优先匹配同段位玩家
- **ELO 系统**: 地主/农民分开计分，考虑倍数和对手评分差异

#### 段位等级

| 段位 | ELO 范围 | 说明 |
|------|---------|------|
| 青铜 I-III | 0-1199 | 新手保护 |
| 白银 I-III | 1200-1399 | - |
| 黄金 I-III | 1400-1599 | - |
| 铂金 I-III | 1600-1799 | - |
| 钻石 I-III | 1800-1999 | - |
| 大师 | 2000+ | 顶尖玩家 |

### AI 托管

- **触发条件**: 15 秒不出牌自动托管、手动托管、断线超时
- **难度等级**: 简单（随机出牌）、普通（最小牌型）、困难（记牌+推理）
- **策略**: 手牌强度评估、局势分析、炸弹策略、队友识别

## 快速开始

### 环境要求

- Go 1.25+
- Redis 6+
- MySQL 8+（可选，支持内存模式）
- Node.js（前端开发用）

### 本地开发

```bash
# 1. 克隆仓库
git clone <仓库地址>
cd go-zero-ddz

# 2. 安装依赖
go mod tidy

# 3. 启动 Redis 和 MySQL（Docker）
make docker-up

# 4. 启动用户 API 服务
go run app/user/user.go -f app/user/etc/user-api.yaml

# 5. 启动游戏 WebSocket 服务
go run app/game/cmd/server/main.go -f app/game/etc/game-local.yaml

# 6. 打开浏览器访问
# http://localhost:8080/
```

### API 接口

| 方法 | 路径 | 说明 | 认证 |
|------|------|------|------|
| POST | `/user/register` | 注册新用户 | 无 |
| POST | `/user/login` | 用户登录 | 无 |
| GET | `/user/info` | 获取用户信息 | JWT |

#### 注册示例

```bash
curl -X POST http://localhost:8888/user/register \
  -H "Content-Type: application/json" \
  -d '{"username":"test","password":"123","nickname":"TestPlayer"}'
```

#### 登录示例

```bash
curl -X POST http://localhost:8888/user/login \
  -H "Content-Type: application/json" \
  -d '{"username":"test","password":"123"}'
```

### WebSocket 协议

消息帧格式: `[Length 4B][MsgID 2B][JSON Payload]`

#### 消息 ID 范围

| 范围 | 模块 | 说明 |
|------|------|------|
| `0x00xx` | 系统 | 心跳、错误 |
| `0x01xx` | 认证 | 登录、登出 |
| `0x02xx` | 房间 | 创建、加入、准备 |
| `0x03xx` | 匹配 | 开始匹配、匹配成功 |
| `0x04xx` | 游戏 | 发牌、叫地主、出牌、结算 |
| `0x05xx` | 重连 | 断线重连 |

## 集群部署

### Docker Compose

```bash
# 启动所有服务
make docker-up

# 停止所有服务
make docker-down
```

### Kubernetes

```bash
kubectl apply -f k8s/
```

### 扩展策略

| 组件 | 扩展方式 | 说明 |
|------|---------|------|
| 游戏服务 | HPA | 根据连接数自动扩展 |
| Redis | Redis Cluster | 最多 1000 节点 |
| MySQL | 主从复制 | 主库写入，从库读取 |
| Nginx | 负载均衡 | 最小连接数策略 |

## 环境变量

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `REDIS_HOST` | `localhost:6379` | Redis 地址 |
| `REDIS_PASS` | `` | Redis 密码 |
| `MYSQL_DSN` | `` | MySQL DSN（空=内存模式） |
| `CLUSTER_ENABLED` | `false` | 启用集群模式 |
| `INSTANCE_ID` | 自动生成 | 实例 ID |
| `GAME_PORT` | `8080` | 游戏服务端口 |
| `USER_PORT` | `8888` | 用户服务端口 |
| `JWT_SECRET` | `ddz-secret-key-2025` | JWT 密钥 |

## 性能指标

- **单实例容量**: 15,000+ 并发 WebSocket 连接（5,000+ 房间）
- **内存占用**: ~1GB/实例
- **消息延迟**: <1ms（同实例），<5ms（跨实例）
- **牌型判定**: <10μs/次

## 测试

```bash
# 运行所有测试
go test ./... -v

# 运行牌型库测试（带覆盖率）
go test ./pkg/cardutil/... -v -cover

# 竞态检测
go test -race ./...
```

## 开发路线图

| 阶段 | 状态 | 说明 |
|------|------|------|
| P0 | ✅ 完成 | 协议定义 + 消息帧 |
| P1 | ✅ 完成 | 牌型库 + 测试用例 |
| P2 | ✅ 完成 | 用户 API（登录/注册/JWT） |
| P3 | ✅ 完成 | WebSocket 网关 |
| P4 | ✅ 完成 | 房间管理 + 游戏状态机 |
| P5 | ✅ 完成 | 完整游戏流程（发牌→叫地主→出牌→结算） |
| P6 | ✅ 完成 | 匹配系统 + AI 托管 |
| P7 | ✅ 完成 | Web 前端 |
| P8 | ✅ 完成 | 数据库集成 |

## 许可证

MIT 许可证

## 贡献指南

欢迎贡献代码！请遵循 `AGENTS.md` 中的开发规范。

## 联系方式

问题和建议请在仓库中提交 Issue。
