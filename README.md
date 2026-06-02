# WhatsApp AI Chatbot

An AI-powered WhatsApp bot for helping students with school questions. Built with Go and whatsmeow.

> 📖 Architecture docs: [docs/architecture.md](docs/architecture.md)

## Prerequisites

- Go 1.26+
- Docker & Docker Compose (for Redis, SearXNG, 9Router)

## Quick Start

```bash
# 1. Configure environment
cp .env.example .env
# Edit .env — set API key if needed

# 2. Start infrastructure (Redis, SearXNG, 9Router)
docker compose up -d redis searxng 9router

# 3. Run the bot
go run ./cmd/bot
```

Scan the QR code that appears in the terminal with WhatsApp.

## Configuration

All settings in `.env`:

| Variable | Default | Description |
|----------|---------|-------------|
| `NINE_ROUTER_BASE_URL` | `http://127.0.0.1:20128/v1` | 9Router endpoint |
| `NINE_ROUTER_MODEL` | `ocg/mimo-v2.5` | AI model |
| `NINE_ROUTER_API_KEY` | `sk-9router-key` | 9Router API key |
| `REDIS_URL` | `redis://127.0.0.1:6379` | Redis connection |
| `SEARXNG_BASE_URL` | `http://127.0.0.1:4000` | SearXNG endpoint |
| `SELF_RESPON` | `false` | Reply to own messages |
| `ALLOWED_NUMBERS` | `*` | Allowed numbers (`*` = all, or `62812,62813`) |
| `LOG_LEVEL` | `info` | Log level (debug, info, warn, error) |

### Allowed Numbers

- `*` — respond to everyone
- `62812,62813` — only specific numbers

### Self-Reply (Testing)

```env
SELF_RESPON=true
ALLOWED_NUMBERS=*
```

Bot will reply to your own messages without looping.

## Docker

```bash
# Start everything (build + run)
docker compose up -d --build

# Update only the app after code changes
docker compose up -d --build app
```

## Project Structure

```
├── cmd/bot/main.go           # Entry point
├── internal/
│   ├── ai/                   # 9Router API client
│   ├── config/               # Config loader
│   ├── handler/              # Message orchestrator
│   ├── logger/               # Logger
│   ├── search/               # SearXNG client
│   ├── store/                # Redis client
│   └── whatsapp/             # whatsmeow client
├── docker-compose.yml
├── Dockerfile
└── .env.example
```

## Features

- AI-powered text replies
- Web search via SearXNG
- Vision/image support (mimo-v2.5)
- Rate limiting (10 msg/min/user)
- Session memory (20 msgs, 24h expiry)
- WhatsApp multi-device (QR pairing)
- Auto-reconnect
- Indonesian language system prompt

## Architecture

See [docs/architecture.md](docs/architecture.md) for detailed architecture documentation in English.
