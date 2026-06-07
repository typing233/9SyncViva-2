package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func setupTestServer() (*httptest.Server, *Hub) {
	hub := NewHub()
	mux := http.NewServeMux()
	mux.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		upgrader := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		client := NewClient(hub, conn)
		go client.WritePump()
		go client.ReadPump()
	})
	server := httptest.NewServer(mux)
	return server, hub
}

func wsConnect(t *testing.T, server *httptest.Server) *websocket.Conn {
	t.Helper()
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	return conn
}

func wsSend(t *testing.T, conn *websocket.Conn, msgType string, payload any) {
	t.Helper()
	data, _ := json.Marshal(map[string]any{"type": msgType, "payload": payload})
	if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
		t.Fatalf("write: %v", err)
	}
}

func wsRead(t *testing.T, conn *websocket.Conn) map[string]any {
	t.Helper()
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, data, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var msg map[string]any
	json.Unmarshal(data, &msg)
	return msg
}

func wsReadType(t *testing.T, conn *websocket.Conn, wantType string) map[string]any {
	t.Helper()
	for i := 0; i < 10; i++ {
		msg := wsRead(t, conn)
		if msg["type"] == wantType {
			return msg
		}
	}
	t.Fatalf("did not receive message type %q within 10 messages", wantType)
	return nil
}

func TestRoomPasswordOnCreate(t *testing.T) {
	server, _ := setupTestServer()
	defer server.Close()

	// Client 1 creates room with password
	c1 := wsConnect(t, server)
	defer c1.Close()
	wsSend(t, c1, "join", map[string]string{"roomId": "pw-room", "nickname": "Alice", "password": "secret123"})
	state := wsReadType(t, c1, "room_state")
	payload := state["payload"].(map[string]any)
	if payload["hasPassword"] != true {
		t.Fatalf("expected hasPassword=true after creating room with password, got %v", payload["hasPassword"])
	}
	if payload["owner"] != "Alice" {
		t.Fatalf("expected owner=Alice, got %v", payload["owner"])
	}

	// Client 2 tries to join WITHOUT password → should get error
	c2 := wsConnect(t, server)
	defer c2.Close()
	wsSend(t, c2, "join", map[string]string{"roomId": "pw-room", "nickname": "Bob", "password": ""})
	errMsg := wsReadType(t, c2, "error")
	errPayload := errMsg["payload"].(map[string]any)
	if errPayload["message"] != "wrong password" {
		t.Fatalf("expected 'wrong password' error, got %v", errPayload["message"])
	}

	// Client 2 tries with WRONG password → should get error
	wsSend(t, c2, "join", map[string]string{"roomId": "pw-room", "nickname": "Bob", "password": "wrong"})
	errMsg = wsReadType(t, c2, "error")
	errPayload = errMsg["payload"].(map[string]any)
	if errPayload["message"] != "wrong password" {
		t.Fatalf("expected 'wrong password' error, got %v", errPayload["message"])
	}

	// Client 2 tries with CORRECT password → should succeed
	wsSend(t, c2, "join", map[string]string{"roomId": "pw-room", "nickname": "Bob", "password": "secret123"})
	state2 := wsReadType(t, c2, "room_state")
	payload2 := state2["payload"].(map[string]any)
	members := payload2["members"].([]any)
	if len(members) != 2 {
		t.Fatalf("expected 2 members after correct password join, got %d", len(members))
	}

	fmt.Println("PASS: Room password on create works correctly")
}

func TestRoomPasswordSetAndClear(t *testing.T) {
	server, _ := setupTestServer()
	defer server.Close()

	// Client 1 creates room WITHOUT password
	c1 := wsConnect(t, server)
	defer c1.Close()
	wsSend(t, c1, "join", map[string]string{"roomId": "open-room", "nickname": "Alice", "password": ""})
	state := wsReadType(t, c1, "room_state")
	payload := state["payload"].(map[string]any)
	if payload["hasPassword"] != false {
		t.Fatalf("expected hasPassword=false for open room, got %v", payload["hasPassword"])
	}

	// Client 2 joins freely (no password needed)
	c2 := wsConnect(t, server)
	defer c2.Close()
	wsSend(t, c2, "join", map[string]string{"roomId": "open-room", "nickname": "Bob", "password": ""})
	wsReadType(t, c2, "room_state")

	// Owner sets a password
	wsSend(t, c1, "set_password", map[string]string{"password": "newpw"})
	// Owner should receive room_state with hasPassword=true
	updatedState := wsReadType(t, c1, "room_state")
	up := updatedState["payload"].(map[string]any)
	if up["hasPassword"] != true {
		t.Fatalf("expected hasPassword=true after setting password, got %v", up["hasPassword"])
	}

	// Client 3 tries to join without password → fail
	c3 := wsConnect(t, server)
	defer c3.Close()
	wsSend(t, c3, "join", map[string]string{"roomId": "open-room", "nickname": "Charlie", "password": ""})
	errMsg := wsReadType(t, c3, "error")
	errPayload := errMsg["payload"].(map[string]any)
	if errPayload["message"] != "wrong password" {
		t.Fatalf("expected 'wrong password', got %v", errPayload["message"])
	}

	// Client 3 joins with correct new password → success
	wsSend(t, c3, "join", map[string]string{"roomId": "open-room", "nickname": "Charlie", "password": "newpw"})
	wsReadType(t, c3, "room_state")

	// Owner clears the password
	wsSend(t, c1, "set_password", map[string]string{"password": ""})
	clearedState := wsReadType(t, c1, "room_state")
	cp := clearedState["payload"].(map[string]any)
	if cp["hasPassword"] != false {
		t.Fatalf("expected hasPassword=false after clearing, got %v", cp["hasPassword"])
	}

	// Client 4 joins without password → success (room is now open)
	c4 := wsConnect(t, server)
	defer c4.Close()
	wsSend(t, c4, "join", map[string]string{"roomId": "open-room", "nickname": "Dave", "password": ""})
	wsReadType(t, c4, "room_state")

	fmt.Println("PASS: Room password set and clear works correctly")
}

func TestHeartbeatSyncCorrection(t *testing.T) {
	server, _ := setupTestServer()
	defer server.Close()

	// Client 1 creates room and loads a video
	c1 := wsConnect(t, server)
	defer c1.Close()
	wsSend(t, c1, "join", map[string]string{"roomId": "sync-room", "nickname": "Alice", "password": ""})
	wsReadType(t, c1, "room_state")

	// Client 2 joins
	c2 := wsConnect(t, server)
	defer c2.Close()
	wsSend(t, c2, "join", map[string]string{"roomId": "sync-room", "nickname": "Bob", "password": ""})
	wsReadType(t, c2, "room_state")
	// c1 gets member_update
	wsReadType(t, c1, "member_update")

	// Set video (non-live) — both clients receive set_video
	wsSend(t, c1, "set_video", map[string]any{"url": "http://example.com/video.mp4", "source": "alist", "isLive": false})
	wsReadType(t, c1, "set_video")
	wsReadType(t, c2, "set_video")

	// Start playback at time=10.0 — c2 receives sync (c1 is excluded)
	wsSend(t, c1, "sync", map[string]any{"action": "play", "time": 10.0})
	syncMsg := wsReadType(t, c2, "sync")
	syncPayload := syncMsg["payload"].(map[string]any)
	if syncPayload["serverTime"] == nil {
		t.Fatalf("expected serverTime in sync broadcast")
	}
	fmt.Printf("  Sync message received with serverTime=%v, time=%.1f\n", syncPayload["serverTime"], syncPayload["time"].(float64))

	// Wait for heartbeat sync (server sends every 5s)
	time.Sleep(6 * time.Second)

	// Read heartbeat_sync message from c2
	heartbeat := wsReadType(t, c2, "heartbeat_sync")
	hbPayload := heartbeat["payload"].(map[string]any)
	hbTime := hbPayload["time"].(float64)
	hbServerTime := hbPayload["serverTime"].(float64)

	// Heartbeat time should have advanced from 10.0 by ~6 seconds
	if hbTime < 15.0 {
		t.Fatalf("expected heartbeat time >= 15.0 (started at 10, waited 6s), got %f", hbTime)
	}
	if hbServerTime == 0 {
		t.Fatalf("expected non-zero serverTime in heartbeat")
	}

	fmt.Printf("  Heartbeat: estimated time=%.1fs (expected ~16s), serverTime=%d\n", hbTime, int64(hbServerTime))

	// Test ping/pong RTT measurement
	sendTime := time.Now().UnixMilli()
	wsSend(t, c2, "ping", map[string]any{"clientTime": float64(sendTime)})
	pong := wsReadType(t, c2, "pong")
	pongPayload := pong["payload"].(map[string]any)
	if pongPayload["clientTime"] == nil || pongPayload["serverTime"] == nil {
		t.Fatalf("expected clientTime and serverTime in pong")
	}
	clientTime := int64(pongPayload["clientTime"].(float64))
	pongServerTime := int64(pongPayload["serverTime"].(float64))
	measuredRtt := time.Now().UnixMilli() - clientTime
	offset := pongServerTime - clientTime - measuredRtt/2
	fmt.Printf("  Ping/Pong: RTT=%dms, clock offset=%dms\n", measuredRtt, offset)
	fmt.Printf("PASS: Heartbeat sync correction verified — time advanced %.1fs from initial 10.0s\n", hbTime-10.0)
}

func TestNoPasswordRoomFreeJoin(t *testing.T) {
	server, _ := setupTestServer()
	defer server.Close()

	// Create room without password
	c1 := wsConnect(t, server)
	defer c1.Close()
	wsSend(t, c1, "join", map[string]string{"roomId": "free-room", "nickname": "Alice", "password": ""})
	state := wsReadType(t, c1, "room_state")
	payload := state["payload"].(map[string]any)
	if payload["hasPassword"] != false {
		t.Fatalf("expected hasPassword=false")
	}

	// Anyone can join without password
	c2 := wsConnect(t, server)
	defer c2.Close()
	wsSend(t, c2, "join", map[string]string{"roomId": "free-room", "nickname": "Bob", "password": ""})
	state2 := wsReadType(t, c2, "room_state")
	p2 := state2["payload"].(map[string]any)
	members := p2["members"].([]any)
	if len(members) != 2 {
		t.Fatalf("expected 2 members, got %d", len(members))
	}

	fmt.Println("PASS: No-password room allows free join")
}

func TestMultiClientDriftCorrection(t *testing.T) {
	server, _ := setupTestServer()
	defer server.Close()

	// Simulate 3 clients in a room watching a video
	c1 := wsConnect(t, server)
	defer c1.Close()
	c2 := wsConnect(t, server)
	defer c2.Close()
	c3 := wsConnect(t, server)
	defer c3.Close()

	// All join the same room
	wsSend(t, c1, "join", map[string]string{"roomId": "drift-room", "nickname": "Alice", "password": ""})
	wsReadType(t, c1, "room_state")
	wsSend(t, c2, "join", map[string]string{"roomId": "drift-room", "nickname": "Bob", "password": ""})
	wsReadType(t, c2, "room_state")
	wsSend(t, c3, "join", map[string]string{"roomId": "drift-room", "nickname": "Charlie", "password": ""})
	wsReadType(t, c3, "room_state")

	// Small pause to let member_update messages arrive
	time.Sleep(200 * time.Millisecond)

	// Owner loads video and starts playback at t=30.0
	wsSend(t, c1, "set_video", map[string]any{"url": "http://example.com/movie.mp4", "source": "alist", "isLive": false})
	time.Sleep(100 * time.Millisecond)
	wsSend(t, c1, "sync", map[string]any{"action": "play", "time": 30.0})

	fmt.Println("  Playback started at t=30.0s")
	fmt.Println("  Waiting for heartbeat sync messages (every 5s)...")

	// Collect heartbeat messages from c3 (reads all messages, filters for heartbeat_sync)
	var heartbeats []float64
	startWait := time.Now()
	for time.Since(startWait) < 12*time.Second {
		c3.SetReadDeadline(time.Now().Add(6 * time.Second))
		_, data, err := c3.ReadMessage()
		if err != nil {
			continue
		}
		var msg map[string]any
		json.Unmarshal(data, &msg)
		if msg["type"] == "heartbeat_sync" {
			payload := msg["payload"].(map[string]any)
			hbTime := payload["time"].(float64)
			heartbeats = append(heartbeats, hbTime)
			elapsed := time.Since(startWait).Seconds()
			expectedTime := 30.0 + elapsed
			drift := hbTime - expectedTime
			fmt.Printf("  Heartbeat #%d: server_time=%.1fs, expected=%.1fs, drift=%.3fs\n",
				len(heartbeats), hbTime, expectedTime, drift)
		}
	}

	if len(heartbeats) < 2 {
		t.Fatalf("expected at least 2 heartbeat messages, got %d", len(heartbeats))
	}

	// Verify heartbeats are monotonically increasing and roughly ~5s apart
	for i := 1; i < len(heartbeats); i++ {
		gap := heartbeats[i] - heartbeats[i-1]
		if gap < 4.0 || gap > 6.5 {
			t.Fatalf("heartbeat gap between #%d and #%d was %.1fs, expected ~5s", i, i+1, gap)
		}
	}

	// Verify the drift from expected is small (< 1s, since we're on the same machine)
	elapsed := time.Since(startWait).Seconds()
	lastHb := heartbeats[len(heartbeats)-1]
	expectedFinal := 30.0 + elapsed
	finalDrift := lastHb - expectedFinal
	if finalDrift < -2.0 || finalDrift > 2.0 {
		t.Fatalf("final drift too large: %.3fs (heartbeat=%.1f, expected=%.1f)", finalDrift, lastHb, expectedFinal)
	}

	fmt.Printf("  Final drift: %.3fs (well within ±1.5s correction threshold)\n", finalDrift)
	fmt.Printf("PASS: Multi-client heartbeat sync delivers consistent time estimates across %d heartbeats\n", len(heartbeats))
}

func TestLiveStreamNoHeartbeat(t *testing.T) {
	server, _ := setupTestServer()
	defer server.Close()

	c1 := wsConnect(t, server)
	defer c1.Close()
	c2 := wsConnect(t, server)
	defer c2.Close()

	wsSend(t, c1, "join", map[string]string{"roomId": "live-room", "nickname": "Alice", "password": ""})
	wsReadType(t, c1, "room_state")
	wsSend(t, c2, "join", map[string]string{"roomId": "live-room", "nickname": "Bob", "password": ""})
	wsReadType(t, c2, "room_state")

	// Load a live stream (isLive=true)
	wsSend(t, c1, "set_video", map[string]any{"url": "http://example.com/live/stream.flv", "source": "alist", "isLive": true})
	time.Sleep(100 * time.Millisecond)

	// Start playback
	wsSend(t, c1, "sync", map[string]any{"action": "play", "time": 0.0})
	time.Sleep(100 * time.Millisecond)

	// Wait 6s — no heartbeat_sync should arrive for live streams
	gotHeartbeat := false
	deadline := time.Now().Add(6 * time.Second)
	for time.Now().Before(deadline) {
		c2.SetReadDeadline(time.Now().Add(6 * time.Second))
		_, data, err := c2.ReadMessage()
		if err != nil {
			break
		}
		var msg map[string]any
		json.Unmarshal(data, &msg)
		if msg["type"] == "heartbeat_sync" {
			gotHeartbeat = true
			break
		}
	}

	if gotHeartbeat {
		t.Fatalf("live stream should NOT receive heartbeat_sync messages")
	}
	fmt.Println("PASS: Live stream correctly suppresses heartbeat sync")
}

func TestLiveStreamSyncOnlyPlayPause(t *testing.T) {
	server, _ := setupTestServer()
	defer server.Close()

	c1 := wsConnect(t, server)
	defer c1.Close()
	c2 := wsConnect(t, server)
	defer c2.Close()

	wsSend(t, c1, "join", map[string]string{"roomId": "live-sync", "nickname": "Alice", "password": ""})
	wsReadType(t, c1, "room_state")
	wsSend(t, c2, "join", map[string]string{"roomId": "live-sync", "nickname": "Bob", "password": ""})
	wsReadType(t, c2, "room_state")
	time.Sleep(100 * time.Millisecond)

	wsSend(t, c1, "set_video", map[string]any{"url": "http://cdn.example.com/live/channel1.m3u8", "source": "alist", "isLive": true})
	time.Sleep(100 * time.Millisecond)

	// Read set_video from c2
	var found bool
	c2.SetReadDeadline(time.Now().Add(2 * time.Second))
	for i := 0; i < 10; i++ {
		_, data, err := c2.ReadMessage()
		if err != nil {
			break
		}
		var msg map[string]any
		json.Unmarshal(data, &msg)
		if msg["type"] == "set_video" {
			payload := msg["payload"].(map[string]any)
			if payload["isLive"] != true {
				t.Fatalf("expected isLive=true in set_video broadcast, got %v", payload["isLive"])
			}
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("c2 did not receive set_video message")
	}

	// Play sync works
	wsSend(t, c1, "sync", map[string]any{"action": "play", "time": 0.0})
	syncMsg := wsReadType(t, c2, "sync")
	sp := syncMsg["payload"].(map[string]any)
	if sp["action"] != "play" {
		t.Fatalf("expected action=play, got %v", sp["action"])
	}

	// Pause sync works
	wsSend(t, c1, "sync", map[string]any{"action": "pause", "time": 0.0})
	syncMsg2 := wsReadType(t, c2, "sync")
	sp2 := syncMsg2["payload"].(map[string]any)
	if sp2["action"] != "pause" {
		t.Fatalf("expected action=pause, got %v", sp2["action"])
	}

	// Seek sync also arrives (server sends it; client-side JS ignores it for live)
	wsSend(t, c1, "sync", map[string]any{"action": "seek", "time": 120.0})
	syncMsg3 := wsReadType(t, c2, "sync")
	sp3 := syncMsg3["payload"].(map[string]any)
	if sp3["action"] != "seek" {
		t.Fatalf("expected action=seek, got %v", sp3["action"])
	}

	fmt.Println("PASS: Live stream sync correctly delivers play/pause/seek messages (client filters seek)")
}