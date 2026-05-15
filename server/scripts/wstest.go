package main

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"time"

	"github.com/gorilla/websocket"
)

// 简易 WebSocket 实时同步测试：
// 两个客户端连接同一房间，A 发送 update 消息，B 应在 1s 内收到相同内容。
func main() {
	if len(os.Args) < 3 {
		fmt.Println("usage: go run wstest.go <code> <token>")
		os.Exit(1)
	}
	code := os.Args[1]
	token := os.Args[2]

	u := url.URL{Scheme: "ws", Host: "localhost:8080", Path: "/api/ws/" + code, RawQuery: "token=" + token}

	a, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		fmt.Println("A dial err:", err)
		os.Exit(1)
	}
	defer a.Close()

	b, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		fmt.Println("B dial err:", err)
		os.Exit(1)
	}
	defer b.Close()

	received := make(chan map[string]any, 4)
	go func() {
		for {
			_, data, err := b.ReadMessage()
			if err != nil {
				return
			}
			var m map[string]any
			_ = json.Unmarshal(data, &m)
			received <- m
		}
	}()

	time.Sleep(150 * time.Millisecond)

	payload := map[string]any{"type": "update", "content": "hello-from-A", "contentType": "text"}
	raw, _ := json.Marshal(payload)
	if err := a.WriteMessage(websocket.TextMessage, raw); err != nil {
		fmt.Println("A write err:", err)
		os.Exit(1)
	}

	deadline := time.After(2 * time.Second)
	for {
		select {
		case m := <-received:
			if m["type"] == "update" && m["content"] == "hello-from-A" {
				fmt.Println("OK: B received update from A, content=", m["content"])
				return
			}
		case <-deadline:
			fmt.Println("FAIL: B did not receive update within 2s")
			os.Exit(2)
		}
	}
}
