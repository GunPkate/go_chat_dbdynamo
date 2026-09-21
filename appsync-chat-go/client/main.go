// A minimal Go client for an AppSync chat API.
//
// It demonstrates two things:
//  1. Sending a mutation (sendMessage) over plain HTTPS.
//  2. Subscribing to onMessageSent over AppSync's real-time WebSocket
//     protocol, so the client receives new messages as they're sent
//     (by anyone, including other clients) without polling.
//
// This example uses API Key auth for simplicity. Swap the auth header
// logic for a Cognito JWT or SigV4 signer for production use.
package main

import (
	"bytes"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

// Config — fill these in for your AppSync API (found in the AWS console
// under AppSync > your API > Settings).
type Config struct {
	HTTPEndpoint string // e.g. https://xxxx.appsync-api.us-east-1.amazonaws.com/graphql
	APIKey       string // e.g. da2-xxxxxxxxxxxxxxxxxxxxxxxxxx
	ChatRoomID   string
	Sender       string
}

func main() {
	cfg := Config{}
	flag.StringVar(&cfg.HTTPEndpoint, "endpoint", "", "AppSync GraphQL HTTPS endpoint")
	flag.StringVar(&cfg.APIKey, "apikey", "", "AppSync API key")
	flag.StringVar(&cfg.ChatRoomID, "room", "room-123", "chat room id")
	flag.StringVar(&cfg.Sender, "sender", "golang-client", "sender name")
	send := flag.String("send", "", "if set, sends this message and exits instead of subscribing")
	flag.Parse()

	if cfg.HTTPEndpoint == "" || cfg.APIKey == "" {
		log.Fatal("both -endpoint and -apikey are required")
	}

	if *send != "" {
		msg, err := sendMessage(cfg, *send)
		if err != nil {
			log.Fatalf("sendMessage failed: %v", err)
		}
		fmt.Printf("sent: %+v\n", msg)
		return
	}

	fmt.Printf("subscribing to room %q — Ctrl+C to stop\n", cfg.ChatRoomID)
	if err := subscribe(cfg, func(m Message) {
		fmt.Printf("[%s] %s: %s\n", m.CreatedAt, m.Sender, m.Content)
	}); err != nil {
		log.Fatalf("subscription failed: %v", err)
	}
}

// ---------- Shared types ----------

type Message struct {
	ChatRoomID string `json:"chatRoomId"`
	CreatedAt  string `json:"createdAt"`
	ID         string `json:"id"`
	Sender     string `json:"sender"`
	Content    string `json:"content"`
}

type graphqlRequest struct {
	Query     string                 `json:"query"`
	Variables map[string]interface{} `json:"variables,omitempty"`
}

// ---------- Mutation over HTTPS ----------

func sendMessage(cfg Config, content string) (*Message, error) {
	const mutation = `
		mutation SendMessage($chatRoomId: ID!, $sender: String!, $content: String!) {
			sendMessage(chatRoomId: $chatRoomId, sender: $sender, content: $content) {
				chatRoomId
				createdAt
				id
				sender
				content
			}
		}`

	body := graphqlRequest{
		Query: mutation,
		Variables: map[string]interface{}{
			"chatRoomId": cfg.ChatRoomID,
			"sender":     cfg.Sender,
			"content":    content,
		},
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest(http.MethodPost, cfg.HTTPEndpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", cfg.APIKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Data struct {
			SendMessage Message `json:"sendMessage"`
		} `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	if len(result.Errors) > 0 {
		return nil, fmt.Errorf("graphql error: %s", result.Errors[0].Message)
	}
	return &result.Data.SendMessage, nil
}

// ---------- Subscription over AppSync's real-time WebSocket protocol ----------
//
// Docs: https://docs.aws.amazon.com/appsync/latest/devguide/real-time-websocket-client.html
//
// Flow:
//  1. Derive the wss:// realtime endpoint from the https:// GraphQL endpoint.
//  2. Connect with the "graphql-ws" subprotocol.
//  3. Send {"type":"connection_init"} and wait for {"type":"connection_ack"}.
//  4. Send {"type":"start", "id": "...", "payload": {...}} with the
//     subscription query and an auth block in "extensions.authorization".
//  5. Receive {"type":"data", "id":"...", "payload":{"data":{...}}} for
//     every new event, until you send {"type":"stop"} or close the socket.

type wsMessage struct {
	ID      string          `json:"id,omitempty"`
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

func subscribe(cfg Config, onMessage func(Message)) error {
	host, realtimeURL, err := realtimeURLFromHTTP(cfg.HTTPEndpoint)
	if err != nil {
		return err
	}

	// AppSync expects the auth header itself base64-encoded into the
	// "header" query param for the initial handshake.
	authHeader := map[string]string{
		"host":      host,
		"x-api-key": cfg.APIKey,
	}
	authJSON, _ := json.Marshal(authHeader)
	headerParam := base64.StdEncoding.EncodeToString(authJSON)
	payloadParam := base64.StdEncoding.EncodeToString([]byte("{}"))

	wsURL := fmt.Sprintf("%s?header=%s&payload=%s", realtimeURL, url.QueryEscape(headerParam), url.QueryEscape(payloadParam))

	dialer := websocket.Dialer{
		Subprotocols:    []string{"graphql-ws"},
		TLSClientConfig: &tls.Config{},
	}
	conn, _, err := dialer.Dial(wsURL, nil)
	if err != nil {
		return fmt.Errorf("dial: %w", err)
	}
	defer conn.Close()

	if err := conn.WriteJSON(wsMessage{Type: "connection_init"}); err != nil {
		return fmt.Errorf("connection_init: %w", err)
	}

	// Wait for connection_ack before subscribing.
	var ack wsMessage
	if err := conn.ReadJSON(&ack); err != nil {
		return fmt.Errorf("reading connection_ack: %w", err)
	}
	if ack.Type != "connection_ack" {
		return fmt.Errorf("expected connection_ack, got %q", ack.Type)
	}

	const subscriptionQuery = `
		subscription OnMessageSent($chatRoomId: ID!) {
			onMessageSent(chatRoomId: $chatRoomId) {
				chatRoomId
				createdAt
				id
				sender
				content
			}
		}`

	dataPayload, _ := json.Marshal(graphqlRequest{
		Query:     subscriptionQuery,
		Variables: map[string]interface{}{"chatRoomId": cfg.ChatRoomID},
	})

	startPayload := map[string]interface{}{
		"data": string(dataPayload),
		"extensions": map[string]interface{}{
			"authorization": authHeader,
		},
	}
	startPayloadJSON, _ := json.Marshal(startPayload)

	subID := uuid.NewString()
	if err := conn.WriteJSON(wsMessage{
		ID:      subID,
		Type:    "start",
		Payload: startPayloadJSON,
	}); err != nil {
		return fmt.Errorf("start: %w", err)
	}

	// Keep the connection alive and dispatch incoming events.
	for {
		var raw wsMessage
		if err := conn.ReadJSON(&raw); err != nil {
			if strings.Contains(err.Error(), "close") {
				return nil
			}
			return fmt.Errorf("read: %w", err)
		}

		switch raw.Type {
		case "start_ack":
			// subscription confirmed by the server
		case "ka":
			// keep-alive ping, nothing to do
		case "data":
			var dataEnv struct {
				Data struct {
					OnMessageSent Message `json:"onMessageSent"`
				} `json:"data"`
			}
			if err := json.Unmarshal(raw.Payload, &dataEnv); err != nil {
				log.Printf("unmarshal data payload: %v", err)
				continue
			}
			onMessage(dataEnv.Data.OnMessageSent)
		case "error":
			log.Printf("subscription error: %s", string(raw.Payload))
		case "connection_error":
			return fmt.Errorf("connection error: %s", string(raw.Payload))
		}
	}
}

// realtimeURLFromHTTP converts the standard AppSync HTTPS endpoint into
// its real-time (WebSocket) counterpart, per AWS's documented mapping:
// "appsync-api" -> "appsync-realtime-api", https -> wss.
func realtimeURLFromHTTP(httpEndpoint string) (host string, wsURL string, err error) {
	u, err := url.Parse(httpEndpoint)
	if err != nil {
		return "", "", err
	}
	host = u.Host // used in the auth header, must be the original https host

	rtHost := strings.Replace(u.Host, "appsync-api", "appsync-realtime-api", 1)
	rt := url.URL{Scheme: "wss", Host: rtHost, Path: u.Path}
	return host, rt.String(), nil
}

var _ = time.Now // keep time imported if you add reconnect/backoff logic
