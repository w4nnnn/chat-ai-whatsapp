package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"chat-ai-whatsapp/internal/ai"
	"chat-ai-whatsapp/internal/logger"
	"chat-ai-whatsapp/internal/search"
	"chat-ai-whatsapp/internal/store"
)

const systemPrompt = `Kamu adalah asisten belajar untuk siswa sekolah di Indonesia.
Gunakan bahasa Indonesia yang baik.
Jika butuh informasi terbaru, gunakan tool search_web.
Sertakan sumber jika merujuk pada data spesifik.

Jika siswa mengirim gambar, analisis gambar tersebut dan bantu menjawab pertanyaan terkait.
Bersikap sabar dan ramah layaknya guru yang membantu murid.

Format output kamu HARUS seperti ini:
===<enter>
<jawaban singkat dan padat untuk soal><enter>
===<enter>
<penjelasan singkat><enter>
===`

type Handler struct {
	ai      *ai.Client
	store   *store.Store
	search  *search.Client
}

func New(aiClient *ai.Client, redisStore *store.Store, searchClient *search.Client) *Handler {
	return &Handler{
		ai:     aiClient,
		store:  redisStore,
		search: searchClient,
	}
}

// Handle processes a message and returns the AI response.
// If imageBase64 is not empty, it's sent as a multimodal message.
func (h *Handler) Handle(ctx context.Context, phone, text string, imageBase64 string) (string, error) {
	// 1. Rate limit check
	allowed, err := h.store.CheckRateLimit(ctx, phone)
	if err != nil {
		logger.Error("Rate limit check failed: %v", err)
	}
	if !allowed {
		return "Mohon tunggu sebentar ya, kamu terlalu cepat! 😊 Coba lagi dalam 1 menit.", nil
	}

	// 2. Get session history
	history, err := h.store.GetMessages(ctx, phone)
	if err != nil {
		return "", fmt.Errorf("get history: %w", err)
	}
	logger.Debug("Session history for %s: %d messages", phone, len(history))

	// 3. Build messages
	messages := []ai.Message{
		{Role: "system", Content: systemPrompt},
	}
	for _, m := range history {
		messages = append(messages, ai.Message{Role: m.Role, Content: m.Content})
	}

	// Build user content — support multimodal (text + image)
	var userContent any = text
	if imageBase64 != "" {
		caption := text
		if caption == "" {
			caption = "Analisis gambar ini"
		}
		parts := []ai.ContentPart{
			{Type: "text", Text: caption},
			{
				Type: "image_url",
				ImageURL: &ai.ImageURLPart{
					URL: "data:image/jpeg;base64," + imageBase64,
				},
			},
		}
		userContent = parts
	}
	messages = append(messages, ai.Message{Role: "user", Content: userContent})

	// 4. Call AI with retry (3 attempts)
	var response string
	for attempt := 1; attempt <= 5; attempt++ {
		var toolCalls []ai.ToolCall
		var callErr error

		response, toolCalls, callErr = h.ai.Chat(ctx, messages)
		if callErr != nil {
			logger.Error("AI call failed (attempt %d/3): %v", attempt, callErr)
			if attempt == 3 {
				return "", fmt.Errorf("ai call failed: %w", callErr)
			}
			continue
		}

		// Handle tool calls
		if len(toolCalls) > 0 {
			messages = append(messages, ai.Message{
				Role:    "assistant",
				Content: response,
			})

			for _, tc := range toolCalls {
				result := h.executeTool(ctx, tc)
				messages = append(messages, ai.Message{
					Role:       "tool",
					ToolCallID: tc.ID,
					Content:    result,
				})
			}

			// Call again with tool results
			response, toolCalls, callErr = h.ai.Chat(ctx, messages)
			if callErr != nil {
				logger.Error("AI call (tool result) failed (attempt %d/3): %v", attempt, callErr)
				if attempt == 3 {
					return "", fmt.Errorf("ai call with tools failed: %w", callErr)
				}
				continue
			}
		}

		response = strings.TrimSpace(response)
		if response != "" {
			break
		}
		logger.Warn("AI returned empty response (attempt %d/3)", attempt)
	}

	if response == "" {
		response = "Maaf, saya tidak bisa menjawab pertanyaan ini saat ini. Coba tanya dengan cara lain ya!"
	}

	logger.Debug("AI response: %s...", truncate(response, 100))

	// 5. Save to Redis
	userMsg := store.Message{
		Role:      "user",
		Content:   text,
		Timestamp: time.Now().UnixMilli(),
	}
	aiMsg := store.Message{
		Role:      "assistant",
		Content:   response,
		Timestamp: time.Now().UnixMilli(),
	}

	if err := h.store.AddMessage(ctx, phone, userMsg); err != nil {
		logger.Error("Save user message: %v", err)
	}
	if err := h.store.AddMessage(ctx, phone, aiMsg); err != nil {
		logger.Error("Save AI message: %v", err)
	}

	return response, nil
}

func (h *Handler) executeTool(ctx context.Context, tc ai.ToolCall) string {
	logger.Info("Executing tool: %s (args: %s)", tc.Function.Name, tc.Function.Arguments)

	switch tc.Function.Name {
	case "search_web":
		var args struct {
			Query string `json:"query"`
		}
		if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
			return fmt.Sprintf("Error parsing search query: %v", err)
		}
		result, err := h.search.Search(ctx, args.Query)
		if err != nil {
			logger.Error("Search failed: %v", err)
			return "Pencarian gagal. Silakan coba lagi nanti."
		}
		return result
	default:
		return fmt.Sprintf("Unknown tool: %s", tc.Function.Name)
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
