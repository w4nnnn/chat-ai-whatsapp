package whatsapp

import (
	"context"
	"database/sql"
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
	"chat-ai-whatsapp/internal/handler"
	"chat-ai-whatsapp/internal/logger"

	"github.com/mdp/qrterminal/v3"
	_ "modernc.org/sqlite"
)

type Client struct {
	client  *whatsmeow.Client
	handler *handler.Handler
	config  *config.Config
}

func New(cfg *config.Config, dbPath string, msgHandler *handler.Handler) (*Client, error) {
	ctx := context.Background()
	db, err := sql.Open("sqlite", "file:"+dbPath)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	// Enable foreign keys and WAL mode
	for _, pragma := range []string{
		"PRAGMA foreign_keys = ON",
		"PRAGMA journal_mode = WAL",
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
		logger.Info("WhatsApp connected!")
	case *events.Disconnected:
		logger.Warn("WhatsApp disconnected")
	case *events.LoggedOut:
		logger.Error("WhatsApp logged out — delete session and re-pair")
	}
}

func (wc *Client) handleMessage(msg *events.Message) {
	// ─── Filter: self messages ──────────────────────────
	if !wc.config.SelfRespon && msg.Info.IsFromMe {
		return
	}

	// ─── Filter: group chats ────────────────────────────
	if msg.Info.Chat.Server == "g.us" {
		return
	}

	// ─── Filter: allowed numbers ─────────────────────────
	phone := msg.Info.Sender.String()
	if phone == "" {
		return
	}
	phoneNumber := strings.Split(phone, "@")[0] // ambil nomor aja tanpa @s.whatsapp.net
	isAllowed := false
	for _, a := range wc.config.AllowedNumbers {
		if a == "*" || a == phoneNumber {
			isAllowed = true
			break
		}
	}
	if !isAllowed {
		logger.Debug("Ignored message from %s (not in allowed list)", phoneNumber)
		return
	}

	// ─── Extract text content ────────────────────────────
	text := msg.Message.GetConversation()
	if text == "" {
		if ext := msg.Message.GetExtendedTextMessage(); ext != nil {
			text = ext.GetText()
		}
	}
	if text == "" {
		return
	}

	logger.Info("Message from %s: %s...", phoneNumber, truncate(text, 50))

	// Send typing indicator
	_ = wc.client.SendChatPresence(context.Background(), msg.Info.Sender, types.ChatPresenceComposing, types.ChatPresenceMediaText)

	// Process with AI
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	response, err := wc.handler.Handle(ctx, phone, text)
	if err != nil {
		logger.Error("Handle message: %v", err)
		response = "Maaf, ada kendala teknis. Coba tanya lagi ya!"
	}

	// Send reply
	jid := msg.Info.Sender
	_, err = wc.client.SendMessage(ctx, jid, &waProto.Message{
		Conversation: proto.String(response),
	})
	if err != nil {
		logger.Error("Send message: %v", err)
	} else {
		logger.Info("Reply sent to %s", phone)
	}
}

func truncate(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n])
}
