package main

import (
	"encoding/json"
	"log"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const (
	writeWait      = 10 * time.Second
	pongWait       = 60 * time.Second
	pingPeriod     = (pongWait * 9) / 10
	maxMessageSize = 8192
)

type Client struct {
	hub      *Hub
	conn     *websocket.Conn
	send     chan map[string]any
	room     *Room
	nickname string
	mu       sync.Mutex
}

func NewClient(hub *Hub, conn *websocket.Conn) *Client {
	return &Client{
		hub:  hub,
		conn: conn,
		send: make(chan map[string]any, 64),
	}
}

func (c *Client) Send(msg map[string]any) {
	select {
	case c.send <- msg:
	default:
	}
}

func (c *Client) ReadPump() {
	defer func() {
		c.leaveRoom()
		c.conn.Close()
	}()

	c.conn.SetReadLimit(maxMessageSize)
	c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			break
		}
		c.handleMessage(message)
	}
}

func (c *Client) WritePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()

	for {
		select {
		case msg, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			data, err := json.Marshal(msg)
			if err != nil {
				continue
			}
			if err := c.conn.WriteMessage(websocket.TextMessage, data); err != nil {
				return
			}
		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

func (c *Client) handleMessage(raw []byte) {
	var env struct {
		Type    MessageType     `json:"type"`
		Payload json.RawMessage `json:"payload"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		c.Send(errorMsg("invalid message format"))
		return
	}

	switch env.Type {
	case TypeJoin:
		var p JoinPayload
		if err := json.Unmarshal(env.Payload, &p); err != nil || p.RoomID == "" || p.Nickname == "" {
			c.Send(errorMsg("invalid join payload"))
			return
		}
		c.joinRoom(p)

	case TypeSetVideo:
		if c.room == nil {
			c.Send(errorMsg("not in a room"))
			return
		}
		if !c.room.IsOwner(c) {
			c.Send(errorMsg("only room owner can change video"))
			return
		}
		var p SetVideoPayload
		if err := json.Unmarshal(env.Payload, &p); err != nil || p.URL == "" {
			c.Send(errorMsg("invalid set_video payload"))
			return
		}
		c.room.SetVideo(p.URL, p.Source, p.IsLive)
		c.room.BroadcastAll(newEnvelope(TypeSetVideo, p))

	case TypeSync:
		if c.room == nil {
			c.Send(errorMsg("not in a room"))
			return
		}
		if c.room.GetMode() == "strict" && !c.room.IsOwner(c) {
			c.Send(errorMsg("strict mode: only owner can control playback"))
			return
		}
		var p SyncPayload
		if err := json.Unmarshal(env.Payload, &p); err != nil {
			c.Send(errorMsg("invalid sync payload"))
			return
		}
		c.room.UpdatePlayState(p.Action, p.Time)
		c.room.Broadcast(newEnvelope(TypeSync, map[string]any{
			"action":     p.Action,
			"time":       p.Time,
			"from":       c.nickname,
			"serverTime": time.Now().UnixMilli(),
		}), c)

	case TypeChat:
		if c.room == nil {
			c.Send(errorMsg("not in a room"))
			return
		}
		var p ChatPayload
		if err := json.Unmarshal(env.Payload, &p); err != nil || p.Message == "" {
			return
		}
		c.room.BroadcastAll(chatBroadcastMsg(c.nickname, p.Message))

	case TypeDanmaku:
		if c.room == nil {
			c.Send(errorMsg("not in a room"))
			return
		}
		var p DanmakuPayload
		if err := json.Unmarshal(env.Payload, &p); err != nil || p.Text == "" {
			return
		}
		if len([]rune(p.Text)) > 100 {
			p.Text = string([]rune(p.Text)[:100])
		}
		c.room.BroadcastAll(newEnvelope(TypeDanmaku, DanmakuBroadcast{
			Text:     p.Text,
			Color:    p.Color,
			Position: p.Position,
			Nickname: c.nickname,
		}))

	case TypePing:
		var p PingPayload
		if err := json.Unmarshal(env.Payload, &p); err != nil {
			return
		}
		c.Send(newEnvelope(TypePong, PongPayload{
			ClientTime: p.ClientTime,
			ServerTime: time.Now().UnixMilli(),
		}))

	case TypeSetPassword:
		if c.room == nil || !c.room.IsOwner(c) {
			c.Send(errorMsg("only room owner can set password"))
			return
		}
		var p SetPasswordPayload
		if err := json.Unmarshal(env.Payload, &p); err != nil {
			return
		}
		c.room.SetPassword(p.Password)
		c.room.BroadcastAll(newEnvelope(TypeRoomState, c.room.State()))

	case TypeKick:
		if c.room == nil || !c.room.IsOwner(c) {
			c.Send(errorMsg("only room owner can kick"))
			return
		}
		var p KickPayload
		if err := json.Unmarshal(env.Payload, &p); err != nil || p.Nickname == "" {
			return
		}
		target := c.room.FindClientByNickname(p.Nickname)
		if target == nil || target == c {
			return
		}
		target.Send(errorMsg("you have been kicked from the room"))
		go func() {
			time.Sleep(100 * time.Millisecond)
			target.conn.Close()
		}()

	case TypeTransferOwner:
		if c.room == nil || !c.room.IsOwner(c) {
			c.Send(errorMsg("only room owner can transfer ownership"))
			return
		}
		var p TransferOwnerPayload
		if err := json.Unmarshal(env.Payload, &p); err != nil || p.Nickname == "" {
			return
		}
		target := c.room.FindClientByNickname(p.Nickname)
		if target == nil || target == c {
			return
		}
		c.room.SetOwner(target)
		c.room.BroadcastAll(newEnvelope(TypeRoomState, c.room.State()))

	case TypeRoomConfig:
		if c.room == nil || !c.room.IsOwner(c) {
			c.Send(errorMsg("only room owner can change config"))
			return
		}
		var p RoomConfigPayload
		if err := json.Unmarshal(env.Payload, &p); err != nil {
			return
		}
		if p.Mode == "free" || p.Mode == "strict" {
			c.room.SetMode(p.Mode)
			c.room.BroadcastAll(newEnvelope(TypeRoomConfig, map[string]any{
				"mode": p.Mode,
			}))
		}

	default:
		c.Send(errorMsg("unknown message type"))
	}
}

func (c *Client) joinRoom(p JoinPayload) {
	c.leaveRoom()

	room := c.hub.GetOrCreateRoom(p.RoomID)

	if !room.CheckPassword(p.Password) {
		c.Send(errorMsg("wrong password"))
		return
	}

	c.mu.Lock()
	c.nickname = p.Nickname
	c.mu.Unlock()

	room.AddClient(c)
	c.room = room

	room.mu.Lock()
	if room.owner == nil {
		room.owner = c
		if p.Password != "" {
			room.password = p.Password
		}
	}
	room.mu.Unlock()

	c.Send(newEnvelope(TypeRoomState, room.State()))

	room.BroadcastAll(newEnvelope(TypeMemberUpdate, map[string]any{
		"members": room.Members(),
		"owner":   room.OwnerNickname(),
	}))

	log.Printf("[%s] %s joined room %s (%d members)", p.RoomID, p.Nickname, p.RoomID, room.ClientCount())
}

func (c *Client) leaveRoom() {
	if c.room == nil {
		return
	}
	room := c.room
	room.RemoveClient(c)
	c.room = nil

	room.BroadcastAll(newEnvelope(TypeMemberUpdate, map[string]any{
		"members": room.Members(),
		"owner":   room.OwnerNickname(),
	}))

	c.hub.RemoveRoomIfEmpty(room.ID)
	log.Printf("[%s] %s left room", room.ID, c.nickname)
}
