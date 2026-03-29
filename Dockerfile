FROM golang:1.25-alpine AS builder

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 \
   go build -ldflags="-s -w" -o /bin/go-to-rag .

FROM alpine:3.21

RUN apk add --no-cache ca-certificates
WORKDIR /app

COPY --from=builder /bin/go-to-rag /usr/local/bin/go-to-rag
COPY scripts/docker-entrypoint.sh /usr/local/bin/docker-entrypoint.sh
RUN chmod +x /usr/local/bin/docker-entrypoint.sh

ENV OLLAMA_HOST=http://localhost:11434
ENV CHAT_MODEL=go-to-rag:latest
ENV EMBED_MODEL=nomic-embed-text:latest
ENV DEMO_PROMPT="How does OLM manage the lifecycle of Operators on OpenShift?"

ENTRYPOINT ["docker-entrypoint.sh"]
CMD ["demo"]
