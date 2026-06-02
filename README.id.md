# WhatsApp AI Chatbot

Bot WhatsApp bertenaga AI untuk membantu siswa menjawab pertanyaan sekolah. Dibangun dengan Go dan whatsmeow.

> 📖 Dokumentasi arsitektur: [docs/architecture.id.md](docs/architecture.id.md)

## Prasyarat

- Go 1.26+
- Docker & Docker Compose (untuk Redis, SearXNG, 9Router)

## Mulai Cepat

```bash
# 1. Konfigurasi environment
cp .env.example .env
# Edit .env — isi API key kalo perlu

# 2. Jalankan infrastruktur (Redis, SearXNG, 9Router)
docker compose up -d redis searxng 9router

# 3. Jalankan bot
go run ./cmd/bot
```

Scan QR code yang muncul di terminal dengan WhatsApp.

## Konfigurasi

Semua pengaturan di `.env`:

| Variable | Default | Deskripsi |
|----------|---------|-----------|
| `NINE_ROUTER_BASE_URL` | `http://127.0.0.1:20128/v1` | Endpoint 9Router |
| `NINE_ROUTER_MODEL` | `ocg/mimo-v2.5` | Model AI |
| `NINE_ROUTER_API_KEY` | `sk-9router-key` | API key 9Router |
| `REDIS_URL` | `redis://127.0.0.1:6379` | Koneksi Redis |
| `SEARXNG_BASE_URL` | `http://127.0.0.1:4000` | Endpoint SearXNG |
| `SELF_RESPON` | `false` | Balas chat sendiri |
| `ALLOWED_NUMBERS` | `*` | Nomor diizinkan (`*` = semua, `self` = sendiri, atau `62812,self`) |
| `LOG_LEVEL` | `info` | Level log (debug, info, warn, error) |

### Nomor yang Diizinkan

- `*` — balas semua nomor
- `self` — cuma balas chat sendiri (testing)
- `62812,self` — chat sendiri + nomor tertentu
- `62812,62813` — cuma nomor tertentu (pake nomor HP, bukan LID)

### Mode Testing

Dua cara untuk testing dengan chat sendiri:

**Opsi A — Izinkan semua (termasuk diri sendiri):**
```env
SELF_RESPON=true
ALLOWED_NUMBERS=*
```

**Opsi B — Hanya diri sendiri (abaikan yang lain):**
```env
ALLOWED_NUMBERS=self
```

## Docker

```bash
# Start semua (build + jalankan)
docker compose up -d --build

# Update cuma app setelah ganti kode
docker compose up -d --build app
```

## Struktur Project

```
├── cmd/bot/main.go           # Entry point
├── internal/
│   ├── ai/                   # Client API 9Router
│   ├── config/               # Loader konfigurasi
│   ├── handler/              # Orchestrator pesan
│   ├── logger/               # Logger
│   ├── search/               # Client SearXNG
│   ├── store/                # Client Redis
│   └── whatsapp/             # Client whatsmeow
├── docker-compose.yml
├── Dockerfile
└── .env.example
```

## Fitur

- Balas teks dengan AI
- Pencarian web via SearXNG
- Support gambar/vision (mimo-v2.5)
- Rate limiting (10 pesan/menit/user)
- Memori sesi (20 pesan, expiry 24 jam)
- WhatsApp multi-device (pairing QR)
- Auto-reconnect
- System prompt bahasa Indonesia

## Arsitektur

Lihat [docs/architecture.id.md](docs/architecture.id.md) untuk dokumentasi arsitektur lengkap.
