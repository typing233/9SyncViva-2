package main

import "time"

type MessageType string

const (
	TypeJoin         MessageType = "join"
	TypeSetVideo     MessageType = "set_video"
	TypeSync         MessageType = "sync"
	TypeChat         MessageType = "chat"
	TypeRoomState    MessageType = "room_state"
	TypeMemberUpdate MessageType = "member_update"
	TypeError        MessageType = "error"
)

type Envelope struct {
	Type    MessageType    `json:"type"`
	Payload map[string]any `json:"payload,omitempty"`
}

type JoinPayload struct {
	RoomID   string `json:"roomId"`
	Nickname string `json:"nickname"`
}

type SetVideoPayload struct {
	URL    string `json:"url"`
	Source string `json:"source"` // "alist" or "bilibili"
}

type SyncPayload struct {
	Action string  `json:"action"` // "play", "pause", "seek"
	Time   float64 `json:"time"`
}

type ChatPayload struct {
	Message string `json:"message"`
}

type RoomStatePayload struct {
	RoomID      string   `json:"roomId"`
	VideoURL    string   `json:"videoUrl"`
	VideoSource string   `json:"videoSource"`
	Members     []string `json:"members"`
	CurrentTime float64  `json:"currentTime"`
	Playing     bool     `json:"playing"`
}

type ChatBroadcast struct {
	Nickname string `json:"nickname"`
	Message  string `json:"message"`
	Ts       int64  `json:"ts"`
}

func newEnvelope(t MessageType, payload any) map[string]any {
	m := map[string]any{"type": t}
	if payload != nil {
		m["payload"] = payload
	}
	return m
}

func errorMsg(msg string) map[string]any {
	return newEnvelope(TypeError, map[string]string{"message": msg})
}

func chatBroadcastMsg(nickname, message string) map[string]any {
	return newEnvelope(TypeChat, ChatBroadcast{
		Nickname: nickname,
		Message:  message,
		Ts:       time.Now().UnixMilli(),
	})
}
