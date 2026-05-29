package cluster

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/redis/go-redis/v9"
)

type InstanceInfo struct {
	InstanceID  string `json:"instance_id"`
	Host        string `json:"host"`
	Port        int    `json:"port"`
	Status      string `json:"status"`
	Connections int    `json:"connections"`
	Rooms       int    `json:"rooms"`
	StartedAt   string `json:"started_at"`
}

type Registry struct {
	rdb        redis.UniversalClient
	instanceID string
	info       InstanceInfo
	ctx        context.Context
	cancel     context.CancelFunc
	ttl        time.Duration
}

func NewRegistry(rdb redis.UniversalClient, host string, port int) *Registry {
	instanceID := generateInstanceID()
	ctx, cancel := context.WithCancel(context.Background())

	return &Registry{
		rdb:        rdb,
		instanceID: instanceID,
		info: InstanceInfo{
			InstanceID: instanceID,
			Host:       host,
			Port:       port,
			Status:     "active",
			StartedAt:  time.Now().UTC().Format(time.RFC3339),
		},
		ctx:    ctx,
		cancel: cancel,
		ttl:    30 * time.Second,
	}
}

func generateInstanceID() string {
	hostname, _ := os.Hostname()
	return fmt.Sprintf("%s-%d", hostname, os.Getpid())
}

func (r *Registry) Register() error {
	key := fmt.Sprintf("ddz:instance:%s", r.instanceID)
	val, _ := json.Marshal(r.info)
	_, err := r.rdb.SetEx(r.ctx, key, string(val), r.ttl).Result()
	if err != nil {
		return fmt.Errorf("register instance: %w", err)
	}
	go r.heartbeatLoop()
	r.broadcastEvent("instance_joined", r.info)
	return nil
}

func (r *Registry) heartbeatLoop() {
	ticker := time.NewTicker(r.ttl / 3)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			r.refreshHeartbeat()
		case <-r.ctx.Done():
			return
		}
	}
}

func (r *Registry) refreshHeartbeat() {
	key := fmt.Sprintf("ddz:instance:%s", r.instanceID)
	val, _ := json.Marshal(r.info)
	r.rdb.SetEx(r.ctx, key, string(val), r.ttl)
}

func (r *Registry) Unregister() error {
	r.cancel()
	r.info.Status = "offline"
	key := fmt.Sprintf("ddz:instance:%s", r.instanceID)
	val, _ := json.Marshal(r.info)
	r.rdb.Set(r.ctx, key, string(val), 60*time.Second)
	r.broadcastEvent("instance_left", r.info)
	return nil
}

func (r *Registry) GetInstanceID() string {
	return r.instanceID
}

func (r *Registry) UpdateStats(connections, rooms int) {
	r.info.Connections = connections
	r.info.Rooms = rooms
}

func (r *Registry) broadcastEvent(eventType string, info InstanceInfo) {
	event := map[string]any{
		"type":      eventType,
		"instance":  info,
		"timestamp": time.Now().UnixMilli(),
	}
	payload, _ := json.Marshal(event)
	r.rdb.Publish(r.ctx, "global:events", string(payload))
}
