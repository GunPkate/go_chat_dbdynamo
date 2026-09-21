// Package logic holds all the chat business logic (DynamoDB reads/writes,
// idempotency, event publishing) independent of any transport. Both the
// Lambda resolver and this gqlgen server call the same shape of logic;
// here it's factored into a package so gqlgen's generated code can bind
// straight to these types via gqlgen.yml's "models:" section.
package logic

import (
	"context"
	"fmt"
	"os"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
)

var (
	DDB                *dynamodb.Client
	MessagesTable      string
	ConversationsTable string
	IdempotencyTable   string
)

// Init must be called once at startup (see server.go). If
// DYNAMODB_LOCAL_ENDPOINT is set, it talks to DynamoDB Local with dummy
// credentials instead of resolving real AWS credentials — this is what
// lets the whole thing run without an AWS account.
func Init() {
	MessagesTable = envOr("MESSAGES_TABLE", "ChatMessages")
	ConversationsTable = envOr("CONVERSATIONS_TABLE", "UserConversations")
	IdempotencyTable = envOr("IDEMPOTENCY_TABLE", "ChatIdempotency")

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
		DDB = dynamodb.NewFromConfig(cfg, func(o *dynamodb.Options) {
			o.BaseEndpoint = aws.String(localEndpoint)
		})
	} else {
		DDB = dynamodb.NewFromConfig(cfg)
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
