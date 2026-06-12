package websocket

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/zeromicro/go-zero/core/logx"

	"go-zero-ddz/app/game/internal/config"
)

// MessageHandler 消息处理函数
type MessageHandler func(client *Client, msgID uint16, payload []byte)

// Hub 管理所有 WebSocket 连接
type Hub struct {
	config   *config.WebSocketConfig
	upgrader websocket.Upgrader

	// 已注册的客户端
	clients      map[string]*Client
	clientsByUID map[string]*Client
	clientsMu    sync.RWMutex

	// 消息处理器注册表
	handlers   map[uint16]MessageHandler
	handlersMu sync.RWMutex

	// 注册/注销通道
	register   chan *Client
	unregister chan *Client

	// 服务状态
	ctx    context.Context
	cancel context.CancelFunc

	// 连接统计
	stats      ConnectionStats
	statsMu    sync.Mutex
	maxClients int
}

// ConnectionStats 连接统计
type ConnectionStats struct {
	TotalConnections   int64
	CurrentConnections int
	MaxConnections     int
	MessageCount       int64
	ErrorCount         int64
	LastResetTime      time.Time
}

// NewHub 创建 Hub
func NewHub(cfg *config.WebSocketConfig) *Hub {
	ctx, cancel := context.WithCancel(context.Background())

	return &Hub{
		config:     cfg,
		maxClients: 10000, // 默认最大连接数
		upgrader: websocket.Upgrader{
			ReadBufferSize:  cfg.ReadBufferSize,
			WriteBufferSize: cfg.WriteBufferSize,
			CheckOrigin: func(r *http.Request) bool {
				return true // 生产环境需要验证 Origin
			},
		},
		clients:      make(map[string]*Client),
		clientsByUID: make(map[string]*Client),
		handlers:     make(map[uint16]MessageHandler),
		register:     make(chan *Client),
		unregister:   make(chan *Client),
		ctx:          ctx,
		cancel:       cancel,
		stats: ConnectionStats{
			LastResetTime: time.Now(),
		},
	}
}

// SetMaxClients 设置最大连接数
func (h *Hub) SetMaxClients(max int) {
	h.maxClients = max
}

// GetStats 获取连接统计
func (h *Hub) GetStats() ConnectionStats {
	h.statsMu.Lock()
	defer h.statsMu.Unlock()
	return h.stats
}

// ResetStats 重置统计
func (h *Hub) ResetStats() {
	h.statsMu.Lock()
	defer h.statsMu.Unlock()
	h.stats = ConnectionStats{
		CurrentConnections: h.stats.CurrentConnections,
		LastResetTime:      time.Now(),
	}
}

// incrStats 增加统计计数
func (h *Hub) incrStats(field string) {
	h.statsMu.Lock()
	defer h.statsMu.Unlock()
	switch field {
	case "connections":
		h.stats.TotalConnections++
		h.stats.CurrentConnections++
		if h.stats.CurrentConnections > h.stats.MaxConnections {
			h.stats.MaxConnections = h.stats.CurrentConnections
		}
	case "disconnections":
		h.stats.CurrentConnections--
	case "messages":
		h.stats.MessageCount++
	case "errors":
		h.stats.ErrorCount++
	}
}

// Run 启动 Hub 主循环
func (h *Hub) Run() {
	logx.Info("WebSocket Hub started")

	go h.heartbeatCheck()

	for {
		select {
		case client := <-h.register:
			h.clientsMu.Lock()
			if len(h.clients) >= h.maxClients {
				h.clientsMu.Unlock()
				logx.Infof("Client %s rejected: max connections reached (%d)", client.ID, h.maxClients)
				client.Close()
				continue
			}
			h.clients[client.ID] = client
			if client.UID != "" {
				h.clientsByUID[client.UID] = client
			}
			count := len(h.clients)
			h.clientsMu.Unlock()
			h.incrStats("connections")
			logx.Infof("Client registered: %s (total: %d)", client.ID, count)

		case client := <-h.unregister:
			h.clientsMu.Lock()
			if _, ok := h.clients[client.ID]; ok {
				delete(h.clients, client.ID)
				if client.UID != "" {
					delete(h.clientsByUID, client.UID)
				}
			}
			count := len(h.clients)
			h.clientsMu.Unlock()
			client.Close()
			h.incrStats("disconnections")
			logx.Infof("Client unregistered: %s (total: %d)", client.ID, count)

		case <-h.ctx.Done():
			logx.Info("WebSocket Hub stopping...")
			h.clientsMu.Lock()
			for id, client := range h.clients {
				client.Close()
				delete(h.clients, id)
			}
			h.clientsByUID = make(map[string]*Client)
			h.clientsMu.Unlock()
			return
		}
	}
}

// ServeHTTP 处理 WebSocket 升级请求
func (h *Hub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	conn, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		logx.Infof("WebSocket upgrade error: %v", err)
		return
	}

	clientID := fmt.Sprintf("conn-%d", time.Now().UnixNano())
	client := NewClient(clientID, conn, 256)

	token := r.URL.Query().Get("token")
	if token != "" {
		uid := extractUIDFromToken(token)
		if uid != "" {
			client.UID = uid
			logx.Infof("Client %s auto-login with UID: %s", clientID, uid)
		}
	}

	h.register <- client

	go h.readPump(client)
	go h.writePump(client)
}

// extractUIDFromToken 从 token 中提取 UID（简化版，与 handler.go 中的实现一致）
func extractUIDFromToken(token string) string {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return token
	}

	payloadBytes, err := base64.RawStdEncoding.DecodeString(parts[1])
	if err != nil {
		payloadBytes, err = base64.StdEncoding.DecodeString(parts[1])
	}
	if err != nil {
		return token
	}

	var claims map[string]interface{}
	if err := json.Unmarshal(payloadBytes, &claims); err != nil {
		return token
	}

	if uid, ok := claims["uid"].(string); ok {
		return uid
	}
	return token
}

// readPump 从客户端读取消息并分发
func (h *Hub) readPump(client *Client) {
	defer func() {
		h.unregister <- client
	}()

	client.mu.RLock()
	if client.Conn == nil {
		client.mu.RUnlock()
		return
	}
	client.Conn.SetReadLimit(int64(h.config.MaxMessageSize))
	client.mu.RUnlock()

	for {
		client.mu.RLock()
		if client.Conn == nil {
			client.mu.RUnlock()
			return
		}
		_, message, err := client.Conn.ReadMessage()
		client.mu.RUnlock()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				logx.Infof("Client %s unexpected disconnect: %v", client.ID, err)
			}
			return
		}

		logx.Infof("Client %s received raw message: %d bytes, first 10 bytes: %v", client.ID, len(message), message[:min(len(message), 10)])

		msgID, payload, err := DecodeFromBytes(message)
		if err != nil {
			logx.Infof("Client %s decode error: %v", client.ID, err)
			continue
		}

		logx.Infof("Client %s decoded message: msgID=0x%04X, payload length=%d", client.ID, msgID, len(payload))

		if msgID == 0x0001 {
			client.UpdateHeartbeat()
			h.sendHeartbeatRespSafe(client)
			continue
		}

		h.handlersMu.RLock()
		handler, exists := h.handlers[msgID]
		h.handlersMu.RUnlock()

		if !exists {
			logx.Infof("No handler for message ID: 0x%04X", msgID)
			continue
		}

		logx.Infof("Dispatching message 0x%04X to handler", msgID)
		go handler(client, msgID, payload)
	}
}

// writePump 向客户端发送消息
func (h *Hub) writePump(client *Client) {
	ticker := time.NewTicker(time.Duration(h.config.PingPeriod) * time.Second)
	defer func() {
		ticker.Stop()
		client.Close()
	}()

	for {
		select {
		case message, ok := <-client.Send:
			if !ok {
				client.mu.RLock()
				if client.Conn != nil {
					client.Conn.WriteMessage(websocket.CloseMessage, []byte{})
				}
				client.mu.RUnlock()
				return
			}

			client.mu.RLock()
			if client.Conn == nil {
				client.mu.RUnlock()
				return
			}
			client.Conn.SetWriteDeadline(time.Now().Add(time.Duration(h.config.WriteWait) * time.Second))
			err := client.Conn.WriteMessage(websocket.BinaryMessage, message)
			client.mu.RUnlock()
			if err != nil {
				logx.Infof("Client %s write error: %v", client.ID, err)
				return
			}

		case <-ticker.C:
			client.mu.RLock()
			if client.Conn == nil {
				client.mu.RUnlock()
				return
			}
			client.Conn.SetWriteDeadline(time.Now().Add(time.Duration(h.config.WriteWait) * time.Second))
			err := client.Conn.WriteMessage(websocket.PingMessage, nil)
			client.mu.RUnlock()
			if err != nil {
				logx.Infof("Client %s ping error: %v", client.ID, err)
				return
			}
		}
	}
}

// RegisterHandler 注册消息处理器
func (h *Hub) RegisterHandler(msgID uint16, handler MessageHandler) {
	h.handlersMu.Lock()
	defer h.handlersMu.Unlock()
	h.handlers[msgID] = handler
	logx.Infof("Registered handler for message ID: 0x%04X", msgID)
}

// BroadcastToRoom 向房间内所有客户端广播消息
func (h *Hub) BroadcastToRoom(roomID string, msgID uint16, payload []byte) {
	data, err := Encode(msgID, payload)
	if err != nil {
		logx.Infof("Encode broadcast message error: %v", err)
		return
	}

	h.clientsMu.RLock()
	defer h.clientsMu.RUnlock()

	for _, client := range h.clients {
		if client.RoomID == roomID {
			select {
			case client.Send <- data:
			default:
				logx.Infof("Client %s send buffer full, skipping broadcast", client.ID)
			}
		}
	}
}

// SendToClient 向指定客户端发送消息
func (h *Hub) SendToClient(clientID string, msgID uint16, payload []byte) error {
	h.clientsMu.RLock()
	client, exists := h.clients[clientID]
	h.clientsMu.RUnlock()

	if !exists {
		return fmt.Errorf("client %s not found", clientID)
	}

	data, err := Encode(msgID, payload)
	if err != nil {
		return err
	}

	select {
	case client.Send <- data:
		return nil
	default:
		return fmt.Errorf("client %s send buffer full", clientID)
	}
}

// GetClientCount 获取连接数
func (h *Hub) GetClientCount() int {
	h.clientsMu.RLock()
	defer h.clientsMu.RUnlock()
	return len(h.clients)
}

// GetClientByUID 通过 UID 查找客户端
func (h *Hub) GetClientByUID(uid string) *Client {
	h.clientsMu.RLock()
	defer h.clientsMu.RUnlock()

	// 优先使用索引查找
	if client, exists := h.clientsByUID[uid]; exists {
		return client
	}
	return nil
}

// heartbeatCheck 定期检查心跳，断开超时客户端
func (h *Hub) heartbeatCheck() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	pongWait := time.Duration(h.config.PongWait) * time.Second

	for {
		select {
		case <-ticker.C:
			h.clientsMu.RLock()
			for _, client := range h.clients {
				if time.Since(client.LastHeartbeat()) > pongWait {
					logx.Infof("Client %s heartbeat timeout, disconnecting", client.ID)
					go func(c *Client) {
						h.unregister <- c
						c.Close()
					}(client)
				}
			}
			h.clientsMu.RUnlock()

		case <-h.ctx.Done():
			return
		}
	}
}

// sendHeartbeatResp 发送心跳响应
func (h *Hub) sendHeartbeatResp(client *Client) {
	resp, err := Encode(0x0002, []byte("{}"))
	if err != nil {
		logx.Infof("encode heartbeat resp error: %v", err)
		return
	}

	select {
	case client.Send <- resp:
	default:
	}
}

// sendHeartbeatRespSafe 安全发送心跳响应（带关闭检查）
func (h *Hub) sendHeartbeatRespSafe(client *Client) {
	h.clientsMu.RLock()
	_, exists := h.clients[client.ID]
	h.clientsMu.RUnlock()

	if !exists {
		logx.Infof("Client %s not found, skip heartbeat response", client.ID)
		return
	}

	h.sendHeartbeatResp(client)
}

// Stop 停止 Hub
func (h *Hub) Stop() {
	h.cancel()
}
