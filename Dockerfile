FROM golang:1.22-alpine AS builder

WORKDIR /build

RUN apk add --no-cache git make

COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG TARGETARCH
RUN CGO_ENABLED=0 GOOS=linux GOARCH=${TARGETARCH} \
    go build -ldflags="-s -w" -o /build/bin/game-service ./app/game/cmd/server

FROM alpine:3.19

RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app

COPY --from=builder /build/bin/game-service /app/game-service
COPY --from=builder /build/app/game/etc /app/etc

EXPOSE 8080

ENTRYPOINT ["/app/game-service"]
CMD ["-f", "/app/etc/game.yaml"]
