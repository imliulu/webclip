package hub

import "sync"

// Client 代表一个 WebSocket 连接
type Client struct {
	ID   string
	Send chan []byte
}

// Hub 管理按房间分组的客户端
type Hub struct {
	rooms map[string]map[*Client]struct{}
	mu    sync.RWMutex
}

// Default 默认全局 Hub 实例
var Default = NewHub()

// NewHub 构造新的 Hub
func NewHub() *Hub {
	return &Hub{rooms: make(map[string]map[*Client]struct{})}
}

// Join 将客户端加入房间
func (h *Hub) Join(code string, c *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	m, ok := h.rooms[code]
	if !ok {
		m = make(map[*Client]struct{})
		h.rooms[code] = m
	}
	m[c] = struct{}{}
}

// Leave 将客户端从房间移除
func (h *Hub) Leave(code string, c *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if m, ok := h.rooms[code]; ok {
		delete(m, c)
		if len(m) == 0 {
			delete(h.rooms, code)
		}
	}
}

// Broadcast 向房间内除发送者外的所有客户端广播
func (h *Hub) Broadcast(code string, from *Client, msg []byte) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	m, ok := h.rooms[code]
	if !ok {
		return
	}
	for c := range m {
		if c == from {
			continue
		}
		select {
		case c.Send <- msg:
		default:
			// 慢客户端：丢弃消息以免阻塞广播
		}
	}
}
