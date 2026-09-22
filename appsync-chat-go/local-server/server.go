// This is the AppSync stand-in: a GraphQL server exposing the same
// schema (queries, mutations, subscriptions) but running entirely
// locally, on top of DynamoDB Local, with no AWS account involved.
package main

import (
	"log"
	"net/http"
	"os"

	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/handler/extension"
	"github.com/99designs/gqlgen/graphql/handler/transport"
	"github.com/99designs/gqlgen/graphql/playground"
	"github.com/gorilla/websocket"

	"appsync-chat-go/local-server/generated"
	"appsync-chat-go/local-server/graph"
	"appsync-chat-go/local-server/logic"
)

func main() {
	logic.Init()

	// Built manually with handler.New() rather than NewDefaultServer():
	// NewDefaultServer() registers its own default WebSocket transport
	// internally (with no CheckOrigin override), and since transports are
	// matched in registration order, that default one silently wins over
	// any custom Websocket transport added afterward via AddTransport —
	// which is exactly what was happening here. Building the transport
	// list ourselves means there's no conflicting default to shadow it.
	srv := handler.New(
		generated.NewExecutableSchema(generated.Config{Resolvers: &graph.Resolver{}}),
	)

	srv.AddTransport(transport.Options{})
	srv.AddTransport(transport.GET{})
	srv.AddTransport(transport.POST{})
	srv.AddTransport(transport.MultipartForm{})

	// Enables subscriptions over WebSocket — the local equivalent of
	// AppSync's real-time endpoint that the Go client's subscribe()
	// function talks to.
	srv.AddTransport(&transport.Websocket{
		Upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return true }, // fine for local dev only
			// Without listing these, gorilla's Upgrade() won't confirm a
			// subprotocol in its handshake response even though it accepts
			// the connection — and a browser that requested one (like the
			// frontend, which asks for "graphql-transport-ws") will then
			// immediately close the socket per the WebSocket spec, which
			// looks exactly like "connects, then instantly disconnects".
			Subprotocols: []string{"graphql-transport-ws", "graphql-ws"},
		},
	})

	// Needed for the GraphQL Playground's schema introspection/autocomplete.
	srv.Use(extension.Introspection{})

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	http.Handle("/", playground.Handler("Chat GraphQL Playground", "/query"))
	http.Handle("/query", withCORS(srv))

	log.Printf("GraphQL server (AppSync stand-in) listening on :%s — playground at http://localhost:%s/", port, port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}

// withCORS lets a browser-based frontend (opened via file://, a local dev
// server, or any other origin) call this server directly — fine for local
// dev, not something you'd ship to production as-is.
func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		// Chrome's Private Network Access policy blocks fetches from a
		// page with no clear "private" origin (e.g. one opened via
		// file://, or any public site) to localhost/private IPs, unless
		// the server explicitly opts in with this header on the
		// preflight response. Without it, the browser fails the request
		// silently before it ever reaches this handler.
		w.Header().Set("Access-Control-Allow-Private-Network", "true")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
