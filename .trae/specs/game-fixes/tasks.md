# 任务清单

## [ ] 任务1：检查并修复后端叫地主广播
- **Priority**: P0
- **Depends On**: None
- **Description**: 
  - 检查 `app/game/internal/handler/handler.go` 中玩家叫地主后的广播逻辑
  - 检查 `app/game/internal/room/manager.go` 中机器人叫地主后的广播逻辑
  - 确保每个玩家叫地主/不叫后都广播消息
  - 添加"轮到谁叫地主"的广播消息
- **Acceptance Criteria Addressed**: AC-1, AC-2, AC-3
- **Test Requirements**:
  - `human-judgement`: 测试叫地主阶段是否显示所有玩家的叫地主结果
  - `human-judgement`: 测试是否显示"轮到XX叫地主"的消息
- **Notes**: 需要修改后端广播逻辑

## [ ] 任务2：检查并修复前端叫地主通知处理
- **Priority**: P0
- **Depends On**: Task 1
- **Description**: 
  - 检查 `client-web/js/game.js` 中 CALL_LANDLORD_NOTIFY 的处理逻辑
  - 确保正确处理所有叫地主相关的广播消息
- **Acceptance Criteria Addressed**: AC-1, AC-2, AC-3
- **Test Requirements**:
  - `human-judgement`: 测试前端是否正确显示叫地主广播消息
- **Notes**: 需要修改前端消息处理逻辑

## [ ] 任务3：修复用户被托管问题
- **Priority**: P0
- **Depends On**: Tasks 1, 2
- **Description**: 
  - 在后端确保进入出牌阶段前，所有非机器人玩家的 `IsAIControlled` 状态被正确重置为 `false`
  - 添加详细日志便于排查问题
- **Acceptance Criteria Addressed**: AC-4
- **Test Requirements**:
  - `human-judgement`: 测试机器人作为地主时用户是否能正常出牌
- **Notes**: 需要修改 `app/game/internal/room/manager.go` 文件

## [x] 任务4：重新编译并重启服务
- **Priority**: P1
- **Depends On**: Tasks 1, 2, 3
- **Description**: 
  - 重新编译 game-service
  - 重启所有服务
- **Acceptance Criteria Addressed**: 
  - 服务启动成功
- **Test Requirements**:
  - `programmatic`: 服务能正常启动
- **Notes**: 使用 `go build` 和启动脚本

## [ ] 任务5：测试验证
- **Priority**: P1
- **Depends On**: Task 4
- **Description**: 
  - 测试叫地主广播功能
  - 测试机器人作为地主时的游戏流程
  - 确认用户可以正常出牌
- **Acceptance Criteria Addressed**: AC-1, AC-2, AC-3, AC-4
- **Test Requirements**:
  - `human-judgement`: 测试游戏流程是否正常
- **Notes**: 需要用户参与测试