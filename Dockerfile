FROM golang:1.26-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY cmd ./cmd
COPY internal ./internal
COPY migrations ./migrations
COPY sqlc.yaml ./

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/app-go ./cmd/server

FROM alpine:3.22

RUN apk add --no-cache ca-certificates tzdata wget
WORKDIR /app

COPY --from=builder /out/app-go /usr/local/bin/app-go
COPY migrations ./migrations

CMD ["app-go"]
