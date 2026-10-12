package main

import (
	"context"
	"net/http"
	"sync"
	"time"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"
)

const (
	sessionConnected    = "connected"
	sessionDisconnected = "disconnected"
	sessionPairing      = "pairing"
	sessionLoggingOut   = "logging_out"
)

type SessionManager struct {
	mu         sync.Mutex
	container  *sqlstore.Container
	store      *MessageStore
	logger     waLog.Logger
	client     *whatsmeow.Client
	loggingOut bool
	pairing    bool
	qrCode     string
	qrExpires  time.Time
}

func NewSessionManager(container *sqlstore.Container, messageStore *MessageStore, logger waLog.Logger) *SessionManager {
	return &SessionManager{container: container, store: messageStore, logger: logger}
}

func (s *SessionManager) Client() *whatsmeow.Client {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.client
}

func (s *SessionManager) Install(device *store.Device) *whatsmeow.Client {
	client := whatsmeow.NewClient(device, s.logger)
	if client == nil {
		return nil
	}
	client.AddEventHandler(func(evt interface{}) {
		switch v := evt.(type) {
		case *events.Message:
			handleMessage(client, s.store, v, s.logger)
		case *events.HistorySync:
			handleHistorySync(client, s.store, v, s.logger)
		case *events.Connected:
			s.logger.Infof("Connected to WhatsApp")
			handleConnected()
		case *events.Disconnected:
			s.logger.Warnf("Disconnected from WhatsApp")
			handleDisconnected(client, s.logger)
		case *events.LoggedOut:
			handleLoggedOut(s.logger)
		}
	})
	s.mu.Lock()
	s.client = client
	s.mu.Unlock()
	return client
}

type sessionView struct {
	State    string  `json:"state"`
	LoggedIn bool    `json:"loggedIn"`
	Phone    *string `json:"phone"`
	PushName *string `json:"pushName"`
}

func (s *SessionManager) View() sessionView {
	s.mu.Lock()
	client := s.client
	loggingOut := s.loggingOut
	pairing := s.pairing
	s.mu.Unlock()
	view := sessionView{State: sessionDisconnected}
	if client == nil {
		return view
	}
	if client.Store != nil && client.Store.ID != nil {
		phone := client.Store.ID.User
		view.Phone = &phone
		if client.Store.PushName != "" {
			name := client.Store.PushName
			view.PushName = &name
		}
	}
	view.LoggedIn = client.IsLoggedIn()
	switch {
	case loggingOut:
		view.State = sessionLoggingOut
	case pairing && !view.LoggedIn:
		view.State = sessionPairing
	case view.LoggedIn:
		view.State = sessionConnected
	}
	return view
}

type qrView struct {
	State     string  `json:"state"`
	Code      *string `json:"code"`
	ExpiresAt *string `json:"expiresAt"`
}

func (s *SessionManager) QR() qrView {
	view := s.View()
	out := qrView{State: "none"}
	switch view.State {
	case sessionConnected:
		out.State = "connected"
	case sessionPairing:
		out.State = "pairing"
		s.mu.Lock()
		if s.qrCode != "" && time.Now().Before(s.qrExpires) {
			code := s.qrCode
			expires := s.qrExpires.UTC().Format(time.RFC3339)
			out.Code = &code
			out.ExpiresAt = &expires
		}
		s.mu.Unlock()
	}
	return out
}

func (s *SessionManager) setQR(code string, expires time.Time) {
	s.mu.Lock()
	s.qrCode = code
	s.qrExpires = expires
	s.mu.Unlock()
}

func (s *SessionManager) endPairing() {
	s.mu.Lock()
	s.pairing = false
	s.qrCode = ""
	s.qrExpires = time.Time{}
	s.mu.Unlock()
}

type logoutError struct {
	status  int
	message string
}

func (e *logoutError) Error() string { return e.message }

func (s *SessionManager) Logout() error {
	s.mu.Lock()
	if s.loggingOut {
		s.mu.Unlock()
		return &logoutError{http.StatusConflict, "Logout already in progress"}
	}
	old := s.client
	if old == nil || old.Store == nil || old.Store.ID == nil {
		s.mu.Unlock()
		return &logoutError{http.StatusConflict, "No linked session"}
	}
	s.loggingOut = true
	s.pairing = false
	s.qrCode = ""
	s.qrExpires = time.Time{}
	s.mu.Unlock()

	finish := func() {
		s.mu.Lock()
		s.loggingOut = false
		s.mu.Unlock()
	}
	if err := old.Logout(context.Background()); err != nil {
		finish()
		return &logoutError{http.StatusBadGateway, "Logout failed: " + err.Error()}
	}
	old.Disconnect()
	client := s.Install(s.container.NewDevice())
	if client == nil {
		finish()
		return &logoutError{http.StatusInternalServerError, "Could not create a new client"}
	}
	if err := s.startPairing(client); err != nil {
		finish()
		return &logoutError{http.StatusInternalServerError, "Could not start pairing: " + err.Error()}
	}
	finish()
	return nil
}

func (s *SessionManager) startPairing(client *whatsmeow.Client) error {
	qrChan, err := client.GetQRChannel(context.Background())
	if err != nil {
		return err
	}
	if err := client.Connect(); err != nil {
		return err
	}
	s.mu.Lock()
	s.pairing = true
	s.mu.Unlock()
	go s.watchQR(client, qrChan)
	return nil
}

func (s *SessionManager) watchQR(client *whatsmeow.Client, qrChan <-chan whatsmeow.QRChannelItem) {
	for evt := range qrChan {
		if s.Client() != client {
			return
		}
		switch evt.Event {
		case "code":
			s.setQR(evt.Code, time.Now().Add(evt.Timeout))
		case "success":
			s.endPairing()
			s.logger.Infof("Pairing completed")
			return
		case "timeout":
			s.setQR("", time.Time{})
			client.Disconnect()
			next, err := client.GetQRChannel(context.Background())
			if err == nil {
				err = client.Connect()
			}
			if err != nil {
				s.logger.Warnf("Could not restart pairing: %v", err)
				s.endPairing()
				return
			}
			go s.watchQR(client, next)
			return
		default:
			s.logger.Warnf("Pairing ended with event %s", evt.Event)
			s.endPairing()
			return
		}
	}
	s.endPairing()
}

func (s *SessionManager) registerRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/session", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		writeJSON(w, http.StatusOK, s.View())
	})
	mux.HandleFunc("/api/session/qr", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		writeJSON(w, http.StatusOK, s.QR())
	})
	mux.HandleFunc("/api/session/logout", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if err := s.Logout(); err != nil {
			status := http.StatusInternalServerError
			if le, ok := err.(*logoutError); ok {
				status = le.status
			}
			writeJSON(w, status, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"state": sessionPairing})
	})
}
