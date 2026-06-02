# Architecture

## Overview

WhatsApp AI Chatbot is a Go application that connects to WhatsApp via whatsmeow (multi-device API), processes incoming messages using an LLM through 9Router, and maintains session state in Redis.

## System Diagram

```
┌─────────────┐
│  WhatsApp   │
└──────┬──────┘
       │ WebSocket
┌──────┴──────────────────────────────────────────────┐
│  whatsmeow (Go library)                             │
│  • Multi-device WhatsApp protocol                   │
│  • QR pairing (first time)                          │
│  • Auto-reconnect                                   │
│  • Event-driven (messages, connection status)       │
└──────┬──────────────────────────────────────────────┘
       │
┌──────┴──────────────────────────────────────────────┐
│  internal/whatsapp/client.go                        │
│  • Event handler registration                       │
│  • Message filtering (self-reply, allowed numbers)  │
│  • Media download (future)                          │
└──────┬──────────────────────────────────────────────┘
       │
┌──────┴──────────────────────────────────────────────┐
│  internal/handler/handler.go (Orchestrator)         │
│                                                     │
│  Flow per message:                                  │
│  1. Rate limit check (Redis INCR + EXPIRE)          │
│  2. Load session history (Redis, max 20 msg)        │
│  3. Build message array: system prompt + history +  │
│     current message                                 │
│  4. Call AI (9Router) with tools                    │
│     └─ If AI calls search_web → SearXNG            │
│  5. Save user + AI messages to Redis                │
│  6. Return response text                            │
└──────┬──────────────────────────────────────────────┘
       │
       ├──────────────────────────────────────┐
       │                                      │
┌──────┴──────────────┐             ┌─────────┴──────────┐
│  9Router (AI Proxy) │             │  Redis              │
│  • OpenAI-compatible │             │  • whatsapp:session │
│  • Format translation│             │    :{phone}:messages│
│  • 3-tier fallback  │             │    → List (20 msg)  │
│  • RTK token saver  │             │    → TTL 24h        │
│  • 40+ providers    │             │  • whatsapp:ratelimit│
└──────────┬───────────┘             │    :{phone}         │
           │                         │    → TTL 60s        │
           ▼                         └────────────────────┘
┌──────────────────────┐
│  AI Model            │
│  OpenCode Go         │
│  (mimo-v2.5)         │
│  • Vision-capable    │
│  • Tool calling      │
└──────────────────────┘

           ▼
┌──────────────────────┐
│  SearXNG             │
│  (when tool is used) │
│  • Meta search engine│
│  • Google + Bing     │
│  • Unlimited queries │
│  • Self-hosted       │
└──────────────────────┘
```

## Data Flow (Detailed)

```
User sends message
    ↓
whatsmeow emits events.Message
    ↓
client.handleMessage():
    ├─ Check group chat (skip @g.us)
    ├─ Resolve phone number from JID
    │   ├─ Sender may be LID (e.g., 88721...@lid)
    │   ├─ Use SenderAlt/RecipientAlt for phone number
    │   └─ candidateIDs = [phone, lid, ...] for matching
    ├─ Check allowed_numbers
    │   ├─ * = allow all
    │   ├─ self = only when Chat == own JID
    │   └─ number = match against candidateIDs
    ├─ Check message timestamp (skip if before connectTime)
    │   └─ Prevents replying to old messages from history sync
    └─ Extract text content
    ↓
handler.Handle():
    ├─ checkRateLimit(phone) → Redis INCR
    │   └─ Block if >10 msg/min
    ├─ getSessionMessages(phone) → Redis LRANGE
    │   └─ Returns max 20 messages
    ├─ Build messages array:
    │   [{role: system, content: systemPrompt},
    │    ...history,
    │    {role: user, content: text}]
    ├─ AI.Chat(messages) → POST to 9Router
    │   ├─ Response has tool_calls?
    │   │   └─ Execute tool (search_web → SearXNG)
    │   │   └─ Call AI again with tool result
    │   └─ Return final text
    ├─ addMessage(phone, userMsg) → Redis LPUSH+LTRIM
    ├─ addMessage(phone, aiMsg) → Redis LPUSH+LTRIM
    └─ Return response text
    ↓
Send "typing..." keepalive every 5s while AI processes
    ↓
whatsmeow sends reply via SendMessage()
```

## Storage

### Redis (Session Store)

| Key | Type | TTL | Purpose |
|-----|------|-----|---------|
| `whatsapp:sessions:{phone}:messages` | List | 24h | Message history (max 20) |
| `whatsapp:ratelimit:{phone}` | String | 60s | Rate limit counter |

### SQLite (Auth Store)

File: `whatsmeow-store.db`

Stores WhatsApp session credentials (signal keys, prekeys, identity). Managed automatically by whatsmeow's sqlstore.

## Error Handling

| Scenario | Behavior |
|----------|----------|
| 9Router error | Retry 2x, then return error message |
| SearXNG timeout | Skip search, AI answers from knowledge |
| AI empty response | Fallback message to user |
| Redis down | App fails to start (hard dependency) |
| WhatsApp disconnect | Auto-reconnect (built-in whatsmeow) |
| Rate limited | Polite "please wait" reply |
| Unknown message type | Silently skipped |
| Image message | Currently skipped (text only for now) |
| Old message (history sync) | Silently skipped based on timestamp |
| LID vs phone number | Resolved via SenderAlt/RecipientAlt for filtering |

## Configuration

See `.env.example` for all configuration options. Key settings:

- **SELF_RESPON**: Enable self-reply for testing
- **ALLOWED_NUMBERS**: Restrict which numbers the bot responds to
- **NINE_ROUTER_***: 9Router connection settings
- **REDIS_URL**: Redis connection string
- **SEARXNG_BASE_URL**: SearXNG endpoint
