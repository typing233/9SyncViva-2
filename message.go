package main

import "time"

type MessageType string

const (
	TypeJoin          MessageType = "join"
	TypeSetVideo      MessageType = "set_video"
	TypeSync          MessageType = "sync"
	TypeChat          MessageType = "chat"
	TypeDanmaku       MessageType = "danmaku"
	TypeRoomState     MessageType = "room_state"
	TypeMemberUpdate  MessageType = "member_update"
	TypePing          MessageType = "ping"
	TypePong          MessageType = "pong"
	TypeHeartbeatSync MessageType = "heartbeat_sync"
	TypeSetPassword   MessageType = "set_password"
	TypeKick          MessageType = "kick"
	TypeTransferOwner MessageType = "transfer_owner"
	TypeRoomConfig    MessageType = "room_config"
	TypeError         MessageType = "error"
)

type Envelope struct {
	Type    MessageType    `json:"type"`
	Payload map[string]any `json:"payload,omitempty"`
}

type JoinPayload struct {
	RoomID   string `json:"roomId"`
	Nickname string `json:"nickname"`
	Password string `json:"password"`
}

type SetVideoPayload struct {
	URL    string `json:"url"`
	Source string `json:"source"`
	IsLive bool   `json:"isLive"`
}

type SyncPayload struct {
	Action string  `json:"action"`
	Time   float64 `json:"time"`
}

type ChatPayload struct {
	Message string `json:"message"`
}

type DanmakuPayload struct {
	Text     string `json:"text"`
	Color    string `json:"color"`
	Position string `json:"position"`
}

type PingPayload struct {
	ClientTime int64 `json:"clientTime"`
}

type PongPayload struct {
	ClientTime int64 `json:"clientTime"`
	ServerTime int64 `json:"serverTime"`
}

type SetPasswordPayload struct {
	Password string `json:"password"`
}

type KickPayload struct {
	Nickname string `json:"nickname"`
}

type TransferOwnerPayload struct {
	Nickname string `json:"nickname"`
}

type RoomConfigPayload struct {
	Mode string `json:"mode"`
}

type RoomStatePayload struct {
	RoomID      string   `json:"roomId"`
	VideoURL    string   `json:"videoUrl"`
	VideoSource string   `json:"videoSource"`
	Members     []string `json:"members"`
	CurrentTime float64  `json:"currentTime"`
	Playing     bool     `json:"playing"`
	Owner       string   `json:"owner"`
	HasPassword bool     `json:"hasPassword"`
	Mode        string   `json:"mode"`
	IsLive      bool     `json:"isLive"`
}

type ChatBroadcast struct {
	Nickname string `json:"nickname"`
	Message  string `json:"message"`
	Ts       int64  `json:"ts"`
}

type DanmakuBroadcast struct {
	Text     string `json:"text"`
	Color    string `json:"color"`
	Position string `json:"position"`
	Nickname string `json:"nickname"`
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
