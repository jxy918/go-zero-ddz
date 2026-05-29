package cluster

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

type PlayerRoute struct {
	InstanceID string `json:"instance_id"`
	RoomID     string `json:"room_id"`
	ConnID     string `json:"conn_id"`
}

type Router struct {
	rdb        redis.UniversalClient
	instanceID string
	ttl        time.Duration
}

func NewRouter(rdb redis.UniversalClient, instanceID string) *Router {
	return &Router{
		rdb:        rdb,
		instanceID: instanceID,
		ttl:        5 * time.Minute,
	}
}

func (r *Router) RegisterPlayer(ctx context.Context, uid string, roomID, connID string) error {
	key := fmt.Sprintf("ddz:player:route:%s", uid)
	route := PlayerRoute{
		InstanceID: r.instanceID,
		RoomID:     roomID,
		ConnID:     connID,
	}
	val, _ := json.Marshal(route)
	_, err := r.rdb.SetEx(ctx, key, string(val), r.ttl).Result()
	return err
}

func (r *Router) GetPlayerRoute(ctx context.Context, uid string) (*PlayerRoute, error) {
	key := fmt.Sprintf("ddz:player:route:%s", uid)
	val, err := r.rdb.Get(ctx, key).Result()
	if err != nil {
		return nil, err
	}
	var route PlayerRoute
	if err := json.Unmarshal([]byte(val), &route); err != nil {
		return nil, err
	}
	return &route, nil
}

func (r *Router) IsLocalPlayer(ctx context.Context, uid string) bool {
	route, err := r.GetPlayerRoute(ctx, uid)
	if err != nil {
		return false
	}
	return route.InstanceID == r.instanceID
}

func (r *Router) UnregisterPlayer(ctx context.Context, uid string) error {
	key := fmt.Sprintf("ddz:player:route:%s", uid)
	return r.rdb.Del(ctx, key).Err()
}

func (r *Router) RenewPlayerRoute(ctx context.Context, uid string) error {
	key := fmt.Sprintf("ddz:player:route:%s", uid)
	return r.rdb.Expire(ctx, key, r.ttl).Err()
}
