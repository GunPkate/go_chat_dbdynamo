# gqlgen's generated code (generated/generated.go, generated/models_gen.go)
# isn't checked into this project — it's produced fresh at build time by
# running the generator here, which needs network access during `docker
# build` to fetch Go modules (same as any Go build; this container's
# build step is not subject to this project's own sandbox restrictions).
FROM golang:1.22 AS build
WORKDIR /app

COPY go.mod ./
COPY gqlgen.yml schema.graphqls server.go ./
COPY graph ./graph
COPY logic ./logic

RUN go mod tidy
# Produces generated/generated.go + generated/models_gen.go. Because
# graph/schema.resolvers.go already exists with implementations matching
# gqlgen's expected signatures, this should NOT overwrite them — it only
# fills in the exec schema and any missing model glue.
RUN go get github.com/99designs/gqlgen@v0.17.44
RUN go run github.com/99designs/gqlgen generate
RUN CGO_ENABLED=0 go build -o /server .

FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates \
    && rm -rf /var/lib/apt/lists/*
COPY --from=build /server /server
EXPOSE 8080
ENTRYPOINT ["/server"]
