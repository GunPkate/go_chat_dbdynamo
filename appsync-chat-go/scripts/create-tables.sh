#!/bin/sh
# Creates the three tables this project needs against DynamoDB Local.
# Runs inside the "create-tables" service in docker-compose.yml, so it
# talks to "dynamodb-local:8000" (the service name on the compose network).
set -e

ENDPOINT="http://dynamodb-local:8000"

echo "waiting for dynamodb-local to accept connections..."
until aws dynamodb list-tables --endpoint-url "$ENDPOINT" >/dev/null 2>&1; do
  sleep 1
done

echo "creating ChatMessages (pk: chatRoomId, sk: createdAt)..."
aws dynamodb create-table \
  --endpoint-url "$ENDPOINT" \
  --table-name ChatMessages \
  --attribute-definitions \
      AttributeName=chatRoomId,AttributeType=S \
      AttributeName=createdAt,AttributeType=S \
  --key-schema \
      AttributeName=chatRoomId,KeyType=HASH \
      AttributeName=createdAt,KeyType=RANGE \
  --billing-mode PAY_PER_REQUEST \
  >/dev/null 2>&1 && echo "  created" || echo "  already exists, skipping"

echo "creating UserConversations (pk: userId, sk: chatRoomId)..."
aws dynamodb create-table \
  --endpoint-url "$ENDPOINT" \
  --table-name UserConversations \
  --attribute-definitions \
      AttributeName=userId,AttributeType=S \
      AttributeName=chatRoomId,AttributeType=S \
  --key-schema \
      AttributeName=userId,KeyType=HASH \
      AttributeName=chatRoomId,KeyType=RANGE \
  --billing-mode PAY_PER_REQUEST \
  >/dev/null 2>&1 && echo "  created" || echo "  already exists, skipping"

echo "creating ChatIdempotency (pk: clientToken)..."
aws dynamodb create-table \
  --endpoint-url "$ENDPOINT" \
  --table-name ChatIdempotency \
  --attribute-definitions AttributeName=clientToken,AttributeType=S \
  --key-schema AttributeName=clientToken,KeyType=HASH \
  --billing-mode PAY_PER_REQUEST \
  >/dev/null 2>&1 && echo "  created" || echo "  already exists, skipping"

echo "all tables ready"
