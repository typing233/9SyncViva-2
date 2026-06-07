package main

import (
	"sync"
	"time"
)

type Room struct {
	ID            string
	mu            sync.RWMutex
	clients       map[*Client]bool
	videoURL      string
	videoSource   string
	currentTime   float64
	playing       bool
	lastSyncAt    time.Time
	owner         *Client
	password      string
	mode          string // "free" or "strict"
	isLive        bool
	stopHeartbeat chan struct{}
}

func NewRoom(id string) *Room {
	return &Room{
		ID:      id,
		clients: make(map[*Client]bool),
		mode:    "free",
	}
}

func (r *Room) AddClient(c *Client) {
	r.mu.Lock()
	r.clients[c] = true
	needHeartbeat := len(r.clients) == 1 && r.stopHeartbeat == nil
	r.mu.Unlock()
	if needHeartbeat {
		r.StartHeartbeat()
	}
}

func (r *Room) RemoveClient(c *Client) {
	r.mu.Lock()
	delete(r.clients, c)
	empty := len(r.clients) == 0
	if r.owner == c {
		r.owner = nil
		for other := range r.clients {
			r.owner = other
			break
		}
	}
	r.mu.Unlock()
	if empty {
		r.StopHeartbeat()
	}
}

func (r *Room) ClientCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.clients)
}

func (r *Room) Members() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.clients))
	for c := range r.clients {
		names = append(names, c.nickname)
	}
	return names
}

func (r *Room) Broadcast(msg map[string]any, exclude *Client) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for c := range r.clients {
		if c != exclude {
			c.Send(msg)
		}
	}
}

func (r *Room) BroadcastAll(msg map[string]any) {
	r.Broadcast(msg, nil)
}

func (r *Room) SetVideo(url, source string, isLive bool) {
	r.mu.Lock()
	r.videoURL = url
	r.videoSource = source
	r.isLive = isLive
	r.currentTime = 0
	r.playing = false
	r.lastSyncAt = time.Now()
	r.mu.Unlock()
}

func (r *Room) UpdatePlayState(action string, t float64) {
	r.mu.Lock()
	r.currentTime = t
	r.lastSyncAt = time.Now()
	switch action {
	case "play":
		r.playing = true
	case "pause":
		r.playing = false
	case "seek":
		// playing state unchanged
	}
	r.mu.Unlock()
}

func (r *Room) State() RoomStatePayload {
	r.mu.RLock()
	defer r.mu.RUnlock()
	members := make([]string, 0, len(r.clients))
	for c := range r.clients {
		members = append(members, c.nickname)
	}
	ownerNick := ""
	if r.owner != nil {
		ownerNick = r.owner.nickname
	}
	ct := r.currentTime
	if r.playing && !r.lastSyncAt.IsZero() {
		ct += time.Since(r.lastSyncAt).Seconds()
	}
	return RoomStatePayload{
		RoomID:      r.ID,
		VideoURL:    r.videoURL,
		VideoSource: r.videoSource,
		Members:     members,
		CurrentTime: ct,
		Playing:     r.playing,
		Owner:       ownerNick,
		HasPassword: r.password != "",
		Mode:        r.mode,
		IsLive:      r.isLive,
	}
}

func (r *Room) SetOwner(c *Client) {
	r.mu.Lock()
	r.owner = c
	r.mu.Unlock()
}

func (r *Room) IsOwner(c *Client) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.owner == c
}

func (r *Room) OwnerNickname() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.owner != nil {
		return r.owner.nickname
	}
	return ""
}

func (r *Room) SetPassword(pw string) {
	r.mu.Lock()
	r.password = pw
	r.mu.Unlock()
}

func (r *Room) CheckPassword(pw string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.password == "" || r.password == pw
}

func (r *Room) SetMode(mode string) {
	r.mu.Lock()
	r.mode = mode
	r.mu.Unlock()
}

func (r *Room) GetMode() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.mode
}

func (r *Room) FindClientByNickname(nick string) *Client {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for c := range r.clients {
		if c.nickname == nick {
			return c
		}
	}
	return nil
}

func (r *Room) StartHeartbeat() {
	r.mu.Lock()
	if r.stopHeartbeat != nil {
		r.mu.Unlock()
		return
	}
	r.stopHeartbeat = make(chan struct{})
	stop := r.stopHeartbeat
	r.mu.Unlock()

	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				r.mu.RLock()
				if r.playing && !r.isLive && r.videoURL != "" {
					ct := r.currentTime
					if !r.lastSyncAt.IsZero() {
						ct += time.Since(r.lastSyncAt).Seconds()
					}
					msg := newEnvelope(TypeHeartbeatSync, map[string]any{
						"time":       ct,
						"playing":    r.playing,
						"serverTime": time.Now().UnixMilli(),
					})
					for c := range r.clients {
						c.Send(msg)
					}
				}
				r.mu.RUnlock()
			case <-stop:
				return
			}
		}
	}()
}

func (r *Room) StopHeartbeat() {
	r.mu.Lock()
	if r.stopHeartbeat != nil {
		close(r.stopHeartbeat)
		r.stopHeartbeat = nil
	}
	r.mu.Unlock()
}
