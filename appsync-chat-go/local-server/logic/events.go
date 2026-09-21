package logic

import "time"

// TypingEvent / PresenceEvent are never persisted — they're published
// straight to the broker so subscribers get them in real time, mirroring
// the "Subscription / Event" scenarios that don't touch the database.
type TypingEvent struct {
	ChatRoomID string `json:"chatRoomId"`
	Sender     string `json:"sender"`
	IsTyping   bool   `json:"isTyping"`
}

type PresenceEvent struct {
	UserID   string `json:"userId"`
	Online   bool   `json:"online"`
	LastSeen string `json:"lastSeen"`
}

func SendTypingEvent(chatRoomID, sender string, isTyping bool) (*TypingEvent, error) {
	ev := TypingEvent{ChatRoomID: chatRoomID, Sender: sender, IsTyping: isTyping}
	TypingBroker.Publish(chatRoomID, ev)
	return &ev, nil
}

func SetPresence(userID string, online bool) (*PresenceEvent, error) {
	ev := PresenceEvent{
		UserID:   userID,
		Online:   online,
		LastSeen: time.Now().UTC().Format(time.RFC3339Nano),
	}
	PresenceBroker.Publish(userID, ev)
	return &ev, nil
}
