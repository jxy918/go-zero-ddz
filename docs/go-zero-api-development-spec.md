# go-zero 接口开发规范

> 本文档是 [AGENTS.md](../AGENTS.md) 的补充子文档，详细说明 go-zero API 开发流程。
> 主规范文档请先阅读 AGENTS.md 中的「编码规范」和「user-api 规范」章节。

## 1. 开发流程（必须遵守）

```
编辑 .api 文件
    ↓
goctl api go 生成代码
    ↓
在 logic 中填写业务逻辑
    ↓
手动调整 routes.go（公开/JWT 路由分离）
    ↓
编译验证
```

**禁止跳过任何步骤。** 不允许手写 handler 绕过 goctl。

## 2. .api 文件规范

### 2.1 文件位置

每个 API 服务一个 `.api` 文件，放在服务根目录：
```
app/user/user.api    ← user 服务的唯一 API 定义
```

### 2.2 文件头格式

```go
syntax = "v1"

info (
    title:   "服务名称"
    desc:    "服务描述"
    author:  "作者"
    version: "v1"
)
```

### 2.3 类型定义规范

- 请求类型以 `Req` 结尾：`LoginReq`、`RegisterReq`
- 响应类型以 `Resp` 结尾：`LoginResp`、`UserInfoResp`
- 字段使用 `json` tag，可选字段加 `,optional`：
  ```go
  type LoginReq {
      Username string `json:"username"`
      Password string `json:"password"`
  }
  
  type RegisterReq {
      Username string `json:"username"`
      Password string `json:"password"`
      Nickname string `json:"nickname,optional"`  // 可选字段
  }
  ```

### 2.4 路由定义规范

```go
@server (
    prefix: /user        // 统一路径前缀
    jwt:    Auth         // JWT 鉴权配置（如果服务需要鉴权）
)

service user-api {
    @doc "接口描述"
    @handler handlerName    // 驼峰命名，如 login、register、userInfo
    post /login (LoginReq) returns (LoginResp)
    
    @doc "获取用户信息"
    @handler userInfo
    get /info returns (UserInfoResp)
}
```

**命名规则**：
- `@handler` 使用驼峰命名：`login`、`register`、`userInfo`
- 路径使用短横线或无分隔：`/login`、`/user-info` 或 `/userinfo`
- 保持 `@handler` 名与路径一致

## 3. 代码生成

### 3.1 生成命令

```bash
cd app/user && goctl api go -api user.api -dir . -style gozero
```

**参数说明**：
- `-api user.api`：API 定义文件
- `-dir .`：输出到当前目录
- `-style gozero`：使用 go-zero 命名风格

### 3.2 生成后的文件处理

| 文件 | 是否可编辑 | 说明 |
|------|-----------|------|
| `user.go` | ❌ 禁止 | goctl 生成，入口文件 |
| `internal/handler/*handler.go` | ❌ 禁止 | goctl 生成，请求解析层 |
| `internal/handler/routes.go` | ⚠️ 谨慎 | goctl 生成，但需手动调整路由分离 |
| `internal/logic/*logic.go` | ✅ 必须编辑 | 业务逻辑写在这里 |
| `internal/types/types.go` | ❌ 禁止 | goctl 生成，类型定义 |
| `internal/config/config.go` | ✅ 可编辑 | goctl 跳过已存在文件 |
| `internal/svc/servicecontext.go` | ✅ 可编辑 | goctl 跳过已存在文件 |

### 3.3 路由分离（关键）

goctl 默认将所有路由放在同一个 `AddRoutes` 调用中。如果服务同时有**公开接口**和**需鉴权接口**，必须手动拆分 `routes.go`：

```go
func RegisterHandlers(server *rest.Server, serverCtx *svc.ServiceContext) {
    // 公开路由（无需 JWT）
    server.AddRoutes(
        []rest.Route{
            {
                Method:  http.MethodPost,
                Path:    "/register",
                Handler: registerHandler(serverCtx),
            },
            {
                Method:  http.MethodPost,
                Path:    "/login",
                Handler: loginHandler(serverCtx),
            },
        },
        rest.WithPrefix("/user"),
    )

    // 需要 JWT 鉴权的路由
    server.AddRoutes(
        []rest.Route{
            {
                Method:  http.MethodGet,
                Path:    "/info",
                Handler: userInfoHandler(serverCtx),
            },
        },
        rest.WithJwt(serverCtx.Config.Auth.AccessSecret),
        rest.WithPrefix("/user"),
    )
}
```

**每次重新生成后，必须检查并恢复此分离！**

## 4. Config 配置规范

### 4.1 必须嵌入 rest.RestConf

```go
package config

import "github.com/zeromicro/go-zero/rest"

type Config struct {
    rest.RestConf          // 必须嵌入，go-zero 需要
    Auth  AuthConfig       // JWT 配置
    Redis RedisConfig      // Redis 配置
    MySQL MySQLConfig      // MySQL 配置
}

type AuthConfig struct {
    AccessSecret string `json:",default=ddz-secret-key-2025"`
    AccessExpire int64  `json:",default=86400"`
}
```

### 4.2 go-zero 配置标签

- `json:",default=值"`：设置默认值
- `json:",optional"`：标记为可选字段
- **LSP 会报 "unknown JSON option"，这是误报，忽略即可**

## 5. ServiceContext 规范

### 5.1 函数签名

```go
func NewServiceContext(c config.Config) *ServiceContext {
    // 返回 *ServiceContext，不是 (*ServiceContext, error)
}
```

**goctl 生成的调用代码是 `svc.NewServiceContext(c)`，不接受 error 返回值。**

### 5.2 数据库连接处理

MySQL 不可达时不应阻塞启动，应降级为内存模式：

```go
func NewServiceContext(c config.Config) *ServiceContext {
    var db *sql.DB
    
    if c.MySQL.DSN != "" {
        db, err = sql.Open("mysql", c.MySQL.DSN)
        if err != nil || db.Ping() != nil {
            logx.Errorf("MySQL unavailable: %v", err)
            db = nil  // 降级为内存模式
        }
    }
    
    return &ServiceContext{
        Config: c,
        DB:     db,
    }
}
```

## 6. Logic 业务逻辑规范

### 6.1 标准结构

```go
package logic

import (
    "context"
    
    "go-zero-ddz/app/user/internal/svc"
    "go-zero-ddz/app/user/internal/types"
    
    "github.com/zeromicro/go-zero/core/logx"
)

type LoginLogic struct {
    logx.Logger
    ctx    context.Context
    svcCtx *svc.ServiceContext
}

func NewLoginLogic(ctx context.Context, svcCtx *svc.ServiceContext) *LoginLogic {
    return &LoginLogic{
        Logger: logx.WithContext(ctx),
        ctx:    ctx,
        svcCtx: svcCtx,
    }
}

func (l *LoginLogic) Login(req *types.LoginReq) (resp *types.LoginResp, err error) {
    // 业务逻辑写在这里
    // 错误返回 errors.New("错误信息")
    // 成功返回 resp, nil
}
```

### 6.2 错误处理

- 使用 `errors.New("错误信息")` 返回业务错误
- 使用 `l.Errorf()` 记录详细错误日志
- 不要返回内部错误给客户端，转换为友好提示

```go
func (l *LoginLogic) Login(req *types.LoginReq) (resp *types.LoginResp, err error) {
    user, err := l.svcCtx.GetUserByUsername(req.Username)
    if err != nil {
        if errors.Is(err, sql.ErrNoRows) {
            return nil, errors.New("用户不存在")
        }
        l.Errorf("查询用户失败: %v", err)
        return nil, errors.New("登录失败")
    }
    // ...
}
```

### 6.3 JWT Token 生成

```go
func (l *LoginLogic) generateToken(uid, username string) (string, error) {
    now := time.Now().Unix()
    claims := jwt.MapClaims{
        "uid":      uid,
        "username": username,
        "iat":      now,
        "exp":      now + l.svcCtx.Config.Auth.AccessExpire,
    }
    
    token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
    return token.SignedString([]byte(l.svcCtx.Config.Auth.AccessSecret))
}
```

## 7. Handler 规范（goctl 生成，禁止修改）

Handler 由 goctl 自动生成，标准结构如下：

```go
func loginHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        var req types.LoginReq
        if err := httpx.Parse(r, &req); err != nil {
            httpx.ErrorCtx(r.Context(), w, err)
            return
        }

        l := logic.NewLoginLogic(r.Context(), svcCtx)
        resp, err := l.Login(&req)
        if err != nil {
            httpx.ErrorCtx(r.Context(), w, err)
        } else {
            httpx.OkJsonCtx(r.Context(), w, resp)
        }
    }
}
```

**禁止修改 handler 文件！** 所有业务逻辑放在 logic 层。

## 8. 新增 API 服务流程

如果要新增一个 API 服务（如 `app/admin/`）：

1. 创建目录结构：
   ```
   app/admin/
   ├── admin.api
   ├── etc/admin-api.yaml
   └── internal/
       ├── config/
       ├── handler/
       ├── logic/
       ├── svc/
       └── types/
   ```

2. 编写 `admin.api` 文件

3. 运行生成命令：
   ```bash
   cd app/admin && goctl api go -api admin.api -dir . -style gozero
   ```

4. 编辑 `internal/config/config.go`，嵌入 `rest.RestConf`

5. 编辑 `internal/svc/servicecontext.go`，实现 `NewServiceContext`

6. 编辑 `internal/logic/*logic.go`，填写业务逻辑

7. 如果需要路由分离，手动调整 `internal/handler/routes.go`

8. 编译验证：`go build ./app/admin/`

## 9. 检查清单

每次修改 API 后，必须检查：

- [ ] `.api` 文件语法正确（`goctl api go` 不报错）
- [ ] 代码已重新生成（`goctl api go -api xxx.api -dir . -style gozero`）
- [ ] `routes.go` 路由分离正确（公开 vs JWT）
- [ ] `config.go` 嵌入了 `rest.RestConf`
- [ ] `servicecontext.go` 返回 `*ServiceContext`（无 error）
- [ ] `logic` 中填写了业务逻辑（不是 `// todo` 注释）
- [ ] 编译通过（`go build ./app/xxx/`）
