// This is the AppSync stand-in: a GraphQL server exposing the same
// schema (queries, mutations, subscriptions) but running entirely
// locally, on top of DynamoDB Local, with no AWS account involved.
package main

import (
	"log"
	"net/http"
	"os"

	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/handler/transport"
	"github.com/99designs/gqlgen/graphql/playground"
	"github.com/gorilla/websocket"

	"appsync-chat-go/local-server/generated"
	"appsync-chat-go/local-server/graph"
	"appsync-chat-go/local-server/logic"
)

func main() {
	logic.Init()

	srv := handler.NewDefaultServer(
		generated.NewExecutableSchema(generated.Config{Resolvers: &graph.Resolver{}}),
	)

	// Enables subscriptions over WebSocket — the local equivalent of
	// AppSync's real-time endpoint that the Go client's subscribe()
	// function talks to.
	srv.AddTransport(&transport.Websocket{
		Upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return true }, // fine for local dev only
		},
	})

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	http.Handle("/", playground.Handler("Chat GraphQL Playground", "/query"))
	http.Handle("/query", srv)

	log.Printf("GraphQL server (AppSync stand-in) listening on :%s — playground at http://localhost:%s/", port, port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}
