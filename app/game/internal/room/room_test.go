package room

import (
	"testing"
	"time"

	"go-zero-ddz/pkg/cardutil"
	"go-zero-ddz/pkg/types"
)

func TestNewRoom(t *testing.T) {
	roomID := "test-room-001"
	r := NewRoom(roomID)

	if r.ID != roomID {
		t.Errorf("Expected room ID %s, got %s", roomID, r.ID)
	}
	if r.State != types.StateWaiting {
		t.Errorf("Expected state waiting, got %v", r.State)
	}
	if len(r.Players) != 0 {
		t.Errorf("Expected 0 players, got %d", len(r.Players))
	}
}

func TestAddPlayer(t *testing.T) {
	r := NewRoom("test-room")

	p1 := &Player{
		UID:      "player1",
		Nickname: "Player 1",
		IsBot:    false,
	}

	err := r.AddPlayer(p1)
	if err != nil {
		t.Errorf("Failed to add player: %v", err)
	}

	if len(r.Players) != 1 {
		t.Errorf("Expected 1 player, got %d", len(r.Players))
	}

	// 重复添加同一玩家
	err = r.AddPlayer(p1)
	if err == nil {
		t.Error("Expected error when adding duplicate player")
	}

	// 添加第三个玩家
	p2 := &Player{UID: "player2", Nickname: "Player 2", IsBot: false}
	p3 := &Player{UID: "player3", Nickname: "Player 3", IsBot: false}
	r.AddPlayer(p2)
	r.AddPlayer(p3)

	if !r.IsFull() {
		t.Error("Expected room to be full")
	}

	// 尝试添加第四个玩家
	p4 := &Player{UID: "player4", Nickname: "Player 4", IsBot: false}
	err = r.AddPlayer(p4)
	if err == nil {
		t.Error("Expected error when adding to full room")
	}
}

func TestRemovePlayer(t *testing.T) {
	r := NewRoom("test-room")

	p1 := &Player{UID: "player1", Nickname: "Player 1", IsBot: false}
	r.AddPlayer(p1)

	err := r.RemovePlayer("player1")
	if err != nil {
		t.Errorf("Failed to remove player: %v", err)
	}

	if len(r.Players) != 0 {
		t.Errorf("Expected 0 players, got %d", len(r.Players))
	}

	// 移除不存在的玩家
	err = r.RemovePlayer("nonexistent")
	if err == nil {
		t.Error("Expected error when removing nonexistent player")
	}
}

func TestGetPlayer(t *testing.T) {
	r := NewRoom("test-room")

	p1 := &Player{UID: "player1", Nickname: "Player 1", IsBot: false}
	r.AddPlayer(p1)

	p, exists := r.GetPlayer("player1")
	if !exists {
		t.Error("Expected player to exist")
	}
	if p.UID != "player1" {
		t.Errorf("Expected player1, got %s", p.UID)
	}

	_, exists = r.GetPlayer("nonexistent")
	if exists {
		t.Error("Expected player to not exist")
	}
}

func TestIsFull(t *testing.T) {
	r := NewRoom("test-room")

	if r.IsFull() {
		t.Error("Empty room should not be full")
	}

	r.AddPlayer(&Player{UID: "p1", Nickname: "P1", IsBot: false})
	r.AddPlayer(&Player{UID: "p2", Nickname: "P2", IsBot: false})

	if r.IsFull() {
		t.Error("Room with 2 players should not be full")
	}

	r.AddPlayer(&Player{UID: "p3", Nickname: "P3", IsBot: false})

	if !r.IsFull() {
		t.Error("Room with 3 players should be full")
	}
}

func TestAllReady(t *testing.T) {
	r := NewRoom("test-room")

	if r.AllReady() {
		t.Error("Empty room should not be all ready")
	}

	p1 := &Player{UID: "p1", Nickname: "P1", IsBot: false, IsReady: true}
	p2 := &Player{UID: "p2", Nickname: "P2", IsBot: false, IsReady: true}
	p3 := &Player{UID: "p3", Nickname: "P3", IsBot: false, IsReady: false}

	r.AddPlayer(p1)
	r.AddPlayer(p2)
	r.AddPlayer(p3)

	if r.AllReady() {
		t.Error("Not all players ready")
	}

	p3.IsReady = true
	if !r.AllReady() {
		t.Error("All players should be ready")
	}
}

func TestSetReady(t *testing.T) {
	r := NewRoom("test-room")

	p1 := &Player{UID: "p1", Nickname: "P1", IsBot: false, IsReady: false}
	r.AddPlayer(p1)

	err := r.SetReady("p1", true)
	if err != nil {
		t.Errorf("Failed to set ready: %v", err)
	}

	p, _ := r.GetPlayer("p1")
	if !p.IsReady {
		t.Error("Expected player to be ready")
	}

	err = r.SetReady("nonexistent", true)
	if err == nil {
		t.Error("Expected error for nonexistent player")
	}
}

func TestSetOnline(t *testing.T) {
	r := NewRoom("test-room")

	p1 := &Player{UID: "p1", Nickname: "P1", IsBot: false, IsOnline: true}
	r.AddPlayer(p1)

	r.SetOnline("p1", false)
	p, _ := r.GetPlayer("p1")
	if p.IsOnline {
		t.Error("Expected player to be offline")
	}
	if p.DisconnectAt.IsZero() {
		t.Error("Expected disconnect time to be set")
	}

	r.SetOnline("p1", true)
	p, _ = r.GetPlayer("p1")
	if !p.IsOnline {
		t.Error("Expected player to be online")
	}
	if !p.DisconnectAt.IsZero() {
		t.Error("Expected disconnect time to be cleared")
	}
}

func TestNextTurn(t *testing.T) {
	r := NewRoom("test-room")

	r.AddPlayer(&Player{UID: "p1", Nickname: "P1", IsBot: false})
	r.AddPlayer(&Player{UID: "p2", Nickname: "P2", IsBot: false})
	r.AddPlayer(&Player{UID: "p3", Nickname: "P3", IsBot: false})

	// 逆时针方向：p1 -> p3 -> p2 -> p1
	next := r.NextTurn("p1")
	if next != "p3" {
		t.Errorf("Expected p3, got %s", next)
	}

	next = r.NextTurn("p3")
	if next != "p2" {
		t.Errorf("Expected p2, got %s", next)
	}

	next = r.NextTurn("p2")
	if next != "p1" {
		t.Errorf("Expected p1, got %s", next)
	}
}

func TestGetPlayerOrder(t *testing.T) {
	r := NewRoom("test-room")

	r.AddPlayer(&Player{UID: "p1", Nickname: "P1", IsBot: false})
	r.AddPlayer(&Player{UID: "p2", Nickname: "P2", IsBot: false})
	r.AddPlayer(&Player{UID: "p3", Nickname: "P3", IsBot: false})

	order := r.GetPlayerOrder("p1")
	if order != 0 {
		t.Errorf("Expected 0, got %d", order)
	}

	order = r.GetPlayerOrder("p2")
	if order != 1 {
		t.Errorf("Expected 1, got %d", order)
	}

	order = r.GetPlayerOrder("nonexistent")
	if order != -1 {
		t.Errorf("Expected -1, got %d", order)
	}
}

func TestIsPeasant(t *testing.T) {
	r := NewRoom("test-room")

	r.AddPlayer(&Player{UID: "p1", Nickname: "P1", IsBot: false, IsLandlord: false})
	r.AddPlayer(&Player{UID: "p2", Nickname: "P2", IsBot: false, IsLandlord: true})

	if !r.IsPeasant("p1") {
		t.Error("p1 should be peasant")
	}
	if r.IsPeasant("p2") {
		t.Error("p2 is landlord, should not be peasant")
	}
	if r.IsPeasant("nonexistent") {
		t.Error("nonexistent player should not be peasant")
	}
}

func TestGetOtherPeasant(t *testing.T) {
	r := NewRoom("test-room")

	r.AddPlayer(&Player{UID: "landlord", Nickname: "Landlord", IsBot: false, IsLandlord: true})
	r.AddPlayer(&Player{UID: "peasant1", Nickname: "Peasant1", IsBot: false, IsLandlord: false})
	r.AddPlayer(&Player{UID: "peasant2", Nickname: "Peasant2", IsBot: false, IsLandlord: false})

	other, exists := r.GetOtherPeasant("peasant1")
	if !exists {
		t.Error("Expected other peasant to exist")
	}
	if other != "peasant2" {
		t.Errorf("Expected peasant2, got %s", other)
	}

	_, exists = r.GetOtherPeasant("landlord")
	if exists {
		t.Error("Landlord should not have other peasant")
	}
}

func TestGetCardCounts(t *testing.T) {
	r := NewRoom("test-room")

	p1 := &Player{UID: "p1", Nickname: "P1", IsBot: false, Cards: []cardutil.Card{{Value: 3, Suit: 1}, {Value: 4, Suit: 1}}}
	p2 := &Player{UID: "p2", Nickname: "P2", IsBot: false, Cards: []cardutil.Card{{Value: 5, Suit: 1}}}
	p3 := &Player{UID: "p3", Nickname: "P3", IsBot: false, Cards: nil}

	r.AddPlayer(p1)
	r.AddPlayer(p2)
	r.AddPlayer(p3)

	counts := r.GetCardCounts()
	if counts["p1"] != 2 {
		t.Errorf("Expected p1 to have 2 cards, got %d", counts["p1"])
	}
	if counts["p2"] != 1 {
		t.Errorf("Expected p2 to have 1 card, got %d", counts["p2"])
	}
	if counts["p3"] != 0 {
		t.Errorf("Expected p3 to have 0 cards, got %d", counts["p3"])
	}
}

func TestResetPlayersState(t *testing.T) {
	r := NewRoom("test-room")

	p1 := &Player{UID: "p1", Nickname: "P1", IsBot: false, IsLandlord: true, IsAIControlled: true, Role: types.RoleLandlord}
	p2 := &Player{UID: "p2", Nickname: "P2", IsBot: true, IsLandlord: false, IsAIControlled: false, Role: types.RolePeasant}

	r.AddPlayer(p1)
	r.AddPlayer(p2)

	r.ResetPlayersState()

	p, _ := r.GetPlayer("p1")
	if p.IsLandlord {
		t.Error("Expected IsLandlord to be false after reset")
	}
	if p.IsAIControlled {
		t.Error("Expected IsAIControlled to be false after reset")
	}
	if p.Role != types.RoleUnknown {
		t.Errorf("Expected RoleUnknown, got %v", p.Role)
	}

	p, _ = r.GetPlayer("p2")
	if p.IsAIControlled {
		t.Error("Bot should remain not AI controlled")
	}
}

func TestSetState(t *testing.T) {
	r := NewRoom("test-room")

	stateChanged := false
	var oldState, newState types.RoomState

	r.OnStateChange = func(room *Room, old, new types.RoomState) {
		stateChanged = true
		oldState = old
		newState = new
	}

	r.SetState(types.StateDealing)

	if !stateChanged {
		t.Error("Expected state change callback to be called")
	}
	if oldState != types.StateWaiting {
		t.Errorf("Expected old state waiting, got %v", oldState)
	}
	if newState != types.StateDealing {
		t.Errorf("Expected new state dealing, got %v", newState)
	}
}

func TestInitGameState(t *testing.T) {
	r := NewRoom("test-room")

	p1 := &Player{UID: "p1", Nickname: "P1", IsBot: false, IsAIControlled: true}
	p2 := &Player{UID: "p2", Nickname: "P2", IsBot: true, IsAIControlled: false}
	r.AddPlayer(p1)
	r.AddPlayer(p2)

	gsm := r.InitGameState()

	if gsm == nil {
		t.Error("Expected game state machine to be created")
	}

	p, _ := r.GetPlayer("p1")
	if p.IsAIControlled {
		t.Error("Non-bot player should have IsAIControlled reset to false")
	}

	p, _ = r.GetPlayer("p2")
	if !p.IsBot {
		t.Error("p2 should be bot")
	}
}

func TestStartAndStopTimer(t *testing.T) {
	r := NewRoom("test-room")

	r.StartTimer(1, "test-player")

	// 检查计时器是否启动
	r.mu.RLock()
	timerVal := r.Timer
	currentTurn := r.CurrentTurnUID
	r.mu.RUnlock()

	if timerVal != 1 {
		t.Errorf("Expected timer 1, got %d", timerVal)
	}
	if currentTurn != "test-player" {
		t.Errorf("Expected current turn test-player, got %s", currentTurn)
	}

	r.StopTimer()

	// 等待一小段时间确保计时器停止
	time.Sleep(100 * time.Millisecond)

	r.timerLock.Lock()
	timer := r.timer
	r.timerLock.Unlock()

	if timer != nil {
		t.Error("Expected timer to be nil after stop")
	}
}

func TestCount(t *testing.T) {
	r := NewRoom("test-room")

	if r.Count() != 0 {
		t.Errorf("Expected 0, got %d", r.Count())
	}

	r.AddPlayer(&Player{UID: "p1", Nickname: "P1", IsBot: false})
	if r.Count() != 1 {
		t.Errorf("Expected 1, got %d", r.Count())
	}

	r.AddPlayer(&Player{UID: "p2", Nickname: "P2", IsBot: false})
	if r.Count() != 2 {
		t.Errorf("Expected 2, got %d", r.Count())
	}
}
