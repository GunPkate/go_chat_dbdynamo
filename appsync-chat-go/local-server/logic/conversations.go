package logic

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

// Conversation is a per-user summary row. Table key schema: partition
// key = userId, sort key = chatRoomId.
type Conversation struct {
	UserID        string `json:"userId" dynamodbav:"userId"`
	ChatRoomID    string `json:"chatRoomId" dynamodbav:"chatRoomId"`
	LastMessage   string `json:"lastMessage" dynamodbav:"lastMessage"`
	LastMessageAt string `json:"lastMessageAt" dynamodbav:"lastMessageAt"`
	UnreadCount   int    `json:"unreadCount" dynamodbav:"unreadCount"`
}

func bumpConversation(ctx context.Context, userID, chatRoomID, lastMessage, lastMessageAt string) error {
	_, err := DDB.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName: aws.String(ConversationsTable),
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

func ListConversations(ctx context.Context, userID string) ([]Conversation, error) {
	if userID == "" {
		return nil, fmt.Errorf("userId is required")
	}
	out, err := DDB.Query(ctx, &dynamodb.QueryInput{
		TableName:              aws.String(ConversationsTable),
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

func GetUnreadCount(ctx context.Context, chatRoomID, userID string) (int, error) {
	if chatRoomID == "" || userID == "" {
		return 0, fmt.Errorf("chatRoomId and userId are required")
	}
	out, err := DDB.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(ConversationsTable),
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
