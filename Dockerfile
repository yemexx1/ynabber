# Build stage
FROM golang:alpine AS builder
RUN apk add --no-cache git
WORKDIR /go/src/app
COPY . .
RUN go get -d -v ./...
RUN go build -o /go/bin/app -v ./cmd/ynabber/.

# Final stage
FROM alpine:latest
RUN apk --no-cache add ca-certificates
RUN apk --no-cache add curl
COPY --from=builder /go/bin/app /app
COPY reader/nordigen/hooks/telegram-example.sh telegram.sh
RUN chmod +x telegram.sh
ENTRYPOINT /app
LABEL Name=ynabber
