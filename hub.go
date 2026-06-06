package main

import "sync"

type Hub struct {
	mu    sync.RWMutex
	rooms map[string]*Room
}

func NewHub() *Hub {
	return &Hub{
		rooms: make(map[string]*Room),
	}
}

func (h *Hub) GetOrCreateRoom(id string) *Room {
	h.mu.Lock()
	defer h.mu.Unlock()
	if r, ok := h.rooms[id]; ok {
		return r
	}
	r := NewRoom(id)
	h.rooms[id] = r
	return r
}

func (h *Hub) RemoveRoomIfEmpty(id string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if r, ok := h.rooms[id]; ok {
		if r.ClientCount() == 0 {
			delete(h.rooms, id)
		}
	}
}

func (h *Hub) RoomCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.rooms)
}
