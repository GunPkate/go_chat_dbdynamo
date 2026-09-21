// These tests exercise the resolver functions directly (no Lambda, no
// AppSync) against DynamoDB Local, so the whole suite runs without an
// AWS account. Start DynamoDB Local and create the tables first — either
// via `docker compose up dynamodb-local create-tables` (see
// docker-compose.yml) or manually — then run:
//
//	DYNAMODB_LOCAL_ENDPOINT=http://localhost:8000 \
//	AWS_REGION=us-east-1 \
//	MESSAGES_TABLE=ChatMessages \
//	CONVERSATIONS_TABLE=UserConversations \
//	IDEMPOTENCY_TABLE=ChatIdempotency \
//	go test ./... -v
package main

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

func TestSendMessageAndGetMessages(t *testing.T) {
	ctx := context.Background()
	roomID := "test-room-" + uuid.NewString()

	msg, err := sendMessage(ctx, map[string]interface{}{
		"chatRoomId":  roomID,
		"sender":      "alice",
		"content":     "hello",
		"clientToken": uuid.NewString(),
	})
	if err != nil {
		t.Fatalf("sendMessage failed: %v", err)
	}
	if msg.Content != "hello" {
		t.Errorf("expected content %q, got %q", "hello", msg.Content)
	}

	messages, err := getMessages(ctx, map[string]interface{}{"chatRoomId": roomID})
	if err != nil {
		t.Fatalf("getMessages failed: %v", err)
	}
	if len(messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(messages))
	}
}

func TestGetMessagesSince(t *testing.T) {
	ctx := context.Background()
	roomID := "test-room-" + uuid.NewString()

	first, err := sendMessage(ctx, map[string]interface{}{
		"chatRoomId":  roomID,
		"sender":      "alice",
		"content":     "before reconnect",
		"clientToken": uuid.NewString(),
	})
	if err != nil {
		t.Fatalf("sendMessage failed: %v", err)
	}

	if _, err := sendMessage(ctx, map[string]interface{}{
		"chatRoomId":  roomID,
		"sender":      "bob",
		"content":     "missed while offline",
		"clientToken": uuid.NewString(),
	}); err != nil {
		t.Fatalf("second sendMessage failed: %v", err)
	}

	// Simulates B reconnecting and asking only for what it missed.
	missed, err := getMessages(ctx, map[string]interface{}{
		"chatRoomId": roomID,
		"since":      first.CreatedAt,
	})
	if err != nil {
		t.Fatalf("getMessages with since failed: %v", err)
	}
	if len(missed) != 1 || missed[0].Content != "missed while offline" {
		t.Fatalf("expected only the message after 'since', got %+v", missed)
	}
}

func TestDuplicateSendIsIdempotent(t *testing.T) {
	ctx := context.Background()
	roomID := "test-room-" + uuid.NewString()
	token := uuid.NewString()

	args := map[string]interface{}{
		"chatRoomId":  roomID,
		"sender":      "bob",
		"content":     "retry me",
		"clientToken": token,
	}

	first, err := sendMessage(ctx, args)
	if err != nil {
		t.Fatalf("first sendMessage failed: %v", err)
	}

	// Same clientToken, simulating a client retry after a network blip.
	second, err := sendMessage(ctx, args)
	if err != nil {
		t.Fatalf("retry sendMessage failed: %v", err)
	}

	if first.ID != second.ID || first.CreatedAt != second.CreatedAt {
		t.Fatalf("expected identical message on retry, got %+v vs %+v", first, second)
	}

	messages, err := getMessages(ctx, map[string]interface{}{"chatRoomId": roomID})
	if err != nil {
		t.Fatalf("getMessages failed: %v", err)
	}
	if len(messages) != 1 {
		t.Fatalf("expected exactly 1 stored message after duplicate retry, got %d", len(messages))
	}
}

func TestEditAndDeleteMessage(t *testing.T) {
	ctx := context.Background()
	roomID := "test-room-" + uuid.NewString()

	msg, err := sendMessage(ctx, map[string]interface{}{
		"chatRoomId":  roomID,
		"sender":      "carol",
		"content":     "original",
		"clientToken": uuid.NewString(),
	})
	if err != nil {
		t.Fatalf("sendMessage failed: %v", err)
	}

	edited, err := editMessage(ctx, map[string]interface{}{
		"chatRoomId": roomID,
		"createdAt":  msg.CreatedAt,
		"id":         msg.ID,
		"content":    "edited",
	})
	if err != nil {
		t.Fatalf("editMessage failed: %v", err)
	}
	if edited.Content != "edited" || edited.EditedAt == nil {
		t.Fatalf("expected edited content and editedAt set, got %+v", edited)
	}

	deleted, err := deleteMessage(ctx, map[string]interface{}{
		"chatRoomId": roomID,
		"createdAt":  msg.CreatedAt,
		"id":         msg.ID,
	})
	if err != nil {
		t.Fatalf("deleteMessage failed: %v", err)
	}
	if deleted.Deleted == nil || !*deleted.Deleted {
		t.Fatalf("expected deleted=true, got %+v", deleted)
	}
}

func TestEditMessageWrongIDFails(t *testing.T) {
	ctx := context.Background()
	roomID := "test-room-" + uuid.NewString()

	msg, err := sendMessage(ctx, map[string]interface{}{
		"chatRoomId":  roomID,
		"sender":      "carol",
		"content":     "original",
		"clientToken": uuid.NewString(),
	})
	if err != nil {
		t.Fatalf("sendMessage failed: %v", err)
	}

	_, err = editMessage(ctx, map[string]interface{}{
		"chatRoomId": roomID,
		"createdAt":  msg.CreatedAt,
		"id":         "wrong-id",
		"content":    "should not apply",
	})
	if err == nil {
		t.Fatal("expected editMessage to fail when id doesn't match the row")
	}
}

func TestReadAndDeliveredReceipts(t *testing.T) {
	ctx := context.Background()
	roomID := "test-room-" + uuid.NewString()

	msg, err := sendMessage(ctx, map[string]interface{}{
		"chatRoomId":  roomID,
		"sender":      "dave",
		"content":     "read me",
		"clientToken": uuid.NewString(),
	})
	if err != nil {
		t.Fatalf("sendMessage failed: %v", err)
	}

	read, err := markAsRead(ctx, map[string]interface{}{
		"chatRoomId": roomID,
		"createdAt":  msg.CreatedAt,
		"id":         msg.ID,
		"reader":     "erin",
	})
	if err != nil {
		t.Fatalf("markAsRead failed: %v", err)
	}
	if read.ReadAt == nil || read.ReadBy == nil || *read.ReadBy != "erin" {
		t.Fatalf("expected readAt/readBy set, got %+v", read)
	}

	delivered, err := markAsDelivered(ctx, map[string]interface{}{
		"chatRoomId": roomID,
		"createdAt":  msg.CreatedAt,
		"id":         msg.ID,
	})
	if err != nil {
		t.Fatalf("markAsDelivered failed: %v", err)
	}
	if delivered.DeliveredAt == nil {
		t.Fatalf("expected deliveredAt set, got %+v", delivered)
	}
}

func TestTypingAndPresenceEvents(t *testing.T) {
	ev, err := sendTypingEvent(map[string]interface{}{
		"chatRoomId": "room-1",
		"sender":     "alice",
		"isTyping":   true,
	})
	if err != nil {
		t.Fatalf("sendTypingEvent failed: %v", err)
	}
	if !ev.IsTyping {
		t.Fatalf("expected isTyping=true, got %+v", ev)
	}

	presence, err := setPresence(map[string]interface{}{
		"userId": "alice",
		"online": true,
	})
	if err != nil {
		t.Fatalf("setPresence failed: %v", err)
	}
	if !presence.Online || presence.LastSeen == "" {
		t.Fatalf("expected online=true and lastSeen set, got %+v", presence)
	}
}

func TestConversationsAndUnreadCount(t *testing.T) {
	ctx := context.Background()
	roomID := "test-room-" + uuid.NewString()
	userID := "frank-" + uuid.NewString()

	_, err := sendMessage(ctx, map[string]interface{}{
		"chatRoomId":   roomID,
		"sender":       "grace",
		"content":      "hi frank",
		"clientToken":  uuid.NewString(),
		"participants": []interface{}{userID},
	})
	if err != nil {
		t.Fatalf("sendMessage failed: %v", err)
	}

	count, err := getUnreadCount(ctx, map[string]interface{}{
		"chatRoomId": roomID,
		"userId":     userID,
	})
	if err != nil {
		t.Fatalf("getUnreadCount failed: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected unread count 1, got %d", count)
	}

	convos, err := listConversations(ctx, map[string]interface{}{"userId": userID})
	if err != nil {
		t.Fatalf("listConversations failed: %v", err)
	}
	if len(convos) != 1 || convos[0].ChatRoomID != roomID {
		t.Fatalf("expected 1 conversation for room %s, got %+v", roomID, convos)
	}
}
