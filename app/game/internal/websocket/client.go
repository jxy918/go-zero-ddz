package websocket

import (
	"fmt"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// Client 表示一个 WebSocket 客户端连接
type Client struct {
	ID       string
	UID      string // 用户 ID（登录后填充）
	RoomID   string // 所在房间 ID
	Conn     *websocket.Conn
	Send     chan []byte // 发送消息队列
	mu       sync.RWMutex
	lastPing time.Time // 最后一次心跳时间
}

// NewClient 创建新客户端
func NewClient(id string, conn *websocket.Conn, sendBufferSize int) *Client {
	return &Client{
		ID:       id,
		Conn:     conn,
		Send:     make(chan []byte, sendBufferSize),
		lastPing: time.Now(),
	}
}

// SendBinary 发送二进制消息
func (c *Client) SendBinary(data []byte) error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.Conn == nil {
		return fmt.Errorf("client %s connection closed", c.ID)
	}

	c.Conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	return c.Conn.WriteMessage(websocket.BinaryMessage, data)
}

// SendMsg 编码并发送消息
func (c *Client) SendMsg(msgID uint16, payload []byte) error {
	data, err := Encode(msgID, payload)
	if err != nil {
		return fmt.Errorf("encode message: %w", err)
	}

	select {
	case c.Send <- data:
		return nil
	default:
		return fmt.Errorf("send buffer full for client %s", c.ID)
	}
}

// ReadLoop 读取消息循环（保留用于兼容）
func (c *Client) ReadLoop() {
	defer func() {
		c.Conn.Close()
	}()

	c.Conn.SetReadLimit(MaxMessageSize)

	for {
		_, _, err := c.Conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				// 意外断开
			}
			return
		}
	}
}

// WriteLoop 写入消息循环
func (c *Client) WriteLoop() {
	ticker := time.NewTicker(5 * time.Second)
	defer func() {
		ticker.Stop()
		c.Conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.Send:
			if !ok {
				c.Conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			c.mu.RLock()
			if c.Conn == nil {
				c.mu.RUnlock()
				return
			}
			c.Conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			err := c.Conn.WriteMessage(websocket.BinaryMessage, message)
			c.mu.RUnlock()

			if err != nil {
				return
			}

		case <-ticker.C:
			// 定期清理过期的 Send 通道
			c.mu.RLock()
			if c.Conn == nil {
				c.mu.RUnlock()
				return
			}
			c.mu.RUnlock()
		}
	}
}

// UpdateHeartbeat 更新心跳时间
func (c *Client) UpdateHeartbeat() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.lastPing = time.Now()
}

// LastHeartbeat 获取最后心跳时间
func (c *Client) LastHeartbeat() time.Time {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.lastPing
}

// Close 关闭客户端连接
func (c *Client) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.Conn != nil {
		c.Conn.Close()
		c.Conn = nil
	}
	// 防止重复关闭通道
	select {
	case <-c.Send:
		// 通道已关闭
	default:
		close(c.Send)
	}
}
