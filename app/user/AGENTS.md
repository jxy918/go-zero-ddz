# app/user/ — user-api (goctl 生成)

> go-zero HTTP/JSON 用户服务。**严格遵循 goctl 工作流**，禁止随意手写。

## OVERVIEW

提供注册、登录、用户信息查询三个端点，端口 8888。入口 `user.go` 由 goctl 生成。

## WHERE TO LOOK

| 任务 | 位置 |
|------|------|
| 添加/修改 API 端点 | `user.api`（**唯一真理源**） |
| 重生成代码 | `make gen-user-api`（在 `app/user/` 目录运行 goctl） |
| 业务逻辑实现 | `internal/logic/` ✅ 开发者编辑 |
| HTTP 处理器 | `internal/handler/` ❌ goctl 生成，**禁止编辑** |
| 请求/响应类型 | `internal/types/` ❌ goctl 生成，**禁止编辑** |
| 配置 | `internal/config/config.go` ⚠️ 手动维护（嵌入 `rest.RestConf`） |
| 服务上下文 | `internal/svc/servicecontext.go` ⚠️ 手动维护 |
| 内存模式用户 | `internal/svc/servicecontext.go` 的 `GetUserByUsername/GetUserByUID`（DB=nil 时） |

## 端点

| 方法 | 路径 | 鉴权 | 描述 |
|------|------|------|------|
| POST | `/user/register` | 公开 | 注册 |
| POST | `/user/login` | 公开 | 登录 |
| GET | `/user/info` | JWT | 当前用户信息 |

## CONVENTIONS

- **ServiceContext 签名**: `func NewServiceContext(c config.Config) *ServiceContext`（**不能**返回 error）
- **Logic 结构体**: 嵌入 `logx.Logger`，错误用 `errors.New("友好提示")`
- **路由分离**: `routes.go` 需手动分离公开路由（register/login）和 JWT 路由（info）
- **配置标签**: `json:",default=..."` 和 `json:",optional"` 是 go-zero 特有语法，**LSP 报 "unknown JSON option" 是误报，忽略**
- **字段命名**: JSON 字段用 **snake_case**（与前端约定）

## 内存模式

DSN 为空时降级为内存模式（`svc/servicecontext.go`）：
- 硬编码 `test/123` 可登录
- 数据不持久化，日志标注 `[Memory Mode]`
- 用于本地开发 / CI 烟测

## ANTI-PATTERNS

| # | 陷阱 | 说明 |
|---|------|------|
| 1 | **routes.go 被覆盖** | `make gen-user-api` 后需手动恢复公开/JWT 路由分离（见 4.3 节） |
| 2 | **修改 types.go 而不同步 .api** | 会被下一次生成覆盖 |
| 3 | **修改 handler 函数体** | 重新生成会丢失改动 |
| 4 | **Logic 中留 `// todo:`** | PR 评审会卡住 |
| 5 | **新增字段忘记数据库迁移** | `internal/model/` 用 GORM AutoMigrate，但 user-api 当前**用 sql.DB 直接 query**，无 ORM 迁移，需手动跑 `sql/init.sql` |

## 密码

`internal/svc/servicecontext.go` 的 `HashPassword` 用 **SHA256**（仅开发）。**生产必须切换为 bcrypt**（预留接口）。
