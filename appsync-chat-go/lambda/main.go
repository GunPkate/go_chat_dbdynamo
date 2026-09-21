// Package main implements an AWS Lambda function used as an AppSync
// data source ("Lambda resolver"). AppSync invokes this function directly
// for every field in the schema, passing the GraphQL field name and
// arguments in the event payload; the switch in handler() routes each
// one to its own function below.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/google/uuid"
)

// Message mirrors the GraphQL "Message" type and the DynamoDB item shape.
// Table key schema: partition key = chatRoomId, sort key = createdAt.
type Message struct {
	ChatRoomID    string  `json:"chatRoomId" dynamodbav:"chatRoomId"`
	CreatedAt     string  `json:"createdAt" dynamodbav:"createdAt"`
	ID            string  `json:"id" dynamodbav:"id"`
	Sender        string  `json:"sender" dynamodbav:"sender"`
	Content       string  `json:"content" dynamodbav:"content"`
	AttachmentKey *string `json:"attachmentKey,omitempty" dynamodbav:"attachmentKey,omitempty"`
	EditedAt      *string `json:"editedAt,omitempty" dynamodbav:"editedAt,omitempty"`
	Deleted       *bool   `json:"deleted,omitempty" dynamodbav:"deleted,omitempty"`
	ReadAt        *string `json:"readAt,omitempty" dynamodbav:"readAt,omitempty"`
	ReadBy        *string `json:"readBy,omitempty" dynamodbav:"readBy,omitempty"`
	DeliveredAt   *string `json:"deliveredAt,omitempty" dynamodbav:"deliveredAt,omitempty"`
}

// Conversation is a per-user summary row used by listConversations /
// getUnreadCount. Table key schema: partition key = userId, sort key = chatRoomId.
type Conversation struct {
	UserID        string `json:"userId" dynamodbav:"userId"`
	ChatRoomID    string `json:"chatRoomId" dynamodbav:"chatRoomId"`
	LastMessage   string `json:"lastMessage" dynamodbav:"lastMessage"`
	LastMessageAt string `json:"lastMessageAt" dynamodbav:"lastMessageAt"`
	UnreadCount   int    `json:"unreadCount" dynamodbav:"unreadCount"`
}

// TypingEvent / PresenceEvent are never persisted — the Lambda just
// echoes them back so AppSync can push them to subscribers.
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

// idempotencyRecord backs sendMessage's duplicate-send protection.
// Table key schema: partition key = clientToken (TTL enabled).
type idempotencyRecord struct {
	ClientToken string `dynamodbav:"clientToken"`
	ChatRoomID  string `dynamodbav:"chatRoomId"`
	CreatedAt   string `dynamodbav:"createdAt"`
	MessageID   string `dynamodbav:"messageId"`
	ExpiresAt   int64  `dynamodbav:"expiresAt"` // DynamoDB TTL attribute, epoch seconds
}

// AppSyncEvent is the payload AppSync sends when this Lambda is wired up
// directly as a data source ("Direct Lambda" resolver mode).
type AppSyncEvent struct {
	Info struct {
		FieldName string `json:"fieldName"`
	} `json:"info"`
	Arguments map[string]interface{} `json:"arguments"`
}

var (
	ddbClient          *dynamodb.Client
	messagesTable      string
	conversationsTable string
	idempotencyTable   string
)

func init() {
	messagesTable = envOr("MESSAGES_TABLE", "ChatMessages")
	conversationsTable = envOr("CONVERSATIONS_TABLE", "UserConversations")
	idempotencyTable = envOr("IDEMPOTENCY_TABLE", "ChatIdempotency")

	// If DYNAMODB_LOCAL_ENDPOINT is set (e.g. by docker-compose, pointing at
	// "http://dynamodb-local:8000"), talk to DynamoDB Local with dummy
	// credentials instead of resolving real AWS credentials/region — this
	// is what lets the whole project run without an AWS account.
	localEndpoint := os.Getenv("DYNAMODB_LOCAL_ENDPOINT")

	var (
		cfg aws.Config
		err error
	)
	if localEndpoint != "" {
		cfg, err = config.LoadDefaultConfig(context.Background(),
			config.WithRegion(envOr("AWS_REGION", "us-east-1")),
			config.WithCredentialsProvider(
				credentials.NewStaticCredentialsProvider("local", "local", ""),
			),
		)
	} else {
		cfg, err = config.LoadDefaultConfig(context.Background())
	}
	if err != nil {
		panic(fmt.Sprintf("unable to load AWS config: %v", err))
	}

	if localEndpoint != "" {
		ddbClient = dynamodb.NewFromConfig(cfg, func(o *dynamodb.Options) {
			o.BaseEndpoint = aws.String(localEndpoint)
		})
	} else {
		ddbClient = dynamodb.NewFromConfig(cfg)
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func handler(ctx context.Context, event AppSyncEvent) (interface{}, error) {
	switch event.Info.FieldName {
	case "sendMessage":
		return sendMessage(ctx, event.Arguments)
	case "getMessages":
		return getMessages(ctx, event.Arguments)
	case "editMessage":
		return editMessage(ctx, event.Arguments)
	case "deleteMessage":
		return deleteMessage(ctx, event.Arguments)
	case "markAsRead":
		return markAsRead(ctx, event.Arguments)
	case "markAsDelivered":
		return markAsDelivered(ctx, event.Arguments)
	case "sendTypingEvent":
		return sendTypingEvent(event.Arguments)
	case "setPresence":
		return setPresence(event.Arguments)
	case "listConversations":
		return listConversations(ctx, event.Arguments)
	case "getUnreadCount":
		return getUnreadCount(ctx, event.Arguments)
	default:
		return nil, fmt.Errorf("unknown field: %s", event.Info.FieldName)
	}
}

// ---------- 1. Send message (with idempotent retry) ----------

func sendMessage(ctx context.Context, args map[string]interface{}) (*Message, error) {
	chatRoomID, _ := args["chatRoomId"].(string)
	sender, _ := args["sender"].(string)
	content, _ := args["content"].(string)
	clientToken, _ := args["clientToken"].(string)
	attachmentKey, _ := args["attachmentKey"].(string)

	if chatRoomID == "" || sender == "" || content == "" || clientToken == "" {
		return nil, fmt.Errorf("chatRoomId, sender, content and clientToken are required")
	}

	// Reserve the clientToken first. If another request already reserved
	// it (e.g. a network retry of the exact same send), this fails with
	// ConditionalCheckFailedException and we return the original message
	// instead of writing a duplicate — this is scenario "13. Duplicate send".
	messageID := uuid.NewString()
	createdAt := time.Now().UTC().Format(time.RFC3339Nano)

	rec := idempotencyRecord{
		ClientToken: clientToken,
		ChatRoomID:  chatRoomID,
		CreatedAt:   createdAt,
		MessageID:   messageID,
		ExpiresAt:   time.Now().Add(24 * time.Hour).Unix(),
	}
	recItem, err := attributevalue.MarshalMap(rec)
	if err != nil {
		return nil, fmt.Errorf("marshal idempotency record: %w", err)
	}

	_, err = ddbClient.PutItem(ctx, &dynamodb.PutItemInput{
		TableName:           aws.String(idempotencyTable),
		Item:                recItem,
		ConditionExpression: aws.String("attribute_not_exists(clientToken)"),
	})
	if err != nil {
		var condFailed *types.ConditionalCheckFailedException
		if errors.As(err, &condFailed) {
			return getExistingMessageForToken(ctx, clientToken)
		}
		return nil, fmt.Errorf("reserve idempotency token: %w", err)
	}

	msg := Message{
		ChatRoomID: chatRoomID,
		CreatedAt:  createdAt,
		ID:         messageID,
		Sender:     sender,
		Content:    content,
	}
	if attachmentKey != "" {
		msg.AttachmentKey = &attachmentKey
	}

	item, err := attributevalue.MarshalMap(msg)
	if err != nil {
		return nil, fmt.Errorf("marshal message: %w", err)
	}
	if _, err := ddbClient.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(messagesTable),
		Item:      item,
	}); err != nil {
		return nil, fmt.Errorf("put message: %w", err)
	}

	// Best-effort conversation/unread-count fan-out for each participant
	// other than the sender (scenario "14. Conversation list" / "18. Unread count").
	if participants, ok := args["participants"].([]interface{}); ok {
		for _, p := range participants {
			userID, _ := p.(string)
			if userID == "" || userID == sender {
				continue
			}
			if err := bumpConversation(ctx, userID, chatRoomID, content, createdAt); err != nil {
				// Non-fatal: the message itself already succeeded.
				fmt.Printf("warn: bumpConversation failed for %s: %v\n", userID, err)
			}
		}
	}

	// AppSync automatically fans the returned object out to every client
	// subscribed to onMessageSent(chatRoomId: <this room>) — returning the
	// mutation result IS the publish step, no extra code needed for that.
	return &msg, nil
}

func getExistingMessageForToken(ctx context.Context, clientToken string) (*Message, error) {
	out, err := ddbClient.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(idempotencyTable),
		Key: map[string]types.AttributeValue{
			"clientToken": &types.AttributeValueMemberS{Value: clientToken},
		},
	})
	if err != nil || out.Item == nil {
		return nil, fmt.Errorf("lookup idempotency record: %w", err)
	}
	var rec idempotencyRecord
	if err := attributevalue.UnmarshalMap(out.Item, &rec); err != nil {
		return nil, fmt.Errorf("unmarshal idempotency record: %w", err)
	}

	msgOut, err := ddbClient.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(messagesTable),
		Key: map[string]types.AttributeValue{
			"chatRoomId": &types.AttributeValueMemberS{Value: rec.ChatRoomID},
			"createdAt":  &types.AttributeValueMemberS{Value: rec.CreatedAt},
		},
	})
	if err != nil || msgOut.Item == nil {
		return nil, fmt.Errorf("lookup original message: %w", err)
	}
	var msg Message
	if err := attributevalue.UnmarshalMap(msgOut.Item, &msg); err != nil {
		return nil, err
	}
	return &msg, nil
}

// ---------- 2 & 3. Get messages (open conversation / reconnect) ----------

func getMessages(ctx context.Context, args map[string]interface{}) ([]Message, error) {
	chatRoomID, _ := args["chatRoomId"].(string)
	if chatRoomID == "" {
		return nil, fmt.Errorf("chatRoomId is required")
	}

	limit := int32(50)
	if l, ok := args["limit"].(float64); ok && l > 0 {
		limit = int32(l)
	}

	keyCond := "chatRoomId = :cid"
	exprValues := map[string]types.AttributeValue{
		":cid": &types.AttributeValueMemberS{Value: chatRoomID},
	}

	// "since" is used on reconnect (scenarios 4 & 12) to fetch only
	// messages the client missed while disconnected, instead of the
	// whole history.
	if since, ok := args["since"].(string); ok && since != "" {
		keyCond += " AND createdAt > :since"
		exprValues[":since"] = &types.AttributeValueMemberS{Value: since}
	}

	out, err := ddbClient.Query(ctx, &dynamodb.QueryInput{
		TableName:                 aws.String(messagesTable),
		KeyConditionExpression:    aws.String(keyCond),
		ExpressionAttributeValues: exprValues,
		ScanIndexForward:          aws.Bool(false), // newest first
		Limit:                     aws.Int32(limit),
	})
	if err != nil {
		return nil, fmt.Errorf("query: %w", err)
	}

	var messages []Message
	if err := attributevalue.UnmarshalListOfMaps(out.Items, &messages); err != nil {
		return nil, fmt.Errorf("unmarshal items: %w", err)
	}
	return messages, nil
}

// ---------- 9. Edit message / 10. Delete message ----------

func editMessage(ctx context.Context, args map[string]interface{}) (*Message, error) {
	chatRoomID, createdAt, id, ok := keyArgs(args)
	if !ok {
		return nil, fmt.Errorf("chatRoomId, createdAt and id are required")
	}
	content, _ := args["content"].(string)
	if content == "" {
		return nil, fmt.Errorf("content is required")
	}
	editedAt := time.Now().UTC().Format(time.RFC3339Nano)

	return updateMessage(ctx, chatRoomID, createdAt, id,
		"SET content = :content, editedAt = :editedAt",
		map[string]types.AttributeValue{
			":content":  &types.AttributeValueMemberS{Value: content},
			":editedAt": &types.AttributeValueMemberS{Value: editedAt},
		})
}

func deleteMessage(ctx context.Context, args map[string]interface{}) (*Message, error) {
	chatRoomID, createdAt, id, ok := keyArgs(args)
	if !ok {
		return nil, fmt.Errorf("chatRoomId, createdAt and id are required")
	}
	// Soft delete: keep the row (so subscribers/history still resolve it)
	// but flag it and clear the content clients would otherwise render.
	return updateMessage(ctx, chatRoomID, createdAt, id,
		"SET deleted = :deleted, content = :empty",
		map[string]types.AttributeValue{
			":deleted": &types.AttributeValueMemberBOOL{Value: true},
			":empty":   &types.AttributeValueMemberS{Value: ""},
		})
}

// ---------- 7. Read receipt / 8. Delivered receipt ----------

func markAsRead(ctx context.Context, args map[string]interface{}) (*Message, error) {
	chatRoomID, createdAt, id, ok := keyArgs(args)
	if !ok {
		return nil, fmt.Errorf("chatRoomId, createdAt and id are required")
	}
	reader, _ := args["reader"].(string)
	readAt := time.Now().UTC().Format(time.RFC3339Nano)

	return updateMessage(ctx, chatRoomID, createdAt, id,
		"SET readAt = :readAt, readBy = :readBy",
		map[string]types.AttributeValue{
			":readAt": &types.AttributeValueMemberS{Value: readAt},
			":readBy": &types.AttributeValueMemberS{Value: reader},
		})
}

func markAsDelivered(ctx context.Context, args map[string]interface{}) (*Message, error) {
	chatRoomID, createdAt, id, ok := keyArgs(args)
	if !ok {
		return nil, fmt.Errorf("chatRoomId, createdAt and id are required")
	}
	deliveredAt := time.Now().UTC().Format(time.RFC3339Nano)

	return updateMessage(ctx, chatRoomID, createdAt, id,
		"SET deliveredAt = :deliveredAt",
		map[string]types.AttributeValue{
			":deliveredAt": &types.AttributeValueMemberS{Value: deliveredAt},
		})
}

// keyArgs pulls the three fields every update-style mutation needs:
// the DynamoDB primary key (chatRoomId + createdAt) plus the message id,
// which is checked with a ConditionExpression so a stale/wrong createdAt
// can't silently edit the wrong row.
func keyArgs(args map[string]interface{}) (chatRoomID, createdAt, id string, ok bool) {
	chatRoomID, _ = args["chatRoomId"].(string)
	createdAt, _ = args["createdAt"].(string)
	id, _ = args["id"].(string)
	return chatRoomID, createdAt, id, chatRoomID != "" && createdAt != "" && id != ""
}

func updateMessage(ctx context.Context, chatRoomID, createdAt, id, updateExpr string, values map[string]types.AttributeValue) (*Message, error) {
	values[":id"] = &types.AttributeValueMemberS{Value: id}

	out, err := ddbClient.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName: aws.String(messagesTable),
		Key: map[string]types.AttributeValue{
			"chatRoomId": &types.AttributeValueMemberS{Value: chatRoomID},
			"createdAt":  &types.AttributeValueMemberS{Value: createdAt},
		},
		UpdateExpression:          aws.String(updateExpr),
		ConditionExpression:       aws.String("id = :id"),
		ExpressionAttributeValues: values,
		ReturnValues:              types.ReturnValueAllNew,
	})
	if err != nil {
		var condFailed *types.ConditionalCheckFailedException
		if errors.As(err, &condFailed) {
			return nil, fmt.Errorf("message not found or id mismatch")
		}
		return nil, fmt.Errorf("update item: %w", err)
	}

	var msg Message
	if err := attributevalue.UnmarshalMap(out.Attributes, &msg); err != nil {
		return nil, fmt.Errorf("unmarshal updated message: %w", err)
	}
	// AppSync publishes this return value to the matching subscription
	// (onMessageEdited / onMessageDeleted / onReadReceipt / onDeliveredReceipt).
	return &msg, nil
}

// ---------- 5 & 6. Typing indicator / 15. Presence ----------
//
// These never touch DynamoDB — the Lambda just validates and echoes the
// input back so AppSync can push it straight to subscribers.

func sendTypingEvent(args map[string]interface{}) (*TypingEvent, error) {
	chatRoomID, _ := args["chatRoomId"].(string)
	sender, _ := args["sender"].(string)
	isTyping, _ := args["isTyping"].(bool)
	if chatRoomID == "" || sender == "" {
		return nil, fmt.Errorf("chatRoomId and sender are required")
	}
	return &TypingEvent{ChatRoomID: chatRoomID, Sender: sender, IsTyping: isTyping}, nil
}

func setPresence(args map[string]interface{}) (*PresenceEvent, error) {
	userID, _ := args["userId"].(string)
	online, _ := args["online"].(bool)
	if userID == "" {
		return nil, fmt.Errorf("userId is required")
	}
	return &PresenceEvent{
		UserID:   userID,
		Online:   online,
		LastSeen: time.Now().UTC().Format(time.RFC3339Nano),
	}, nil
}

// ---------- 14. Conversation list / 18. Unread count ----------

func bumpConversation(ctx context.Context, userID, chatRoomID, lastMessage, lastMessageAt string) error {
	_, err := ddbClient.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName: aws.String(conversationsTable),
		Key: map[string]types.AttributeValue{
			"userId":     &types.AttributeValueMemberS{Value: userID},
			"chatRoomId": &types.AttributeValueMemberS{Value: chatRoomID},
		},
		UpdateExpression: aws.String(
			"SET lastMessage = :msg, lastMessageAt = :at ADD unreadCount :one"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":msg": &types.AttributeValueMemberS{Value: lastMessage},
			":at":  &types.AttributeValueMemberS{Value: lastMessageAt},
			":one": &types.AttributeValueMemberN{Value: "1"},
		},
	})
	return err
}

func listConversations(ctx context.Context, args map[string]interface{}) ([]Conversation, error) {
	userID, _ := args["userId"].(string)
	if userID == "" {
		return nil, fmt.Errorf("userId is required")
	}

	out, err := ddbClient.Query(ctx, &dynamodb.QueryInput{
		TableName:              aws.String(conversationsTable),
		KeyConditionExpression: aws.String("userId = :uid"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":uid": &types.AttributeValueMemberS{Value: userID},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("query conversations: %w", err)
	}

	var convos []Conversation
	if err := attributevalue.UnmarshalListOfMaps(out.Items, &convos); err != nil {
		return nil, fmt.Errorf("unmarshal conversations: %w", err)
	}
	return convos, nil
}

func getUnreadCount(ctx context.Context, args map[string]interface{}) (int, error) {
	chatRoomID, _ := args["chatRoomId"].(string)
	userID, _ := args["userId"].(string)
	if chatRoomID == "" || userID == "" {
		return 0, fmt.Errorf("chatRoomId and userId are required")
	}

	out, err := ddbClient.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(conversationsTable),
		Key: map[string]types.AttributeValue{
			"userId":     &types.AttributeValueMemberS{Value: userID},
			"chatRoomId": &types.AttributeValueMemberS{Value: chatRoomID},
		},
	})
	if err != nil {
		return 0, fmt.Errorf("get conversation: %w", err)
	}
	if out.Item == nil {
		return 0, nil
	}
	var convo Conversation
	if err := attributevalue.UnmarshalMap(out.Item, &convo); err != nil {
		return 0, err
	}
	return convo.UnreadCount, nil
}

func main() {
	lambda.Start(handler)
}
