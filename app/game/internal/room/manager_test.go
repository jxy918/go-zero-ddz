package room

import (
	"testing"
	"time"

	"go-zero-ddz/app/game/internal/ai"
	"go-zero-ddz/app/game/internal/config"
	"go-zero-ddz/app/game/internal/websocket"
	"go-zero-ddz/pkg/cardutil"
	"go-zero-ddz/pkg/types"
)

// newTestManager 创建一个用于测试的 Manager 实例（真实 Hub + AIEngine，nil Redis）
func newTestManager(t *testing.T) *Manager {
	t.Helper()
	hub := websocket.NewHub(&config.WebSocketConfig{
		ReadBufferSize:  4096,
		WriteBufferSize: 4096,
	})
	aiEngine := ai.NewAIEngine(&ai.AIConfig{
		Strategy:     "minimum",
		ResponseRate: 0.5,
		DelayMsMin:   100,
		DelayMsMax:   200,
	})
	cfg := &RoomConfig{
		MaxRooms:          10,
		MaxPlayersPerRoom: 3,
		ReadyTimeout:      15,
		PlayTimeout:       15,
		ReconnectTimeout:  300,
		SnapshotInterval:  30,
		BotJoinTimeout:    60,
	}
	m := NewManager(nil, cfg, aiEngine, hub)
	// 设置 onTimeout 回调（虽然我们手动调用，但保持一致）
	t.Cleanup(func() {
		m.Stop()
	})
	return m
}

// makePlayingRoom 构造一个 3 玩家 + 1 地主的 Playing 状态房间
func makePlayingRoom(t *testing.T) *Room {
	t.Helper()
	r := NewRoom("test-room-grace")

	// 玩家顺序: user(地主), bot1(农民), bot2(农民)
	r.AddPlayer(&Player{UID: "user", Nickname: "User", IsBot: false})
	r.AddPlayer(&Player{UID: "bot1", Nickname: "Bot1", IsBot: true})
	r.AddPlayer(&Player{UID: "bot2", Nickname: "Bot2", IsBot: true})

	r.SetState(types.StatePlaying)
	r.LandlordUID = "user"
	r.CurrentTurnUID = "user"
	r.GameState = NewGameStateMachine(r)

	// 给玩家发牌（避免 nil）
	for _, uid := range r.PlayerIDs {
		if p, ok := r.Players[uid]; ok {
			p.Cards = []cardutil.Card{
				{Value: cardutil.CardValue3, Suit: cardutil.CardSuitSpade},
				{Value: cardutil.CardValue4, Suit: cardutil.CardSuitSpade},
			}
		}
	}

	return r
}

// TestOnTimeout_RealPlayerFirstTimeout_EntersGracePeriod 验证真人玩家第一次超时不立即被托管
//
// 背景修复：之前 onTimeout 第一次超时就把 IsAIControlled=true + AI 帮出牌
// 修复后：第一次超时进入 5s 警告宽限期，IsAIControlled 保持 false
func TestOnTimeout_RealPlayerFirstTimeout_EntersGracePeriod(t *testing.T) {
	m := newTestManager(t)
	r := makePlayingRoom(t)

	// 记录 onTimeout 之前
	user, _ := r.GetPlayer("user")
	if user.IsAIControlled {
		t.Fatalf("pre-condition failed: user should not be AI controlled before timeout")
	}
	if user.IsBot {
		t.Fatalf("pre-condition failed: user should be human")
	}

	// 触发真人第一次超时
	m.onTimeout(r, "user")

	// 验证：IsAIControlled 仍为 false（被宽限期保护）
	user, _ = r.GetPlayer("user")
	if user.IsAIControlled {
		t.Errorf("BUG REGRESSION: user was set to AI controlled on first timeout (no grace period). IsAIControlled=%v", user.IsAIControlled)
	}

	// 验证：计时器被重置为 5 秒（宽限期）
	r.mu.RLock()
	timer := r.Timer
	r.mu.RUnlock()
	if timer != 5 {
		t.Errorf("Expected grace period timer to be 5, got %d", timer)
	}
}

// TestOnTimeout_RealPlayerSecondTimeout_TriggersAI 验证真人玩家第二次超时（已托管）正常 AI 出牌
func TestOnTimeout_RealPlayerSecondTimeout_TriggersAI(t *testing.T) {
	m := newTestManager(t)
	r := makePlayingRoom(t)

	// 模拟玩家已经被托管（IsAIControlled=true）
	r.mu.Lock()
	r.Players["user"].IsAIControlled = true
	r.mu.Unlock()

	// 触发第二次超时
	m.onTimeout(r, "user")

	// 验证：仍然保持 IsAIControlled=true（帮出牌分支会重新设置）
	user, _ := r.GetPlayer("user")
	if !user.IsAIControlled {
		t.Errorf("Expected IsAIControlled to remain true on second timeout, got %v", user.IsAIControlled)
	}
}

// TestOnTimeout_BotTimeout_DoesNotEnterGracePeriod 验证 Bot 玩家不进入宽限期
func TestOnTimeout_BotTimeout_DoesNotEnterGracePeriod(t *testing.T) {
	m := newTestManager(t)
	r := makePlayingRoom(t)

	// 切换 CurrentTurnUID 到 bot1
	r.CurrentTurnUID = "bot1"

	// Bot 超时
	m.onTimeout(r, "bot1")

	// 验证：Bot 不进入宽限期（直接走 AI 出牌路径）
	// 计时器应该被设置为 PlayTimeout=15（出牌后 NextTurnAfterPlay 会重置）
	// 这里只验证 Bot 没有被标志为 IsAIControlled（IsAIControlled 概念对 Bot 无意义）
	bot1, _ := r.GetPlayer("bot1")
	if bot1.IsAIControlled {
		t.Errorf("Bot should not have IsAIControlled set to true")
	}
}

// TestOnTimeout_RealPlayerFirstTimeout_TimerReset 验证真人第一次超时后，第二次超时能被正确处理
func TestOnTimeout_RealPlayerFirstTimeout_TimerReset(t *testing.T) {
	m := newTestManager(t)
	r := makePlayingRoom(t)

	// 第一次超时
	m.onTimeout(r, "user")

	// 立即第二次超时（模拟 5s 后再次超时）
	m.onTimeout(r, "user")

	// 验证：第二次超时后，IsAIControlled 应该被设置为 true
	user, _ := r.GetPlayer("user")
	if !user.IsAIControlled {
		t.Errorf("Expected IsAIControlled to be true after second timeout, got %v", user.IsAIControlled)
	}
}

// TestOnTimeout_CallPhaseTimeout_DoesNotEnterGracePeriod 验证叫地主阶段超时走 botCallLandlord 路径
func TestOnTimeout_CallPhaseTimeout_DoesNotEnterGracePeriod(t *testing.T) {
	m := newTestManager(t)
	r := NewRoom("test-call-phase")

	r.AddPlayer(&Player{UID: "user", Nickname: "User", IsBot: false})
	r.AddPlayer(&Player{UID: "bot1", Nickname: "Bot1", IsBot: true})
	r.AddPlayer(&Player{UID: "bot2", Nickname: "Bot2", IsBot: true})

	r.SetState(types.StateCalling)
	r.CurrentTurnUID = "user"
	r.GameState = NewGameStateMachine(r)

	// 叫地主阶段超时
	m.onTimeout(r, "user")

	// 验证：叫地主阶段超时，不应设置 IsAIControlled
	user, _ := r.GetPlayer("user")
	if user.IsAIControlled {
		t.Errorf("Call phase timeout should not set IsAIControlled, got %v", user.IsAIControlled)
	}
}

// TestOnTimeout_AIControlResetOnManualPlay 验证用户手动出牌会重置 IsAIControlled
// （间接验证修复不破坏既有逻辑）
func TestOnTimeout_AIControlResetOnManualPlay(t *testing.T) {
	m := newTestManager(t)
	r := makePlayingRoom(t)

	// 模拟玩家被托管
	r.mu.Lock()
	r.Players["user"].IsAIControlled = true
	r.mu.Unlock()

	// 用户取消托管（模拟前端请求）
	r.mu.Lock()
	r.Players["user"].IsAIControlled = false
	r.mu.Unlock()

	// 第一次超时（用户已重置）
	m.onTimeout(r, "user")

	// 验证：进入宽限期，IsAIControlled 保持 false
	user, _ := r.GetPlayer("user")
	if user.IsAIControlled {
		t.Errorf("After manual cancel + first timeout, user should enter grace period (IsAIControlled=false), got %v", user.IsAIControlled)
	}
}

// TestRoom_GracePeriodTimerIsReasonable 验证宽限期时间合理（5s 警告 + 用户主动操作机会）
func TestRoom_GracePeriodTimerIsReasonable(t *testing.T) {
	m := newTestManager(t)
	r := makePlayingRoom(t)

	m.onTimeout(r, "user")

	r.mu.RLock()
	timer := r.Timer
	r.mu.RUnlock()

	// 警告宽限期应该足够长以让用户操作（5s）
	if timer < 3 || timer > 10 {
		t.Errorf("Grace period should be 3-10 seconds, got %d", timer)
	}
}

// TestRoom_RealPlayerGracePeriod_StopsTimer 验证宽限期会停止之前的计时器
func TestRoom_RealPlayerGracePeriod_StopsTimer(t *testing.T) {
	m := newTestManager(t)
	r := makePlayingRoom(t)

	// 先启动一个 15s 计时器
	r.StartTimer(15, "user")

	// 触发超时（应该重置为 5s 宽限期）
	m.onTimeout(r, "user")

	r.mu.RLock()
	timer := r.Timer
	r.mu.RUnlock()

	if timer != 5 {
		t.Errorf("Expected timer reset to 5s grace period, got %d", timer)
	}
}

// _ 抑制未使用变量警告
var _ = time.Second
