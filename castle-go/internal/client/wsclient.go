// Package client handles the WebSocket connection from the castle renderer to PicoClaw.
// It runs in a background goroutine and sends parsed events to the game loop
// via a buffered channel. Connection drops trigger automatic reconnection with
// exponential backoff — the game loop never blocks on a dead connection.
package client

import (
	"encoding/json"
	"log"
	"time"

	"github.com/gorilla/websocket"
)

// RawEvent is a parsed WebSocket envelope from PicoClaw.
// The game loop switches on Type and unmarshals Payload into the appropriate struct.
type RawEvent struct {
	Type      string
	Timestamp string
	Payload   json.RawMessage
}

// Connect dials the PicoClaw WebSocket and forwards events to ch.
// Runs until ctx is cancelled or the process exits.
// Reconnects automatically on any error with exponential backoff (1s → 30s).
func Connect(wsURL string, ch chan<- RawEvent) {
	backoff := time.Second
	maxBackoff := 30 * time.Second

	for {
		if err := runConnection(wsURL, ch); err != nil {
			log.Printf("castle/client: disconnected (%v), reconnecting in %v", err, backoff)
		} else {
			log.Printf("castle/client: connection closed, reconnecting in %v", backoff)
		}

		time.Sleep(backoff)
		backoff *= 2
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
	}
}

func runConnection(wsURL string, ch chan<- RawEvent) error {
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		return err
	}
	defer conn.Close()

	// Reset backoff on successful connection
	log.Printf("castle/client: connected to %s", wsURL)

	// Wire-level envelope matches sidecar-go/internal/ws/hub.go Envelope struct
	type wireEnvelope struct {
		Type      string          `json:"type"`
		Timestamp string          `json:"timestamp"`
		Payload   json.RawMessage `json:"payload"`
	}

	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			return err
		}

		var env wireEnvelope
		if err := json.Unmarshal(msg, &env); err != nil {
			log.Printf("castle/client: unmarshal envelope: %v", err)
			continue
		}

		ev := RawEvent{
			Type:      env.Type,
			Timestamp: env.Timestamp,
			Payload:   env.Payload,
		}

		// Non-blocking send — drop the event if the game loop is not keeping up
		// rather than blocking the read loop and causing the connection to time out.
		select {
		case ch <- ev:
		default:
			log.Printf("castle/client: event channel full, dropping %s", env.Type)
		}
	}
}
