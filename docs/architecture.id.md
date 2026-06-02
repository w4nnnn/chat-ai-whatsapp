# Arsitektur

## Gambaran Umum

WhatsApp AI Chatbot adalah aplikasi Go yang terhubung ke WhatsApp melalui whatsmeow (API multi-device), memproses pesan masuk menggunakan LLM melalui 9Router, dan menyimpan sesi di Redis.

## Diagram Sistem

```
┌─────────────┐
│  WhatsApp   │
└──────┬──────┘
       │ WebSocket
┌──────┴──────────────────────────────────────────────┐
│  whatsmeow (library Go)                             │
│  • Protokol WhatsApp multi-device                   │
│  • Pairing QR (pertama kali)                        │
│  • Auto-reconnect                                   │
│  • Event-driven (pesan, status koneksi)             │
└──────┬──────────────────────────────────────────────┘
       │
┌──────┴──────────────────────────────────────────────┐
│  internal/whatsapp/client.go                        │
│  • Registrasi event handler                         │
│  • Filter pesan (self-reply, nomor diizinkan)       │
│  • Download media (nanti)                            │
└──────┬──────────────────────────────────────────────┘
       │
┌──────┴──────────────────────────────────────────────┐
│  internal/handler/handler.go (Orchestrator)         │
│                                                     │
│  Alur per pesan:                                    │
│  1. Cek rate limit (Redis INCR + EXPIRE)            │
│  2. Ambil riwayat sesi (Redis, maks 20 pesan)       │
│  3. Bangun array pesan: system prompt + history +   │
│     pesan baru                                      │
│  4. Panggil AI (9Router) dengan tools               │
│     └─ Jika AI panggil search_web → SearXNG        │
│  5. Simpan pesan user + AI ke Redis                 │
│  6. Kembalikan teks response                        │
└──────┬──────────────────────────────────────────────┘
       │
       ├──────────────────────────────────────┐
       │                                      │
┌──────┴──────────────┐             ┌─────────┴──────────┐
│  9Router (AI Proxy) │             │  Redis              │
│  • OpenAI-compatible │             │  • whatsapp:session │
│  • Format translation│             │    :{phone}:messages│
│  • 3-tier fallback  │             │    → List (20 msg)  │
│  • RTK token saver  │             │    → TTL 24 jam     │
│  • 40+ provider     │             │  • whatsapp:ratelimit│
└──────────┬───────────┘             │    :{phone}         │
           │                         │    → TTL 60 detik  │
           ▼                         └────────────────────┘
┌──────────────────────┐
│  AI Model            │
│  OpenCode Go         │
│  (mimo-v2.5)         │
│  • Bisa lihat gambar │
│  • Tool calling      │
└──────────────────────┘

           ▼
┌──────────────────────┐
│  SearXNG             │
│  (saat tool dipakai) │
│  • Meta search engine│
│  • Google + Bing     │
│  • Tanpa batas       │
│  • Self-hosted       │
└──────────────────────┘
```

## Alur Data (Detail)

```
User kirim pesan
    ↓
whatsmeow emit events.Message
    ↓
client.handleMessage():
    ├─ Cek group chat (skip @g.us)
    ├─ Resolve nomor HP dari JID
    │   ├─ Sender bisa berupa LID (contoh: 88721...@lid)
    │   ├─ Pake SenderAlt/RecipientAlt buat dapetin nomor HP
    │   └─ candidateIDs = [nomor, lid, ...] buat pencocokan
    ├─ Cek allowed_numbers
    │   ├─ * = izinkan semua
    │   ├─ self = cuma kalo Chat == JID sendiri
    │   └─ nomor = cocokkan dengan candidateIDs
    ├─ Cek timestamp pesan (skip jika sebelum connectTime)
    │   └─ Mencegah bales pesan lama dari history sync
    └─ Ambil teks pesan
    ↓
handler.Handle():
    ├─ checkRateLimit(phone) → Redis INCR
    │   └─ Blokir jika >10 msg/menit
    ├─ getSessionMessages(phone) → Redis LRANGE
    │   └─ Kembalikan maks 20 pesan
    ├─ Bangun array pesan:
    │   [{role: system, content: systemPrompt},
    │    ...history,
    │    {role: user, content: text}]
    ├─ AI.Chat(messages) → POST ke 9Router
    │   ├─ Response ada tool_calls?
    │   │   └─ Eksekusi tool (search_web → SearXNG)
    │   │   └─ Panggil AI lagi dengan hasil tool
    │   └─ Kembalikan teks final
    ├─ addMessage(phone, userMsg) → Redis LPUSH+LTRIM
    ├─ addMessage(phone, aiMsg) → Redis LPUSH+LTRIM
    └─ Kembalikan teks response
    ↓
Kirim "mengetik..." tiap 5 detik selama AI proses
    ↓
Konversi Markdown → format WhatsApp (tebal, miring, dll)
    ↓
whatsmeow kirim balasan via SendMessage()
```

## Storage

### Redis (Session Store)

| Key | Type | TTL | Kegunaan |
|-----|------|-----|----------|
| `whatsapp:sessions:{phone}:messages` | List | 24 jam | Riwayat chat (maks 20) |
| `whatsapp:ratelimit:{phone}` | String | 60 detik | Counter rate limit |

### SQLite (Auth Store)

File: `{DATA_DIR}/whatsmeow-store.db` (default: `./data/whatsmeow/` di Docker atau `./` lokal)

Menyimpan kredensial sesi WhatsApp (signal keys, prekeys, identity). Dikelola otomatis oleh sqlstore dari whatsmeow.

Semua data runtime pake Docker bind mounts di `./data/`:
- `./data/redis/` — Persistensi Redis
- `./data/searxng/` — Konfigurasi settings.yml SearXNG
- `./data/9router/` — Data proxy 9Router
- `./data/whatsmeow/` — Session store WhatsApp

## Error Handling

| Skenario | Perilaku |
|----------|----------|
| 9Router error | Coba ulang 2x, lalu kirim pesan error |
| SearXNG timeout | Skip search, AI jawab dari pengetahuan |
| AI response kosong | Pesan fallback ke user |
| Redis mati | App gagal start (hard dependency) |
| WhatsApp disconnect | Auto-reconnect (built-in whatsmeow) |
| Rate limit terkena | Reply "mohon tunggu" |
| Tipe pesan tak dikenal | Skip diam-diam |
| Pesan gambar | Saat ini di-skip (text-only dulu) |
| Pesan lama (history sync) | Skip diam-diam berdasarkan timestamp |
| LID vs nomor HP | Diresolve via SenderAlt/RecipientAlt untuk filtering |

## Konfigurasi

Lihat `.env.example` untuk semua opsi konfigurasi. Pengaturan utama:

- **SELF_RESPON**: Aktifkan self-reply untuk testing
- **ALLOWED_NUMBERS**: Batasi nomor yang bisa direspon bot
- **NINE_ROUTER_***: Pengaturan koneksi 9Router
- **REDIS_URL**: Koneksi Redis
- **SEARXNG_BASE_URL**: Endpoint SearXNG
