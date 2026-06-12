# app/ — 双服务根目录

## 概览

两个独立 Go 服务，分别走不同协议、不同的代码生成路径：

| 服务 | 路径 | 端口 | 协议 | 代码生成 | 描述 |
|------|------|------|------|----------|------|
| **user-api** | `app/user/` | 8888 | HTTP/JSON | `goctl api go` | 注册/登录/JWT |
| **game-service** | `app/game/` | 8080 | WebSocket | 全部手写 | 房间/匹配/出牌/AI |

**绝对禁止**混用代码生成风格：user-api 走 goctl 工作流，game-service 全手写。

## WHERE TO LOOK

| 任务 | 路径 |
|------|------|
| 改 API 端点 | `app/user/user.api` → `make gen-user-api` → 编辑 `logic/` |
| 改游戏规则 | `app/game/internal/game/`、`app/game/internal/room/state.go` |
| 改 AI 难度/策略 | `app/game/internal/ai/` |
| 添加新错误码 | `app/game/internal/errors/errors.go`（user-api 走 go-zero 错误码） |
| 添加新消息 ID | `pkg/types/message.go`（唯一真理源） |
| 改配置结构 | `app/user/internal/config/config.go`（嵌入 `rest.RestConf`）<br>`app/game/internal/config/config.go`（自定义 `LoadConfig`） |
| 容器化 | `Dockerfile` **仅打包 game-service**（user-api 未容器化） |

## ANTI-PATTERNS

- ❌ **不要**在 user-api 中手写 handler（必须由 goctl 生成）
- ❌ **不要**在 game-service 中使用 go-zero 的 `conf.MustLoad`（用自定义 `LoadConfig`）
- ❌ **不要**让两个服务直接互相 import（通过 HTTP+WS 通信，user-api 不依赖 game-service）
- ❌ **不要**在 `app/` 下放共享代码（共享代码放 `pkg/`）
- ❌ **不要**改 `app/user/internal/types/types.go` 而不更新 `user.api`
