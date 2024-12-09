# compile go binary
FROM golang:1.23.1-alpine AS builder

WORKDIR /app

COPY . .

RUN go build -o main cmd/example/main.go

# create runner container
FROM alpine:latest

WORKDIR /app

ADD server.crt server.key /app/

COPY --from=builder /app/main .

CMD ["./main"]
