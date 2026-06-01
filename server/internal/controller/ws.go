package controller

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"time"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/net/ghttp"
	"github.com/gorilla/websocket"

	"webclip/server/internal/logic/clip"
	"webclip/server/internal/logic/hub"
)

// WS WebSocket 控制器
type WS struct{}

func NewWS() *WS { return &WS{} }

// wsIn 客户端 -> 服务端
type wsIn struct {
	Type        string `json:"type"`
	Content     string `json:"content,omitempty"`
	ContentType string `json:"contentType,omitempty"`
	Id          int64  `json:"id,omitempty"`
	FileKey     string `json:"fileKey,omitempty"`
	FileName    string `json:"fileName,omitempty"`
	FileSize    int64  `json:"fileSize,omitempty"`
}

// wsMessagePayload 消息负载
type wsMessagePayload struct {
	Id          int64       `json:"id"`
	Content     string      `json:"content"`
	ContentType string      `json:"contentType"`
	CreatedAt   interface{} `json:"createdAt"`
	FileName    string      `json:"fileName,omitempty"`
	FileSize    int64       `json:"fileSize,omitempty"`
	FileKey     string      `json:"fileKey,omitempty"`
}

// wsOut 服务端 -> 客户端
type wsOut struct {
	Type    string            `json:"type"`
	Message *wsMessagePayload `json:"message,omitempty"`
	Id      int64             `json:"id,omitempty"`
	From    string            `json:"from,omitempty"`
	Error   string            `json:"error,omitempty"`
}

const (
	readDeadline  = 60 * time.Second
	writeDeadline = 10 * time.Second
)

func randID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func sendBytes(c *hub.Client, b []byte) {
	select {
	case c.Send <- b:
	default:
	}
}

// Handle GET /api/ws/{code}?token=xxx
func (h *WS) Handle(r *ghttp.Request) {
	ctx := r.Context()
	code := r.Get("code").String()
	token := r.Get("token").String()

	// 鉴权（握手阶段使用 HTTP，失败返回 401）
	tokCode, err := clip.VerifyToken(ctx, token)
	if err != nil || tokCode != code {
		r.Response.WriteStatusExit(http.StatusUnauthorized, "unauthorized")
		return
	}

	// 房间必须存在且未过期
	if _, err := clip.GetByCode(ctx, code); err != nil {
		r.Response.WriteStatusExit(http.StatusNotFound, err.Error())
		return
	}

	ws, err := r.WebSocket()
	if err != nil {
		g.Log().Warningf(ctx, "ws upgrade failed: %v", err)
		return
	}
	conn := ws.Conn
	defer conn.Close()

	client := &hub.Client{ID: randID(), Send: make(chan []byte, 32)}
	hub.Default.Join(code, client)
	defer hub.Default.Leave(code, client)

	// writer goroutine
	writerDone := make(chan struct{})
	go func() {
		defer close(writerDone)
		ticker := time.NewTicker(25 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case msg, ok := <-client.Send:
				if !ok {
					return
				}
				_ = conn.SetWriteDeadline(time.Now().Add(writeDeadline))
				if err := conn.WriteMessage(websocket.TextMessage, msg); err != nil {
					return
				}
			case <-ticker.C:
				_ = conn.SetWriteDeadline(time.Now().Add(writeDeadline))
				if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
					return
				}
			}
		}
	}()

	// reader 循环
	_ = conn.SetReadDeadline(time.Now().Add(readDeadline))
	conn.SetPongHandler(func(string) error {
		_ = conn.SetReadDeadline(time.Now().Add(readDeadline))
		return nil
	})

	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			break
		}
		_ = conn.SetReadDeadline(time.Now().Add(readDeadline))

		var m wsIn
		if err := json.Unmarshal(data, &m); err != nil {
			continue
		}
		switch m.Type {
		case "ping":
			out, _ := json.Marshal(wsOut{Type: "pong"})
			sendBytes(client, out)

		case "send":
			msg, err := clip.CreateMessage(ctx, code, m.Content, m.ContentType)
			if err != nil {
				out, _ := json.Marshal(wsOut{Type: "error", Error: err.Error()})
				sendBytes(client, out)
				continue
			}
			payload := &wsMessagePayload{
				Id:          msg.Id,
				Content:     msg.Content,
				ContentType: msg.ContentType,
				CreatedAt:   msg.CreatedAt,
			}
			out, _ := json.Marshal(wsOut{
				Type:    "message_created",
				Message: payload,
				From:    client.ID,
			})
			// 广播给其他人 + 回显给发送者（带 from=自身，前端可识别）
			hub.Default.Broadcast(code, client, out)
			sendBytes(client, out)

		case "send_file":
			msg, err := clip.CreateFileMessage(ctx, code, m.FileKey, m.FileName, m.FileSize, "file")
			if err != nil {
				out, _ := json.Marshal(wsOut{Type: "error", Error: err.Error()})
				sendBytes(client, out)
				continue
			}
			payload := &wsMessagePayload{
				Id:          msg.Id,
				Content:     msg.Content,
				ContentType: msg.ContentType,
				CreatedAt:   msg.CreatedAt,
				FileName:    msg.FileName,
				FileSize:    msg.FileSize,
				FileKey:     msg.FileKey,
			}
			out, _ := json.Marshal(wsOut{
				Type:    "message_created",
				Message: payload,
				From:    client.ID,
			})
			hub.Default.Broadcast(code, client, out)
			sendBytes(client, out)

		case "delete":
			if err := clip.DeleteMessage(ctx, code, m.Id); err != nil {
				out, _ := json.Marshal(wsOut{Type: "error", Error: err.Error()})
				sendBytes(client, out)
				continue
			}
			out, _ := json.Marshal(wsOut{
				Type: "message_deleted",
				Id:   m.Id,
				From: client.ID,
			})
			hub.Default.Broadcast(code, client, out)
			sendBytes(client, out)
		}
	}

	// 先从 Hub 移除（Leave 持写锁，返回后房间 map 中已无本连接，
	// 其他 goroutine 的 Broadcast 不会再向 client.Send 发送），再关闭 channel，
	// 避免向已关闭的 channel 发送导致 panic。
	hub.Default.Leave(code, client)
	close(client.Send)
	<-writerDone
}
