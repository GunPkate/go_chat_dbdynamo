package logic

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
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

// idempotencyRecord backs SendMessage's duplicate-send protection.
// Table key schema: partition key = clientToken.
type idempotencyRecord struct {
	ClientToken string `dynamodbav:"clientToken"`
	ChatRoomID  string `dynamodbav:"chatRoomId"`
	CreatedAt   string `dynamodbav:"createdAt"`
	MessageID   string `dynamodbav:"messageId"`
	ExpiresAt   int64  `dynamodbav:"expiresAt"`
}

func SendMessage(ctx context.Context, chatRoomID, sender, content, clientToken string, attachmentKey *string, participants []string) (*Message, error) {
	if chatRoomID == "" || sender == "" || content == "" || clientToken == "" {
		return nil, fmt.Errorf("chatRoomId, sender, content and clientToken are required")
	}

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

	_, err = DDB.PutItem(ctx, &dynamodb.PutItemInput{
		TableName:           aws.String(IdempotencyTable),
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
		ChatRoomID:    chatRoomID,
		CreatedAt:     createdAt,
		ID:            messageID,
		Sender:        sender,
		Content:       content,
		AttachmentKey: attachmentKey,
	}

	item, err := attributevalue.MarshalMap(msg)
	if err != nil {
		return nil, fmt.Errorf("marshal message: %w", err)
	}
	if _, err := DDB.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(MessagesTable),
		Item:      item,
	}); err != nil {
		return nil, fmt.Errorf("put message: %w", err)
	}

	for _, userID := range participants {
		if userID == "" || userID == sender {
			continue
		}
		if err := bumpConversation(ctx, userID, chatRoomID, content, createdAt); err != nil {
			fmt.Printf("warn: bumpConversation failed for %s: %v\n", userID, err)
		}
	}

	// This Publish is the hand-rolled stand-in for AppSync's
	// @aws_subscribe(mutations: ["sendMessage"]) — it's what actually
	// pushes to every client subscribed to onMessageSent(chatRoomId).
	MessageSentBroker.Publish(chatRoomID, msg)

	return &msg, nil
}

func getExistingMessageForToken(ctx context.Context, clientToken string) (*Message, error) {
	out, err := DDB.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(IdempotencyTable),
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

	msgOut, err := DDB.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(MessagesTable),
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

func GetMessages(ctx context.Context, chatRoomID string, limit *int, since *string) ([]Message, error) {
	if chatRoomID == "" {
		return nil, fmt.Errorf("chatRoomId is required")
	}

	l := int32(50)
	if limit != nil && *limit > 0 {
		l = int32(*limit)
	}

	keyCond := "chatRoomId = :cid"
	exprValues := map[string]types.AttributeValue{
		":cid": &types.AttributeValueMemberS{Value: chatRoomID},
	}
	if since != nil && *since != "" {
		keyCond += " AND createdAt > :since"
		exprValues[":since"] = &types.AttributeValueMemberS{Value: *since}
	}

	out, err := DDB.Query(ctx, &dynamodb.QueryInput{
		TableName:                 aws.String(MessagesTable),
		KeyConditionExpression:    aws.String(keyCond),
		ExpressionAttributeValues: exprValues,
		ScanIndexForward:          aws.Bool(false),
		Limit:                     aws.Int32(l),
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

func EditMessage(ctx context.Context, chatRoomID, createdAt, id, content string) (*Message, error) {
	editedAt := time.Now().UTC().Format(time.RFC3339Nano)
	msg, err := updateMessage(ctx, chatRoomID, createdAt, id,
		"SET content = :content, editedAt = :editedAt",
		map[string]types.AttributeValue{
			":content":  &types.AttributeValueMemberS{Value: content},
			":editedAt": &types.AttributeValueMemberS{Value: editedAt},
		})
	if err == nil {
		MessageEditedBroker.Publish(chatRoomID, *msg)
	}
	return msg, err
}

func DeleteMessage(ctx context.Context, chatRoomID, createdAt, id string) (*Message, error) {
	msg, err := updateMessage(ctx, chatRoomID, createdAt, id,
		"SET deleted = :deleted, content = :empty",
		map[string]types.AttributeValue{
			":deleted": &types.AttributeValueMemberBOOL{Value: true},
			":empty":   &types.AttributeValueMemberS{Value: ""},
		})
	if err == nil {
		MessageDeletedBroker.Publish(chatRoomID, *msg)
	}
	return msg, err
}

func MarkAsRead(ctx context.Context, chatRoomID, createdAt, id, reader string) (*Message, error) {
	readAt := time.Now().UTC().Format(time.RFC3339Nano)
	msg, err := updateMessage(ctx, chatRoomID, createdAt, id,
		"SET readAt = :readAt, readBy = :readBy",
		map[string]types.AttributeValue{
			":readAt": &types.AttributeValueMemberS{Value: readAt},
			":readBy": &types.AttributeValueMemberS{Value: reader},
		})
	if err == nil {
		ReadReceiptBroker.Publish(chatRoomID, *msg)
	}
	return msg, err
}

func MarkAsDelivered(ctx context.Context, chatRoomID, createdAt, id string) (*Message, error) {
	deliveredAt := time.Now().UTC().Format(time.RFC3339Nano)
	msg, err := updateMessage(ctx, chatRoomID, createdAt, id,
		"SET deliveredAt = :deliveredAt",
		map[string]types.AttributeValue{
			":deliveredAt": &types.AttributeValueMemberS{Value: deliveredAt},
		})
	if err == nil {
		DeliveredReceiptBroker.Publish(chatRoomID, *msg)
	}
	return msg, err
}

// updateMessage checks id via ConditionExpression so a stale/wrong
// createdAt can't silently update the wrong row.
func updateMessage(ctx context.Context, chatRoomID, createdAt, id, updateExpr string, values map[string]types.AttributeValue) (*Message, error) {
	values[":id"] = &types.AttributeValueMemberS{Value: id}

	out, err := DDB.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName: aws.String(MessagesTable),
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
	return &msg, nil
}
