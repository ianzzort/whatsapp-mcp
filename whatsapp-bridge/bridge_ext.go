package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"go.mau.fi/whatsmeow"
	waProto "go.mau.fi/whatsmeow/binary/proto"
	"go.mau.fi/whatsmeow/types"
	waLog "go.mau.fi/whatsmeow/util/log"
)

type MsgContext struct {
	ReplyToID string
	Mentions  string
	PushName  string
}

func contextInfoOf(msg *waProto.Message) *waProto.ContextInfo {
	if msg == nil {
		return nil
	}
	switch {
	case msg.GetExtendedTextMessage() != nil:
		return msg.GetExtendedTextMessage().GetContextInfo()
	case msg.GetImageMessage() != nil:
		return msg.GetImageMessage().GetContextInfo()
	case msg.GetVideoMessage() != nil:
		return msg.GetVideoMessage().GetContextInfo()
	case msg.GetDocumentMessage() != nil:
		return msg.GetDocumentMessage().GetContextInfo()
	case msg.GetAudioMessage() != nil:
		return msg.GetAudioMessage().GetContextInfo()
	}
	return nil
}

func buildMsgContext(msg *waProto.Message, pushName string) MsgContext {
	mc := MsgContext{PushName: pushName}
	ci := contextInfoOf(msg)
	if ci == nil {
		return mc
	}
	mc.ReplyToID = ci.GetStanzaID()
	if mentioned := ci.GetMentionedJID(); len(mentioned) > 0 {
		if data, err := json.Marshal(mentioned); err == nil {
			mc.Mentions = string(data)
		}
	}
	return mc
}

func (store *MessageStore) migrate() error {
	for _, col := range []string{"reply_to_id TEXT", "mentions TEXT", "push_name TEXT"} {
		if _, err := store.db.Exec("ALTER TABLE messages ADD COLUMN " + col); err != nil &&
			!strings.Contains(strings.ToLower(err.Error()), "duplicate column") {
			return err
		}
	}
	_, err := store.db.Exec("CREATE INDEX IF NOT EXISTS idx_messages_chat_time ON messages(chat_jid, timestamp)")
	return err
}

type SendLimiter struct {
	mu          sync.Mutex
	perRecip    time.Duration
	perMinute   int
	lastByRecip map[string]time.Time
	window      []time.Time
}

func NewSendLimiter(perRecip time.Duration, perMinute int) *SendLimiter {
	return &SendLimiter{perRecip: perRecip, perMinute: perMinute, lastByRecip: map[string]time.Time{}}
}

func (l *SendLimiter) Allow(recipient string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if last, ok := l.lastByRecip[recipient]; ok && now.Sub(last) < l.perRecip {
		return false
	}
	cutoff := now.Add(-time.Minute)
	kept := l.window[:0]
	for _, t := range l.window {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	l.window = kept
	if len(l.window) >= l.perMinute {
		return false
	}
	l.window = append(l.window, now)
	l.lastByRecip[recipient] = now
	for k, t := range l.lastByRecip {
		if now.Sub(t) > time.Minute {
			delete(l.lastByRecip, k)
		}
	}
	return true
}

func bearerOK(header, token string) bool {
	if token == "" {
		return true
	}
	return header == "Bearer "+token
}

func authMiddleware(token string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") && !bearerOK(r.Header.Get("Authorization"), token) {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

type BridgeState struct {
	mu             sync.Mutex
	startedAt      time.Time
	lastMessageAt  time.Time
	disconnectedAt time.Time
	alerted        bool
	reconnecting   bool
}

var bridgeState = &BridgeState{startedAt: time.Now()}

func (s *BridgeState) touchMessage(t time.Time) {
	s.mu.Lock()
	if t.After(s.lastMessageAt) {
		s.lastMessageAt = t
	}
	s.mu.Unlock()
}

func sendAlert(event, message string, logger waLog.Logger) {
	logger.Warnf("ALERT %s: %s", event, message)
	url := os.Getenv("WHATSAPP_ALERT_WEBHOOK")
	if url == "" {
		return
	}
	body, _ := json.Marshal(map[string]string{
		"event":   event,
		"at":      time.Now().UTC().Format(time.RFC3339),
		"message": message,
	})
	httpClient := &http.Client{Timeout: 10 * time.Second}
	resp, err := httpClient.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		logger.Warnf("Alert webhook failed: %v", err)
		return
	}
	resp.Body.Close()
}

func nextBackoff(d time.Duration) time.Duration {
	d *= 2
	if d > 60*time.Second {
		d = 60 * time.Second
	}
	return d
}

func handleDisconnected(client *whatsmeow.Client, logger waLog.Logger) {
	s := bridgeState
	s.mu.Lock()
	if s.disconnectedAt.IsZero() {
		s.disconnectedAt = time.Now()
	}
	if s.reconnecting {
		s.mu.Unlock()
		return
	}
	s.reconnecting = true
	s.mu.Unlock()

	go func() {
		backoff := time.Second
		for {
			time.Sleep(backoff)
			if client.IsConnected() {
				break
			}
			if client.Store.ID == nil {
				break
			}
			if err := client.Connect(); err != nil {
				logger.Warnf("Reconnect failed, retrying in %s: %v", nextBackoff(backoff), err)
				backoff = nextBackoff(backoff)
			} else {
				break
			}
			s.mu.Lock()
			since := time.Since(s.disconnectedAt)
			shouldAlert := !s.alerted && !s.disconnectedAt.IsZero() && since > 5*time.Minute
			if shouldAlert {
				s.alerted = true
			}
			s.mu.Unlock()
			if shouldAlert {
				sendAlert("disconnected", fmt.Sprintf("WhatsApp disconnected for %s", since.Round(time.Second)), logger)
			}
		}
		s.mu.Lock()
		s.reconnecting = false
		s.mu.Unlock()
	}()
}

func handleConnected() {
	s := bridgeState
	s.mu.Lock()
	s.disconnectedAt = time.Time{}
	s.alerted = false
	s.mu.Unlock()
}

func handleLoggedOut(logger waLog.Logger) {
	logger.Errorf("Device logged out: the WhatsApp session is no longer valid, scan the QR code again")
	go sendAlert("logged_out", "WhatsApp session logged out, re-link the device with a new QR code", logger)
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

type groupParticipantOut struct {
	JID     string `json:"jid"`
	LID     string `json:"lid"`
	Phone   string `json:"phone"`
	IsAdmin bool   `json:"is_admin"`
}

func registerExtraRoutes(mux *http.ServeMux, client *whatsmeow.Client) {
	mux.HandleFunc("/api/health", func(w http.ResponseWriter, r *http.Request) {
		bridgeState.mu.Lock()
		last := bridgeState.lastMessageAt
		started := bridgeState.startedAt
		bridgeState.mu.Unlock()
		var lastOut interface{}
		if !last.IsZero() {
			lastOut = last.UTC().Format(time.RFC3339)
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"connected":       client.IsConnected(),
			"logged_in":       client.IsLoggedIn(),
			"uptime_seconds":  int64(time.Since(started).Seconds()),
			"last_message_at": lastOut,
		})
	})

	mux.HandleFunc("/api/groups", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		groups, err := client.GetJoinedGroups(context.Background())
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		out := make([]map[string]string, 0, len(groups))
		for _, g := range groups {
			out = append(out, map[string]string{"jid": g.JID.String(), "name": g.Name})
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"groups": out})
	})

	mux.HandleFunc("/api/groups/", func(w http.ResponseWriter, r *http.Request) {
		rest := strings.TrimPrefix(r.URL.Path, "/api/groups/")
		if r.Method != http.MethodGet || !strings.HasSuffix(rest, "/participants") {
			http.NotFound(w, r)
			return
		}
		raw := strings.TrimSuffix(rest, "/participants")
		jid, err := types.ParseJID(raw)
		if err != nil || jid.Server != types.GroupServer {
			http.Error(w, "Invalid group JID", http.StatusBadRequest)
			return
		}
		info, err := client.GetGroupInfo(context.Background(), jid)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		parts := make([]groupParticipantOut, 0, len(info.Participants))
		for _, p := range info.Participants {
			parts = append(parts, groupParticipantOut{
				JID:     p.JID.String(),
				LID:     jidString(p.LID),
				Phone:   p.PhoneNumber.User,
				IsAdmin: p.IsAdmin || p.IsSuperAdmin,
			})
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"jid":          info.JID.String(),
			"name":         info.Name,
			"participants": parts,
		})
	})
}

func jidString(j types.JID) string {
	if j.IsEmpty() {
		return ""
	}
	return j.String()
}
