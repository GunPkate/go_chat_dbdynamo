package logic

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

// Product represents one item in the hotel's product catalog — a
// room-service menu item, a minibar item, or a shop item. Table key
// schema: partition key = id (no sort key).
type Product struct {
	ID          string   `json:"id" dynamodbav:"id"`
	Name        string   `json:"name" dynamodbav:"name"`
	Description string   `json:"description" dynamodbav:"description"`
	Category    string   `json:"category" dynamodbav:"category"`
	Price       float64  `json:"price" dynamodbav:"price"`
	Stock       int      `json:"stock" dynamodbav:"stock"`
	Tags        []string `json:"tags" dynamodbav:"tags"`
}

// AssistantReply is the shared shape every AI-driven resolver returns:
// a natural-language answer plus (optionally) the specific products it's
// referring to, so the frontend can render real product cards instead of
// parsing them back out of the prose.
type AssistantReply struct {
	Answer   string    `json:"answer"`
	Products []Product `json:"products"`
}

var ProductsTable = "Products"

func init() {
	if v := envOr("PRODUCTS_TABLE", "Products"); v != "" {
		ProductsTable = v
	}
}

// listAllProducts fetches the whole catalog. A real product catalog would
// use search/an index instead of a full table scan, but for a
// hundred-item hotel catalog this is simple and fast enough — and it's
// exactly the context we hand to the AI for search/recommend/substitute,
// since letting the model reason over the real catalog beats trying to
// keep a separate search index in sync.
func listAllProducts(ctx context.Context) ([]Product, error) {
	out, err := DDB.Scan(ctx, &dynamodb.ScanInput{
		TableName: aws.String(ProductsTable),
	})
	if err != nil {
		return nil, fmt.Errorf("scan products: %w", err)
	}
	var products []Product
	if err := attributevalue.UnmarshalListOfMaps(out.Items, &products); err != nil {
		return nil, fmt.Errorf("unmarshal products: %w", err)
	}
	return products, nil
}

func getProduct(ctx context.Context, id string) (*Product, error) {
	out, err := DDB.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(ProductsTable),
		Key: map[string]types.AttributeValue{
			"id": &types.AttributeValueMemberS{Value: id},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("get product: %w", err)
	}
	if out.Item == nil {
		return nil, fmt.Errorf("product %q not found", id)
	}
	var p Product
	if err := attributevalue.UnmarshalMap(out.Item, &p); err != nil {
		return nil, err
	}
	return &p, nil
}
