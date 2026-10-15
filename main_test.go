package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestIsDarkModeSupportsOvernightWindow(t *testing.T) {
	location := time.FixedZone("test", 8*60*60)

	if isDarkMode(time.Date(2026, 10, 15, 12, 0, 0, 0, location), "22:00:00", "06:00:00") {
		t.Fatal("12:00 should be daytime for an overnight 22:00-06:00 window")
	}
	if !isDarkMode(time.Date(2026, 10, 15, 23, 0, 0, 0, location), "22:00:00", "06:00:00") {
		t.Fatal("23:00 should be dark for an overnight 22:00-06:00 window")
	}
	if !isDarkMode(time.Date(2026, 10, 15, 5, 0, 0, 0, location), "22:00:00", "06:00:00") {
		t.Fatal("05:00 should be dark for an overnight 22:00-06:00 window")
	}
}

func TestAutoUpdatePushRejectsNegativeIntervals(t *testing.T) {
	if _, enabled := autoUpdatePushDuration(-1); enabled {
		t.Fatal("negative autoUpdatePush must disable automatic WebSocket updates")
	}
}

func TestWebSocketWriteTimesOutForSlowClient(t *testing.T) {
	originalTimeout := websocketWriteTimeout
	websocketWriteTimeout = 50 * time.Millisecond
	defer func() { websocketWriteTimeout = originalTimeout }()

	result := make(chan error, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upgrader := websocket.Upgrader{}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			result <- err
			return
		}
		defer conn.Close()
		message := []byte(strings.Repeat("x", 1<<20))
		for i := 0; i < 128; i++ {
			if err := writeWebSocketMessage(conn, websocket.TextMessage, message); err != nil {
				result <- err
				return
			}
		}
		result <- nil
	}))
	defer server.Close()

	client, _, err := websocket.DefaultDialer.Dial(strings.Replace(server.URL, "http", "ws", 1), nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	defer client.Close()

	select {
	case err := <-result:
		if err == nil {
			t.Fatal("a blocked WebSocket write must return an error")
		}
	case <-time.After(time.Second):
		t.Fatal("WebSocket write did not time out")
	}
}
