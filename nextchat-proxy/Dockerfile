FROM golang:1.21-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -ldflags '-w -s' -o proxy main.go

FROM alpine:3.19

WORKDIR /app

RUN apk --no-cache add ca-certificates tzdata

COPY --from=builder /app/proxy .
COPY --from=builder /app/cache ./cache

RUN mkdir -p /app/cache && chmod 755 /app/cache

EXPOSE 8080

ENV GOMAXPROCS=4

CMD ["./proxy"]
