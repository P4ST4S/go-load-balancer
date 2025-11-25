FROM golang:1.25.4-alpine AS builder

WORKDIR /app

COPY go.mod ./

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -o lb cmd/lb/main.go

FROM alpine:latest

WORKDIR /root/

# On copie uniuement le binaire compilé depuis l'étape 1
COPY --from=builder /app/lb .

EXPOSE 3030

ENTRYPOINT ["./lb"]