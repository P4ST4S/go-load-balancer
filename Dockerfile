FROM golang:1.25.4-alpine AS builder

WORKDIR /app

COPY go.mod ./

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -o lb ./cmd/lb

FROM alpine:latest

WORKDIR /root/

COPY --from=builder /app/lb .

EXPOSE 3030

ENTRYPOINT ["./lb"]