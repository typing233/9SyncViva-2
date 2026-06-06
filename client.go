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
	maxMessageSize = 4096
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
		// drop message if buffer full
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
		Type    MessageType    `json:"type"`
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
		var p SetVideoPayload
		if err := json.Unmarshal(env.Payload, &p); err != nil || p.URL == "" {
			c.Send(errorMsg("invalid set_video payload"))
			return
		}
		c.room.SetVideo(p.URL, p.Source)
		c.room.BroadcastAll(newEnvelope(TypeSetVideo, p))

	case TypeSync:
		if c.room == nil {
			c.Send(errorMsg("not in a room"))
			return
		}
		var p SyncPayload
		if err := json.Unmarshal(env.Payload, &p); err != nil {
			c.Send(errorMsg("invalid sync payload"))
			return
		}
		c.room.UpdatePlayState(p.Action, p.Time)
		c.room.Broadcast(newEnvelope(TypeSync, map[string]any{
			"action": p.Action,
			"time":   p.Time,
			"from":   c.nickname,
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

	default:
		c.Send(errorMsg("unknown message type"))
	}
}

func (c *Client) joinRoom(p JoinPayload) {
	c.leaveRoom()

	c.mu.Lock()
	c.nickname = p.Nickname
	c.mu.Unlock()

	room := c.hub.GetOrCreateRoom(p.RoomID)
	room.AddClient(c)
	c.room = room

	c.Send(newEnvelope(TypeRoomState, room.State()))

	room.Broadcast(newEnvelope(TypeMemberUpdate, map[string]any{
		"members": room.Members(),
	}), c)

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
	}))

	c.hub.RemoveRoomIfEmpty(room.ID)
	log.Printf("[%s] %s left room", room.ID, c.nickname)
}
