package graph

// Resolver is the root resolver gqlgen wires up via
// generated.Config{Resolvers: &Resolver{}} in server.go. This file is
// never touched by `go run github.com/99designs/gqlgen generate` after
// the first run — the actual field implementations live in
// schema.resolvers.go, which gqlgen preserves as long as method
// signatures already match what it expects.
type Resolver struct{}
