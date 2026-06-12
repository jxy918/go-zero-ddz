package handler

import (
	"context"
	"encoding/json"
	"time"

	"go-zero-ddz/app/game/internal/match"
	"go-zero-ddz/app/game/internal/websocket"
	"go-zero-ddz/pkg/types"

	"github.com/zeromicro/go-zero/core/logx"
)

// handleMatchStart 处理开始匹配请求
func (hm *HandlerManager) handleMatchStart(client *websocket.Client, msgID uint16, payload []byte) {
	if client.UID == "" {
		hm.sendError(client, msgID, 401, "not logged in")
		return
	}

	type MatchStartReq struct {
		MatchType int    `json:"match_type"`
		ELO       int32  `json:"elo"`
		Tier      string `json:"tier"`
	}
	var req MatchStartReq
	if err := json.Unmarshal(payload, &req); err != nil {
		hm.sendError(client, msgID, 400, "invalid request")
		return
	}

	if hm.coordinator == nil {
		hm.sendError(client, msgID, 503, "matchmaking not available")
		return
	}

	player := &match.WaitingPlayer{
		UID:       client.UID,
		ELO:       req.ELO,
		Tier:      req.Tier,
		WaitStart: time.Now(),
		MatchType: match.MatchType(req.MatchType),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := hm.coordinator.Enqueue(ctx, player); err != nil {
		hm.sendError(client, msgID, 500, "failed to join queue: "+err.Error())
		return
	}

	logx.Infof("Player %s joined matchmaking (type=%d, elo=%d, tier=%s)", client.UID, req.MatchType, req.ELO, req.Tier)

	hm.sendMsg(client, types.MsgMatchStartReq, map[string]interface{}{
		"success":    true,
		"match_type": req.MatchType,
		"message":    "已加入匹配队列",
	})
}

// handleMatchCancel 处理取消匹配
func (hm *HandlerManager) handleMatchCancel(client *websocket.Client, msgID uint16, payload []byte) {
	if hm.coordinator != nil {
		hm.coordinator.Cancel(context.Background(), client.UID)
	}
	logx.Infof("Player %s cancelled matchmaking", client.UID)
}
