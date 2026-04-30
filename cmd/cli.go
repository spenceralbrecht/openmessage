package cmd

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"go.mau.fi/mautrix-gmessages/pkg/libgm/gmproto"

	"github.com/maxghenis/openmessage/internal/app"
	"github.com/maxghenis/openmessage/internal/db"
)

const maxCLILimit = 200

type conversationOutput struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	IsGroup          bool   `json:"is_group"`
	Participants     any    `json:"participants"`
	LastMessageTS    int64  `json:"last_message_ts"`
	UnreadCount      int    `json:"unread_count"`
	SourcePlatform   string `json:"source_platform"`
	NotificationMode string `json:"notification_mode"`
}

type messageOutput struct {
	ID             string `json:"id"`
	ConversationID string `json:"conversation_id"`
	SenderName     string `json:"sender_name"`
	SenderNumber   string `json:"sender_number"`
	Body           string `json:"body"`
	TimestampMS    int64  `json:"timestamp_ms"`
	Status         string `json:"status"`
	IsFromMe       bool   `json:"is_from_me"`
	MentionsMe     bool   `json:"mentions_me,omitempty"`
	MediaID        string `json:"media_id,omitempty"`
	MimeType       string `json:"mime_type,omitempty"`
	Reactions      string `json:"reactions,omitempty"`
	ReplyToID      string `json:"reply_to_id,omitempty"`
	SourcePlatform string `json:"source_platform"`
	SourceID       string `json:"source_id,omitempty"`
}

type draftOutput struct {
	ID             string `json:"id"`
	ConversationID string `json:"conversation_id"`
	Body           string `json:"body"`
	CreatedAt      int64  `json:"created_at"`
}

func RunConversations(logger zerolog.Logger, args ...string) error {
	fs := flag.NewFlagSet("conversations", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	limit := fs.Int("limit", 20, "maximum number of conversations to return")
	platform := fs.String("platform", "", "optional source platform filter")
	if err := fs.Parse(args); err != nil {
		return err
	}

	a, err := app.New(logger)
	if err != nil {
		return fmt.Errorf("init app: %w", err)
	}
	defer a.Close()

	n := normalizeCLILimit(*limit)
	var convs []*db.Conversation
	if strings.TrimSpace(*platform) != "" {
		convs, err = a.Store.ListConversationsByPlatform(strings.TrimSpace(*platform), n)
	} else {
		convs, err = a.Store.ListConversations(n)
	}
	if err != nil {
		return fmt.Errorf("list conversations: %w", err)
	}
	return writeJSON(os.Stdout, map[string]any{
		"conversations": mapConversations(convs),
		"limit":         n,
	})
}

func RunMessages(logger zerolog.Logger, args ...string) error {
	fs := flag.NewFlagSet("messages", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	limit := fs.Int("limit", 20, "maximum number of messages to return")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() < 1 {
		return fmt.Errorf("usage: openmessage messages <conversation_id> [--limit n]")
	}
	conversationID := fs.Arg(0)

	a, err := app.New(logger)
	if err != nil {
		return fmt.Errorf("init app: %w", err)
	}
	defer a.Close()

	n := normalizeCLILimit(*limit)
	msgs, err := a.Store.GetMessagesByConversation(conversationID, n)
	if err != nil {
		return fmt.Errorf("get messages: %w", err)
	}
	return writeJSON(os.Stdout, map[string]any{
		"conversation_id": conversationID,
		"messages":        mapMessages(msgs),
		"limit":           n,
	})
}

func RunSearch(logger zerolog.Logger, args ...string) error {
	fs := flag.NewFlagSet("search", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	limit := fs.Int("limit", 20, "maximum number of matching messages to return")
	phone := fs.String("phone", "", "optional sender phone/identifier filter")
	if err := fs.Parse(args); err != nil {
		return err
	}
	query := strings.TrimSpace(strings.Join(fs.Args(), " "))
	if query == "" {
		return fmt.Errorf("usage: openmessage search <query> [--limit n] [--phone identifier]")
	}

	a, err := app.New(logger)
	if err != nil {
		return fmt.Errorf("init app: %w", err)
	}
	defer a.Close()

	n := normalizeCLILimit(*limit)
	msgs, err := a.Store.SearchMessages(query, strings.TrimSpace(*phone), n)
	if err != nil {
		return fmt.Errorf("search messages: %w", err)
	}
	return writeJSON(os.Stdout, map[string]any{
		"query":    query,
		"messages": mapMessages(msgs),
		"limit":    n,
	})
}

func RunDraft(logger zerolog.Logger, args ...string) error {
	fs := flag.NewFlagSet("draft", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() < 2 {
		return fmt.Errorf("usage: openmessage draft <conversation_id> <message>")
	}
	conversationID := fs.Arg(0)
	body := strings.TrimSpace(strings.Join(fs.Args()[1:], " "))
	if body == "" {
		return fmt.Errorf("draft body is required")
	}

	a, err := app.New(logger)
	if err != nil {
		return fmt.Errorf("init app: %w", err)
	}
	defer a.Close()
	if conv, err := a.Store.GetConversation(conversationID); err != nil {
		return fmt.Errorf("get conversation: %w", err)
	} else if conv == nil {
		return fmt.Errorf("conversation %s not found", conversationID)
	}

	now := time.Now().UnixMilli()
	draftID, err := newDraftID()
	if err != nil {
		return err
	}
	draft := &db.Draft{
		DraftID:        draftID,
		ConversationID: conversationID,
		Body:           body,
		CreatedAt:      now,
	}
	if err := a.Store.UpsertDraft(draft); err != nil {
		return fmt.Errorf("create draft: %w", err)
	}
	return writeJSON(os.Stdout, map[string]any{
		"draft": mapDraft(draft),
	})
}

func RunDrafts(logger zerolog.Logger, args ...string) error {
	fs := flag.NewFlagSet("drafts", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() < 1 {
		return fmt.Errorf("usage: openmessage drafts <conversation_id>")
	}
	conversationID := fs.Arg(0)

	a, err := app.New(logger)
	if err != nil {
		return fmt.Errorf("init app: %w", err)
	}
	defer a.Close()

	if conv, err := a.Store.GetConversation(conversationID); err != nil {
		return fmt.Errorf("get conversation: %w", err)
	} else if conv == nil {
		return fmt.Errorf("conversation %s not found", conversationID)
	}
	drafts, err := a.Store.ListDrafts(conversationID)
	if err != nil {
		return fmt.Errorf("list drafts: %w", err)
	}
	out := make([]draftOutput, 0, len(drafts))
	for _, draft := range drafts {
		out = append(out, mapDraft(draft))
	}
	return writeJSON(os.Stdout, map[string]any{
		"conversation_id": conversationID,
		"drafts":          out,
	})
}

func RunSendDraft(logger zerolog.Logger, args ...string) error {
	fs := flag.NewFlagSet("send-draft", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	confirm := fs.String("confirm", "", "required confirmation; must exactly match the draft id")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() < 1 {
		return fmt.Errorf("usage: openmessage send-draft <draft_id> --confirm <draft_id>")
	}
	draftID := fs.Arg(0)
	if strings.TrimSpace(*confirm) != draftID {
		return fmt.Errorf("refusing to send draft without --confirm %s", draftID)
	}

	a, err := app.New(logger)
	if err != nil {
		return fmt.Errorf("init app: %w", err)
	}
	defer a.Close()

	draft, err := a.Store.GetDraft(draftID)
	if err != nil {
		return fmt.Errorf("get draft: %w", err)
	}
	if draft == nil {
		return fmt.Errorf("draft %s not found", draftID)
	}
	conv, err := a.Store.GetConversation(draft.ConversationID)
	if err != nil {
		return fmt.Errorf("get conversation: %w", err)
	}
	if conv == nil {
		return fmt.Errorf("conversation %s not found", draft.ConversationID)
	}

	result, err := sendDraftMessage(a, conv, draft)
	if err != nil {
		return err
	}
	return writeJSON(os.Stdout, map[string]any{
		"sent":   true,
		"draft":  mapDraft(draft),
		"result": result,
	})
}

func sendDraftMessage(a *app.App, conv *db.Conversation, draft *db.Draft) (map[string]any, error) {
	switch strings.ToLower(strings.TrimSpace(conv.SourcePlatform)) {
	case "whatsapp":
		if err := a.LoadAndConnectWhatsApp(); err != nil {
			return nil, fmt.Errorf("connect whatsapp: %w", err)
		}
		msg, err := a.SendWhatsAppText(draft.ConversationID, draft.Body, "")
		if err != nil {
			return nil, fmt.Errorf("send whatsapp draft: %w", err)
		}
		if err := a.Store.RecordOutgoingMessage(msg, draft.DraftID); err != nil {
			return nil, fmt.Errorf("record whatsapp draft send: %w", err)
		}
		return map[string]any{"message_id": msg.MessageID, "platform": "whatsapp"}, nil
	case "signal":
		if err := a.LoadAndConnectSignal(); err != nil {
			return nil, fmt.Errorf("connect signal: %w", err)
		}
		msg, err := a.SendSignalText(draft.ConversationID, draft.Body, "")
		if err != nil {
			return nil, fmt.Errorf("send signal draft: %w", err)
		}
		if err := a.Store.RecordOutgoingMessage(msg, draft.DraftID); err != nil {
			return nil, fmt.Errorf("record signal draft send: %w", err)
		}
		return map[string]any{"message_id": msg.MessageID, "platform": "signal"}, nil
	default:
		if err := a.LoadAndConnect(); err != nil {
			return nil, fmt.Errorf("connect google messages: %w", err)
		}
		cli := a.GetClient()
		if cli == nil {
			return nil, fmt.Errorf("client not connected")
		}
		gmConv, err := cli.GM.GetConversation(draft.ConversationID)
		if err != nil {
			return nil, fmt.Errorf("get google conversation: %w", err)
		}
		myParticipantID, simPayload := app.ExtractSIMAndParticipant(gmConv)
		payload := app.BuildSendPayload(draft.ConversationID, draft.Body, "", myParticipantID, simPayload)
		resp, err := cli.GM.SendMessage(payload)
		if err != nil {
			return nil, fmt.Errorf("send google draft: %w", err)
		}
		success := resp.GetStatus() == gmproto.SendMessageResponse_SUCCESS
		now := time.Now().UnixMilli()
		status := "OUTGOING_SENDING"
		deleteDraftID := draft.DraftID
		if !success {
			status = "OUTGOING_FAILED:" + resp.GetStatus().String()
			deleteDraftID = ""
		}
		if err := a.Store.RecordOutgoingMessage(&db.Message{
			MessageID:      payload.TmpID,
			ConversationID: draft.ConversationID,
			Body:           draft.Body,
			IsFromMe:       true,
			TimestampMS:    now,
			Status:         status,
			SourcePlatform: firstNonEmpty(conv.SourcePlatform, "sms"),
		}, deleteDraftID); err != nil {
			return nil, fmt.Errorf("record outgoing message: %w", err)
		}
		if !success {
			return nil, fmt.Errorf("send failed with status %s", resp.GetStatus().String())
		}
		return map[string]any{"message_id": payload.TmpID, "platform": firstNonEmpty(conv.SourcePlatform, "sms"), "status": resp.GetStatus().String()}, nil
	}
}

func normalizeCLILimit(limit int) int {
	if limit <= 0 {
		return 20
	}
	if limit > maxCLILimit {
		return maxCLILimit
	}
	return limit
}

func writeJSON(out *os.File, value any) error {
	enc := json.NewEncoder(out)
	enc.SetEscapeHTML(false)
	return enc.Encode(value)
}

func mapConversations(convs []*db.Conversation) []conversationOutput {
	out := make([]conversationOutput, 0, len(convs))
	for _, conv := range convs {
		out = append(out, conversationOutput{
			ID:               conv.ConversationID,
			Name:             conv.Name,
			IsGroup:          conv.IsGroup,
			Participants:     parseParticipants(conv.Participants),
			LastMessageTS:    conv.LastMessageTS,
			UnreadCount:      conv.UnreadCount,
			SourcePlatform:   firstNonEmpty(conv.SourcePlatform, "sms"),
			NotificationMode: conv.NotificationMode,
		})
	}
	return out
}

func mapMessages(msgs []*db.Message) []messageOutput {
	out := make([]messageOutput, 0, len(msgs))
	for _, msg := range msgs {
		out = append(out, messageOutput{
			ID:             msg.MessageID,
			ConversationID: msg.ConversationID,
			SenderName:     msg.SenderName,
			SenderNumber:   msg.SenderNumber,
			Body:           msg.Body,
			TimestampMS:    msg.TimestampMS,
			Status:         msg.Status,
			IsFromMe:       msg.IsFromMe,
			MentionsMe:     msg.MentionsMe,
			MediaID:        msg.MediaID,
			MimeType:       msg.MimeType,
			Reactions:      msg.Reactions,
			ReplyToID:      msg.ReplyToID,
			SourcePlatform: firstNonEmpty(msg.SourcePlatform, "sms"),
			SourceID:       msg.SourceID,
		})
	}
	return out
}

func mapDraft(draft *db.Draft) draftOutput {
	return draftOutput{
		ID:             draft.DraftID,
		ConversationID: draft.ConversationID,
		Body:           draft.Body,
		CreatedAt:      draft.CreatedAt,
	}
}

func parseParticipants(raw string) any {
	var parsed any
	if err := json.Unmarshal([]byte(raw), &parsed); err == nil {
		return parsed
	}
	return raw
}

func newDraftID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("create draft id: %w", err)
	}
	return "draft_" + hex.EncodeToString(b[:]), nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
