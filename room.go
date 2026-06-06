package main

import "sync"

type Room struct {
	ID          string
	mu          sync.RWMutex
	clients     map[*Client]bool
	videoURL    string
	videoSource string
	currentTime float64
	playing     bool
}

func NewRoom(id string) *Room {
	return &Room{
		ID:      id,
		clients: make(map[*Client]bool),
	}
}

func (r *Room) AddClient(c *Client) {
	r.mu.Lock()
	r.clients[c] = true
	r.mu.Unlock()
}

func (r *Room) RemoveClient(c *Client) {
	r.mu.Lock()
	delete(r.clients, c)
	r.mu.Unlock()
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

func (r *Room) SetVideo(url, source string) {
	r.mu.Lock()
	r.videoURL = url
	r.videoSource = source
	r.currentTime = 0
	r.playing = false
	r.mu.Unlock()
}

func (r *Room) UpdatePlayState(action string, t float64) {
	r.mu.Lock()
	r.currentTime = t
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
	return RoomStatePayload{
		RoomID:      r.ID,
		VideoURL:    r.videoURL,
		VideoSource: r.videoSource,
		Members:     members,
		CurrentTime: r.currentTime,
		Playing:     r.playing,
	}
}
