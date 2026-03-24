package client

import (
	"encoding/json"
	"log"
	"time"

	"github.com/gorilla/websocket"
)

// RawEvent matches the sidecar's WebSocket envelope.
// The renderer treats these as immutable facts: it never initiates state changes.
type RawEvent struct {
	Type      string          `json:"type"`
	Timestamp string          `json:"timestamp"`
	Payload   json.RawMessage `json:"payload"`
}

// Connect dials the sidecar WebSocket and streams events into ch.
// It retries with a small capped backoff so the renderer can survive
// sidecar restarts without turning the app into a crash loop.
func Connect(wsURL string, ch chan<- RawEvent) {
	backoff := time.Second
	const maxBackoff = 10 * time.Second

	for {
		conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
		if err != nil {
			log.Printf("castle client: ws dial failed: %v", err)
			time.Sleep(backoff)
			if backoff < maxBackoff {
				backoff *= 2
				if backoff > maxBackoff {
					backoff = maxBackoff
				}
			}
			continue
		}

		backoff = time.Second
		for {
			var ev RawEvent
			if err := conn.ReadJSON(&ev); err != nil {
				_ = conn.Close()
				log.Printf("castle client: ws read failed: %v", err)
				break
			}
			select {
			case ch <- ev:
			default:
				// Renderer is behind; drop instead of blocking the WS reader.
			}
		}
		time.Sleep(backoff)
	}
}

