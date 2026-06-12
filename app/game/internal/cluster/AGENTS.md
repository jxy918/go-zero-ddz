# app/game/internal/cluster/ — 集群组件

> 多实例集群的注册、路由、消息总线。**`Cluster.Enabled=false` 时整包不加载**。

## OVERVIEW

`Cluster.Enabled=true` 时 `service_context.go` 初始化三件套：

| 组件 | 文件 | 职责 |
|------|------|------|
| `Registry` | `registry.go` | 实例注册 + 30s TTL 心跳 |
| `Router` | `router.go` | 玩家 UID → (instance_id, room_id, conn_id) 路由 |
| `MessageBus` | `message_bus.go` | Redis Pub/Sub 跨实例消息 |

## Redis Key 规范

| Key | 类型 | TTL | 用途 |
|-----|------|-----|------|
| `ddz:instance:{id}` | String (JSON) | 30s | 实例元信息（host/port/status/conns/rooms） |
| `ddz:player:route:{uid}` | String (JSON) | 5min | 玩家路由（被踢/断线时手动删） |
| `ddz:room:{id}` (via pub/sub) | Pub/Sub | - | 房间事件（`ddz:room:{id}:play` 等） |

## MessageBus 消息类型

```go
const (
    MsgTypePlay      MessageType = "play"      // 出牌
    MsgTypeControl   MessageType = "control"   // 托管控制
    MsgTypeBroadcast MessageType = "broadcast" // 广播
    MsgTypeReconnect MessageType = "reconnect" // 重连
)
```

## CONVENTIONS

- **Instance ID**：`{hostname}-{pid}`（`generateInstanceID`），可配置覆盖
- **心跳**：`Registry` 启动后自动续期，TTL 30s
- **路由粒度**：玩家级（不是房间级），便于重连定位
- **Channel 命名**：`ddz:room:{roomID}`，订阅通过 `SubscribeRoom(ctx, roomID)`
- **去重**：跨实例消息自带 `Timestamp`，消费方应做幂等（同一 instance 内不重复处理）

## ANTI-PATTERNS

- ❌ **不要**在单机模式下强制 `Cluster.Enabled=true`（会引入 Pub/Sub 延迟）
- ❌ **不要**绕过 `Router` 直接写 Redis（key/TTL 规范必须统一）
- ❌ **不要**在 MessageBus 回调里做重活（`handlers` 持有 `MessageHandler` 函数，调度要快）
- ❌ **不要**假设实例崩溃后立即清理（依赖 TTL 自动过期，30s 内可能有脏数据）

## 优雅关闭

```
SIGTERM → service_context.Stop() →
  → Registry.Unregister()   // 主动从 Redis 删
  → MessageBus.Close()      // 关闭 Pub/Sub
  → RoomManager.Stop()      // 等待现有房间结束
```

## 部署

docker-compose.yml 启 2 实例（game-1:8081, game-2:8082）+ nginx 负载均衡。
开发多实例：`make run-cluster-1` + `make run-cluster-2`。
