# AppSync Chat — Go Example

A minimal chat backend + client built on AWS AppSync, DynamoDB, and Go.

## Structure

```
appsync-chat-go/
├── schema.graphql       # GraphQL schema (Query/Mutation/Subscription)
├── lambda/              # Go Lambda used as the AppSync data source (resolver)
│   ├── main.go
│   └── go.mod
└── client/              # Go client: send messages + subscribe in real time
    ├── main.go
    └── go.mod
```

## How the pieces fit together

1. **DynamoDB tables**:
   - `ChatMessages` — partition key `chatRoomId` (String), sort key
     `createdAt` (String). Holds every message, including soft-deleted
     and edited ones.
   - `UserConversations` — partition key `userId` (String), sort key
     `chatRoomId` (String). One row per user per room; tracks
     `lastMessage`, `lastMessageAt`, and `unreadCount` for the inbox /
     unread-badge scenarios.
   - `ChatIdempotency` — partition key `clientToken` (String), TTL
     attribute `expiresAt`. Backs duplicate-send protection: a retried
     `sendMessage` with the same token returns the original message
     instead of creating a second one.
2. **Lambda function** (`lambda/main.go`) — deployed and attached to
   AppSync as a single "Direct Lambda" data source for every field in the
   schema. `handler()` routes on `event.info.fieldName` to the matching
   Go function. Covered fields:
   - `sendMessage` (idempotent via `clientToken`, optional `participants`
     fan-out), `getMessages` (with optional `since` for reconnect)
   - `editMessage`, `deleteMessage` (soft delete)
   - `markAsRead`, `markAsDelivered`
   - `sendTypingEvent`, `setPresence` (pass-through, no DB write)
   - `listConversations`, `getUnreadCount`
3. **AppSync API** — created from `schema.graphql`, with every Mutation
   and Query field pointing at the same Lambda data source, and every
   `Subscription` field using `@aws_subscribe(mutations: [...])` so
   AppSync auto-publishes each mutation's return value — no resolver code
   needed for the push side.
4. **Go client** (`client/main.go`) — sends messages over plain HTTPS, and
   subscribes to `onMessageSent` over AppSync's real-time WebSocket protocol
   to receive live updates. (Still wired to the original `sendMessage`
   shape; see "Extending the client" below.)

## Deploying the Lambda + AppSync API (CLI sketch)

```bash
# 1. Build and package the Lambda
cd lambda
GOOS=linux GOARCH=amd64 go build -o bootstrap main.go
zip function.zip bootstrap

aws lambda create-function \
  --function-name chatResolver \
  --runtime provided.al2023 \
  --handler bootstrap \
  --zip-file fileb://function.zip \
  --role arn:aws:iam::<account-id>:role/<lambda-execution-role> \
  --environment Variables={MESSAGES_TABLE=ChatMessages}

# 2. Create the DynamoDB table
aws dynamodb create-table \
  --table-name ChatMessages \
  --attribute-definitions AttributeName=chatRoomId,AttributeType=S AttributeName=createdAt,AttributeType=S \
  --key-schema AttributeName=chatRoomId,KeyType=HASH AttributeName=createdAt,KeyType=RANGE \
  --billing-mode PAY_PER_REQUEST

# 3. Create the AppSync API (schema, data source, resolvers) — easiest done
#    via the AWS Console or Amplify/CDK/Terraform, since wiring resolvers
#    to a Lambda data source is verbose over the raw CLI.
```

In production, use **Terraform**, the **CDK**, or **Amplify** rather than
raw CLI calls — they handle the AppSync ↔ Lambda ↔ IAM wiring far more
cleanly.

## Running the client

```bash
cd client
go mod tidy

# Send a message
go run main.go -endpoint https://xxxx.appsync-api.us-east-1.amazonaws.com/graphql \
  -apikey da2-xxxxxxxxxxxxxxxxxxxxxxxxxx \
  -room room-123 -sender alice -send "hey there!"

# In another terminal, subscribe and watch messages arrive live
go run main.go -endpoint https://xxxx.appsync-api.us-east-1.amazonaws.com/graphql \
  -apikey da2-xxxxxxxxxxxxxxxxxxxxxxxxxx \
  -room room-123
```

Run the subscribe command in one terminal, then send messages from another
(or from a second client) — you'll see them appear in the subscriber's
terminal in real time, with no polling.

## Running it all locally — no AWS account needed

`docker-compose.yml` spins up DynamoDB Local, creates the three tables,
and can run the full Go test suite against them, all with dummy
credentials.

```bash
# 1. Start the database, create tables, and the DynamoDB admin UI
docker compose up dynamodb-local create-tables dynamodb-admin

# 2. In another terminal, run the tests (builds a throwaway Go image)
docker compose run --rm tests
```

Browse table contents any time at **http://localhost:8001**.

`lambda/main_test.go` calls the resolver functions (`sendMessage`,
`getMessages`, `editMessage`, `markAsRead`, `listConversations`, etc.)
directly — no Lambda deploy, no AppSync, no network calls to AWS. This
covers all the business logic: idempotent sends, edits/deletes, receipts,
typing/presence events, reconnect ("since"), and unread counts.

If you have Go installed locally instead of using the `tests` service:

```bash
docker compose up -d dynamodb-local create-tables
cd lambda
DYNAMODB_LOCAL_ENDPOINT=http://localhost:8000 \
AWS_REGION=us-east-1 \
MESSAGES_TABLE=ChatMessages \
CONVERSATIONS_TABLE=UserConversations \
IDEMPOTENCY_TABLE=ChatIdempotency \
go test ./... -v
```

**What this does *not* cover:** AppSync itself — the GraphQL routing and
the real-time WebSocket push to subscribers — has no official free local
emulator from AWS. The tests above validate every resolver's behavior in
isolation; testing the actual subscription fan-out still needs a real
(even free-tier) AppSync API, or a paid LocalStack Pro instance.

## GraphQL layer: gqlgen local-server (a full AppSync stand-in)

`local-server/` is a self-hosted GraphQL server built with
[gqlgen](https://gqlgen.com) that mirrors the AppSync schema — including
**subscriptions** — so you can exercise the whole system (not just the
resolver logic) with no AWS account:

```bash
docker compose up dynamodb-local create-tables dynamodb-admin local-server
```

Then open **http://localhost:8080/** for the GraphQL Playground. Try:

```graphql
mutation {
  sendMessage(chatRoomId: "room-1", sender: "alice", content: "hi", clientToken: "abc-123") {
    id
    createdAt
  }
}
```

...and in a second Playground tab, before or after sending:

```graphql
subscription {
  onMessageSent(chatRoomId: "room-1") {
    sender
    content
  }
}
```

Sending the mutation pushes the new message to the open subscription tab
instantly over WebSocket — the same experience as AppSync's
`@aws_subscribe`, minus AWS.

**How it works:**
- `logic/` holds the same business logic as `lambda/main.go` (idempotent
  sends, edits, receipts, etc.), factored out so both the Lambda and this
  server call the same code shape, against the same DynamoDB tables.
- `logic/broker.go` is a small in-memory pub/sub broker that stands in
  for AppSync's `@aws_subscribe` directive — plain gqlgen has no
  equivalent, so each Mutation function calls `Broker.Publish(...)`
  explicitly, and each Subscription resolver in
  `graph/schema.resolvers.go` calls `Broker.Subscribe(...)`.
- `gqlgen.yml` binds the schema's types directly to the `logic` package's
  structs (via its `models:` section), so no duplicate model types or
  manual conversion is needed.

**Why there's no generated code checked in:** gqlgen's `generate` command
produces `generated/generated.go` and `generated/models_gen.go` from the
schema, and normally you'd commit those. Since this sandbox has no Go
toolchain to run the generator, `local-server/Dockerfile` runs
`go run github.com/99designs/gqlgen generate` **during the image build**
instead — `docker compose build local-server` (or `up`) does the codegen
for you automatically, using your machine's network access.

**If the build fails on the generate step:** `graph/schema.resolvers.go`
is hand-written to match gqlgen's standard naming convention (GraphQL
`chatRoomId` → Go parameter `chatRoomID`, etc.), which should line up on
a fresh `gqlgen generate`. If your installed gqlgen version names an
argument slightly differently, the fix is mechanical: open the compiler
error, find the mismatched method, and forward its parameters — in the
same order the schema declares them — into the matching `logic.*`
function (e.g. `logic.SendMessage(ctx, chatRoomID, sender, content,
clientToken, attachmentKey, participants)`); the logic function's
argument order always matches the schema field's argument order.

**Limitation to know about:** the broker is process-local — it only fans
out to subscribers connected to that one running instance. Fine for local
dev with a single server; a real multi-instance deployment is exactly the
problem AppSync (or a shared broker like Redis pub/sub) solves.

## Testing with a real frontend

`frontend/index.html` is a small, dependency-free test UI — plain HTML/JS,
no build step — that talks directly to `local-server`'s GraphQL endpoint.

**Important:** `http://dynamodb-local:8000` (the address you see in
`docker compose` logs) only resolves *inside* the Docker Compose network —
your browser can't reach it, and shouldn't need to. A frontend should
never talk to DynamoDB directly; it talks to the GraphQL layer
(`local-server`, AppSync's stand-in here), which is published to your
host machine at **`http://localhost:8080`**.

```bash
docker compose up dynamodb-local create-tables dynamodb-admin local-server
```

Then just open `frontend/index.html` directly in a browser (double-click
it, or `open frontend/index.html` / `start frontend/index.html`) — no
server needed, since `local-server` already sends permissive CORS headers
for local dev.

**If you get "Failed to fetch" in the browser:** this is almost always
Chrome's Private Network Access policy silently blocking the request
because the page has no clear origin (a `file://` page looks the same as
a public site to this check). `server.go` already sends the header that
opts back in, but if it still happens, serve the frontend over plain HTTP
instead of opening it as a raw file — it sidesteps the ambiguity entirely:

```bash
cd frontend
python3 -m http.server 5500
# then open http://localhost:5500/ in your browser
```

**To see real-time delivery working:** open the file in two browser tabs
(or two different browsers), set the same **Room ID** in both, a
different **Your name** in each, click **Join room** in both, then send a
message from one tab — it should appear in the other tab instantly over
the WebSocket subscription, with no page refresh.

The page does three things, all against `local-server`:
1. **`getMessages` query** on join, to load history
2. **`sendMessage` mutation** over HTTP when you hit Send
3. **`onMessageSent` subscription** over WebSocket (using the
   `graphql-transport-ws` protocol), which is how every joined tab —
   including your own — receives new messages in real time

## Debugging in VS Code

`.vscode/launch.json` has two ready-made debug configs: **Debug
local-server** and **Debug lambda tests**, both with real breakpoint
support (set a breakpoint in `graph/schema.resolvers.go` or any
`logic/*.go` file and it'll actually stop there).

**Prerequisites (one-time):**
1. Install the [Go extension](https://marketplace.visualstudio.com/items?itemName=golang.go) for VS Code.
2. Install Delve, the Go debugger it uses under the hood:
   ```bash
   go install github.com/go-delve/delve/cmd/dlv@latest
   ```
3. **Important:** `local-server`'s `generated/` package (gqlgen's exec
   schema) is currently only produced *inside the Docker build* — it
   doesn't exist in your checked-out folder, so VS Code can't compile the
   package to debug it yet. Generate it once, locally:
   ```bash
   cd local-server
   go mod tidy
   go run github.com/99designs/gqlgen generate
   ```
   Re-run this any time you change `schema.graphqls`. (`generated/` is
   build output — fine to add to `.gitignore` if you're versioning this.)

**To debug:**
1. Open the project root (the folder containing `docker-compose.yml`) in
   VS Code — the launch config's paths assume that's your workspace root.
2. Start just the database (not `local-server` — VS Code will run that
   part for you): `docker compose up dynamodb-local create-tables dynamodb-admin`
3. Open VS Code's **Run and Debug** panel (`Cmd/Ctrl+Shift+D`), pick
   **Debug local-server** from the dropdown, and hit the green play
   button (or `F5`).
4. Set a breakpoint (click left of a line number) in, say,
   `logic/messages.go`'s `SendMessage`, then trigger it from the
   Playground, the frontend, or `curl` — execution will pause there with
   full variable inspection, call stack, etc.

**Debug lambda tests** does the same for `lambda/main_test.go` — pick it,
hit `F5`, and it runs (and lets you breakpoint into) the whole test suite
directly, no Docker rebuild needed for the Lambda side either.

## Extending the client

`client/main.go` still only implements the original `sendMessage(chatRoomId,
sender, content)` call and the `onMessageSent` subscription — it hasn't been
updated for the new fields. To exercise the new backend logic, add a
`clientToken` (e.g. `uuid.NewString()`) to the mutation variables, and add
the same pattern used for `sendMessage`/`onMessageSent` for whichever new
mutation/subscription pair you want to try (e.g. `sendTypingEvent` /
`onTyping`).

## Notes / next steps

- **Auth**: this example uses an API key for simplicity. For real users,
  switch to **Amazon Cognito User Pools** and pass a JWT in the
  `Authorization` header (mutation) and in the `authorization` extension
  block (subscription) instead of `x-api-key`.
- **Scaling reads**: `getMessages` does a simple DynamoDB `Query` sorted by
  `createdAt`. Add pagination (`ExclusiveStartKey`) for long-lived rooms.
- **Fan-out extras**: typing indicators or presence can be added as their
  own lightweight mutation + subscription pair (e.g. `notifyTyping` /
  `onUserTyping`) following the exact same pattern.
- **Reconnects**: the client's WebSocket loop doesn't currently retry on
  disconnect — add backoff/retry logic around `subscribe()` for production.
- **Attachments**: `sendMessage` accepts an `attachmentKey` pointing at an
  S3 object; the actual file upload (e.g. via a pre-signed URL) happens
  client-side, before calling the mutation — the Lambda only stores the key.
- **Idempotency table TTL**: enable DynamoDB TTL on `ChatIdempotency`'s
  `expiresAt` attribute so old tokens are cleaned up automatically.

## Debug local
- **Window command**: set env for installing missing libs
```
$env:GOFLAGS="-mod=mod"
go get golang.org/x/tools@latest
go run github.com/99designs/gqlgen generate
Remove-Item Env:GOFLAGS
go mod tidy
```

- **MACOS command**: set env for installing missing libs
```
export GOFLAGS=-mod=mod
go get golang.org/x/tools@latest
go run github.com/99designs/gqlgen generate
unset GOFLAGS
go mod tidy
```

- **Config Launch.json**: press debug and disable local-server in docker-compse.yml
```
    {
      "name": "Debug local-server",
      "type": "go",
      "request": "launch",
      "mode": "auto",
      "program": "${workspaceFolder}/local-server",
      "env": {
        "DYNAMODB_LOCAL_ENDPOINT": "http://localhost:8000",
        "AWS_REGION": "us-east-1",
        "MESSAGES_TABLE": "ChatMessages",
        "CONVERSATIONS_TABLE": "UserConversations",
        "IDEMPOTENCY_TABLE": "ChatIdempotency",
        "PORT": "8080"
      }
    },
```