FROM golang:1.26 AS builder

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/grimoire-bot ./cmd/grimoire-bot

FROM alpine:3.22

RUN apk add --no-cache ca-certificates

WORKDIR /opt/grimoire

COPY --from=builder /out/grimoire-bot /opt/grimoire/grimoire-bot
COPY docker-entrypoint.sh /usr/local/bin/docker-entrypoint.sh

RUN chmod +x /opt/grimoire/grimoire-bot /usr/local/bin/docker-entrypoint.sh

ENTRYPOINT ["/usr/local/bin/docker-entrypoint.sh"]
