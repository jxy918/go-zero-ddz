package match

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// MatchType 匹配类型
type MatchType int

const (
	MatchTypeRandom MatchType = 0
	MatchTypeRanked MatchType = 1
)

// WaitingPlayer 等待匹配的玩家
type WaitingPlayer struct {
	UID        string    `json:"uid"`
	ELO        int32     `json:"elo"`
	Tier       string    `json:"tier"`
	InstanceID string    `json:"instance_id"`
	WaitStart  time.Time `json:"wait_start"`
	MatchType  MatchType `json:"match_type"`
}

// Queue 匹配队列
type Queue struct {
	rdb    redis.UniversalClient
	prefix string
}

// NewQueue 创建匹配队列
func NewQueue(rdb redis.UniversalClient) *Queue {
	return &Queue{
		rdb:    rdb,
		prefix: "ddz:match:queue",
	}
}

// Enqueue 入队
func (q *Queue) Enqueue(ctx context.Context, player *WaitingPlayer) error {
	// 防止重复入队
	lockKey := fmt.Sprintf("ddz:match:lock:%s", player.UID)
	acquired, err := q.rdb.SetNX(ctx, lockKey, "1", 30*time.Second).Result()
	if err != nil || !acquired {
		return fmt.Errorf("player already in queue or lock failed")
	}

	data, _ := json.Marshal(player)

	var queueKey string
	if player.MatchType == MatchTypeRandom {
		queueKey = fmt.Sprintf("%s:random", q.prefix)
		// score = 等待时间戳（先到先匹配）
		score := float64(player.WaitStart.UnixMilli())
		return q.rdb.ZAdd(ctx, queueKey, redis.Z{
			Score:  score,
			Member: data,
		}).Err()
	}

	// 段位匹配：按段位分队列
	queueKey = fmt.Sprintf("%s:ranked:%s", q.prefix, player.Tier)
	score := float64(player.ELO)
	return q.rdb.ZAdd(ctx, queueKey, redis.Z{
		Score:  score,
		Member: data,
	}).Err()
}

// Dequeue 出队
func (q *Queue) Dequeue(ctx context.Context, uid string) error {
	lockKey := fmt.Sprintf("ddz:match:lock:%s", uid)
	q.rdb.Del(ctx, lockKey)

	// 从所有可能队列中移除
	cursor := uint64(0)
	pattern := fmt.Sprintf("%s:*", q.prefix)

	for {
		keys, next, err := q.rdb.Scan(ctx, cursor, pattern, 100).Result()
		if err != nil {
			return err
		}

		for _, key := range keys {
			// 遍历 sorted set 找到并移除
			members, err := q.rdb.ZRange(ctx, key, 0, -1).Result()
			if err != nil {
				continue
			}

			for _, member := range members {
				var p WaitingPlayer
				if err := json.Unmarshal([]byte(member), &p); err == nil && p.UID == uid {
					q.rdb.ZRem(ctx, key, member)
					break
				}
			}
		}

		cursor = next
		if cursor == 0 {
			break
		}
	}

	return nil
}

// GetRandomQueue 获取随机匹配队列中的玩家
func (q *Queue) GetRandomQueue(ctx context.Context, limit int64) ([]*WaitingPlayer, error) {
	key := fmt.Sprintf("%s:random", q.prefix)
	members, err := q.rdb.ZRange(ctx, key, 0, limit-1).Result()
	if err != nil {
		return nil, err
	}

	players := make([]*WaitingPlayer, 0, len(members))
	for _, member := range members {
		var p WaitingPlayer
		if err := json.Unmarshal([]byte(member), &p); err == nil {
			players = append(players, &p)
		}
	}

	return players, nil
}

// GetRankedQueue 获取段位匹配队列中的玩家（按 ELO 排序）
func (q *Queue) GetRankedQueue(ctx context.Context, tier string, eloMin, eloMax int32, limit int64) ([]*WaitingPlayer, error) {
	key := fmt.Sprintf("%s:ranked:%s", q.prefix, tier)
	members, err := q.rdb.ZRangeByScore(ctx, key, &redis.ZRangeBy{
		Min: fmt.Sprintf("%d", eloMin),
		Max: fmt.Sprintf("%d", eloMax),
		Count: limit,
	}).Result()
	if err != nil {
		return nil, err
	}

	players := make([]*WaitingPlayer, 0, len(members))
	for _, member := range members {
		var p WaitingPlayer
		if err := json.Unmarshal([]byte(member), &p); err == nil {
			players = append(players, &p)
		}
	}

	return players, nil
}

// RemovePlayers 从队列中移除多个玩家
func (q *Queue) RemovePlayers(ctx context.Context, players []*WaitingPlayer) error {
	for _, p := range players {
		q.Dequeue(ctx, p.UID)
	}
	return nil
}
