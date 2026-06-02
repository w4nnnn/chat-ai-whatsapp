package whatsapp

import (
	"context"
	"database/sql"
	"encoding/base64"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"go.mau.fi/whatsmeow"
	waProto "go.mau.fi/whatsmeow/binary/proto"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"
	"google.golang.org/protobuf/proto"

	"chat-ai-whatsapp/internal/config"
	"chat-ai-whatsapp/internal/format"
	"chat-ai-whatsapp/internal/handler"
	"chat-ai-whatsapp/internal/logger"

	"github.com/mdp/qrterminal/v3"
	_ "modernc.org/sqlite"
)

type Client struct {
	client      *whatsmeow.Client
	handler     *handler.Handler
	config      *config.Config
	connectTime time.Time // waktu bot konek — untuk filter pesan lama
}

func New(cfg *config.Config, dbPath string, msgHandler *handler.Handler) (*Client, error) {
	ctx := context.Background()
	db, err := sql.Open("sqlite", "file:"+dbPath)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	// Enable foreign keys, WAL mode, and busy timeout
	for _, pragma := range []string{
		"PRAGMA foreign_keys = ON",
		"PRAGMA journal_mode = WAL",
		"PRAGMA busy_timeout = 5000",
	} {
		if _, err := db.ExecContext(ctx, pragma); err != nil {
			return nil, fmt.Errorf("exec %s: %w", pragma, err)
		}
	}

	storeContainer := sqlstore.NewWithDB(db, "sqlite", nil)

	if err := storeContainer.Upgrade(ctx); err != nil {
		return nil, fmt.Errorf("upgrade database: %w", err)
	}

	device, err := storeContainer.GetFirstDevice(ctx)
	if err != nil {
		return nil, fmt.Errorf("get device: %w", err)
	}

	client := whatsmeow.NewClient(device, waLog.Stdout("whatsmeow", "WARN", true))

	return &Client{
		client:  client,
		handler: msgHandler,
		config:  cfg,
	}, nil
}

func (wc *Client) Start(ctx context.Context) error {
	wc.client.AddEventHandler(wc.handleEvent)

	if wc.client.Store.ID == nil {
		qrChan, err := wc.client.GetQRChannel(context.Background())
		if err != nil {
			return fmt.Errorf("get qr channel: %w", err)
		}

		if err := wc.client.Connect(); err != nil {
			return fmt.Errorf("connect: %w", err)
		}

		for qr := range qrChan {
			switch qr.Event {
			case "code":
				logger.Info("QR code received — scan with WhatsApp")
				qrterminal.GenerateHalfBlock(qr.Code, qrterminal.L, os.Stdout)
			case "success":
				logger.Info("QR code scanned — logged in!")
			case "timeout":
				logger.Warn("QR code timed out")
			}
		}
	} else {
		if err := wc.client.Connect(); err != nil {
			return fmt.Errorf("connect: %w", err)
		}
		logger.Info("WhatsApp connected (existing session)")
	}

	// Wait for shutdown
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig

	logger.Info("Shutting down...")
	wc.client.Disconnect()
	return nil
}

func (wc *Client) handleEvent(evt interface{}) {
	switch v := evt.(type) {
	case *events.Message:
		wc.handleMessage(v)
	case *events.Connected:
		wc.connectTime = time.Now()
		logger.Info("WhatsApp connected at %s", wc.connectTime.Format(time.TimeOnly))
	case *events.Disconnected:
		logger.Warn("WhatsApp disconnected")
	case *events.LoggedOut:
		logger.Error("WhatsApp logged out — delete session and re-pair")
	}
}

func (wc *Client) handleMessage(msg *events.Message) {
	// ─── Filter: group chats ────────────────────────────
	if msg.Info.Chat.Server == "g.us" {
		return
	}

	// ─── Ekstrak nomor HP dari JID ─────────────────────
	// whatsmeow v7 pake LID (angka random). Nomor HP ada di SenderAlt/RecipientAlt.
	senderJID := msg.Info.Sender.ToNonAD()
	
	// Cari nomor HP: prioritaskan SenderAlt (kalo mode LID)
	pnJID := senderJID
	if !msg.Info.SenderAlt.IsEmpty() {
		pnJID = msg.Info.SenderAlt.ToNonAD()
	}
	// Untuk self-messages, RecipientAlt punya nomor tujuan
	if msg.Info.IsFromMe && !msg.Info.RecipientAlt.IsEmpty() {
		pnJID = msg.Info.RecipientAlt.ToNonAD()
	}
	
	phone := pnJID.String()
	if phone == "" {
		return
	}
	phoneNumber := strings.Split(phone, "@")[0]

	// Kumpulin semua ID yang mungkin untuk filter
	candidateIDs := []string{phoneNumber}
	if !senderJID.IsEmpty() && senderJID.String() != phone {
		candidateIDs = append(candidateIDs, strings.Split(senderJID.String(), "@")[0])
	}

	isAllowed := false
	for _, a := range wc.config.AllowedNumbers {
		switch {
		case a == "*":
			isAllowed = true
		case a == "self" && msg.Info.IsFromMe:
			// self = cuma kalo chat ke diri sendiri
			chatJID := msg.Info.Chat.ToNonAD()
			isAllowed = chatJID == senderJID || chatJID == pnJID
		default:
			// Cocokkan dengan nomor HP (dari SenderAlt / RecipientAlt)
			for _, id := range candidateIDs {
				if a == id {
					isAllowed = true
					break
				}
			}
		}
		if isAllowed {
			break
		}
	}

	// SelfRespon=true: izinkan self message cuma kalo chat ke diri sendiri
	if !isAllowed && msg.Info.IsFromMe && wc.config.SelfRespon {
		chatJID := msg.Info.Chat.ToNonAD()
		isAllowed = chatJID == senderJID || chatJID == pnJID
	}

	if !isAllowed {
		logger.Debug("Ignored message from %s (candidates: %v)", phoneNumber, candidateIDs)
		return
	}
	// ─── Extract text & image ────────────────────────────
	text := msg.Message.GetConversation()
	if text == "" {
		if ext := msg.Message.GetExtendedTextMessage(); ext != nil {
			text = ext.GetText()
		}
	}

	// Download image if present
	var imageBase64 string
	if imgMsg := msg.Message.GetImageMessage(); imgMsg != nil {
		data, err := wc.client.Download(context.Background(), imgMsg)
		if err != nil {
			logger.Error("Download image failed: %v", err)
		} else {
			imageBase64 = base64.StdEncoding.EncodeToString(data)
			logger.Debug("Image downloaded: %d bytes", len(data))
		}
	}

	// Skip if no text and no image
	if text == "" && imageBase64 == "" {
		return
	}

	// ─── Skip pesan lama (dari sebelum bot connect) ─────
	if !wc.connectTime.IsZero() && msg.Info.Timestamp.Before(wc.connectTime) {
		logger.Debug("Skipped old message from %s (%s)", phoneNumber, msg.Info.Timestamp.Format(time.TimeOnly))
		return
	}

	logger.Info("Message from %s: %s...", phoneNumber, truncate(text, 50))

	chatJID := msg.Info.Chat

	// ─── Keepalive typing indicator ─────────────────────
	typingDone := make(chan struct{})
	defer close(typingDone)
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		wc.client.SendChatPresence(context.Background(), chatJID, types.ChatPresenceComposing, types.ChatPresenceMediaText)
		for {
			select {
			case <-ticker.C:
				wc.client.SendChatPresence(context.Background(), chatJID, types.ChatPresenceComposing, types.ChatPresenceMediaText)
			case <-typingDone:
				return
			}
		}
	}()

	// Process with AI
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	response, err := wc.handler.Handle(ctx, phone, text, imageBase64)
	if err != nil {
		logger.Error("Handle message: %v", err)
		response = "Maaf, ada kendala teknis. Coba tanya lagi ya!"
	}

	// Stop typing, kirim pesan
	_ = wc.client.SendChatPresence(context.Background(), chatJID, types.ChatPresencePaused, types.ChatPresenceMediaText)

	formatted := format.ToWhatsApp(response)
	_, err = wc.client.SendMessage(ctx, chatJID, &waProto.Message{
		Conversation: proto.String(formatted),
	})
	if err != nil {
		logger.Error("Send message: %v", err)
	} else {
		logger.Info("Reply sent to %s", phoneNumber)
	}
}

func truncate(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n])
}
