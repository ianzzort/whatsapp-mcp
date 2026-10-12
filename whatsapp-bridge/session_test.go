package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"go.mau.fi/whatsmeow/store/sqlstore"
	waLog "go.mau.fi/whatsmeow/util/log"
)

func newTestSession(t *testing.T) (*SessionManager, http.Handler) {
	t.Helper()
	dir := t.TempDir()
	logger := waLog.Noop
	container, err := sqlstore.New(context.Background(), "sqlite3", "file:"+filepath.Join(dir, "wa.db")+"?_foreign_keys=on", logger)
	if err != nil {
		t.Fatal(err)
	}
	sess := NewSessionManager(container, nil, logger)
	if sess.Install(container.NewDevice()) == nil {
		t.Fatal("client not created")
	}
	mux := http.NewServeMux()
	sess.registerRoutes(mux)
	return sess, authMiddleware("secret", mux)
}

func call(h http.Handler, method, path, auth string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, nil)
	if auth != "" {
		req.Header.Set("Authorization", auth)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestSessionWithoutLinkedDevice(t *testing.T) {
	_, h := newTestSession(t)
	rec := call(h, http.MethodGet, "/api/session", "Bearer secret")
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	var body map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["state"] != "disconnected" || body["loggedIn"] != false || body["phone"] != nil || body["pushName"] != nil {
		t.Fatalf("unexpected body %v", body)
	}
}

func TestLogoutWithoutSessionIsControlled(t *testing.T) {
	_, h := newTestSession(t)
	rec := call(h, http.MethodPost, "/api/session/logout", "Bearer secret")
	if rec.Code != http.StatusConflict {
		t.Fatalf("status %d", rec.Code)
	}
	if call(h, http.MethodGet, "/api/session/logout", "Bearer secret").Code != http.StatusMethodNotAllowed {
		t.Fatal("GET logout should be rejected")
	}
}

func TestSessionRoutesRequireBearer(t *testing.T) {
	_, h := newTestSession(t)
	for _, path := range []string{"/api/session", "/api/session/qr"} {
		if call(h, http.MethodGet, path, "").Code != http.StatusUnauthorized {
			t.Fatalf("%s should require auth", path)
		}
	}
}

func TestQRStateAndExpiry(t *testing.T) {
	sess, h := newTestSession(t)
	rec := call(h, http.MethodGet, "/api/session/qr", "Bearer secret")
	if rec.Body.String() == "" || rec.Code != http.StatusOK {
		t.Fatal("qr endpoint failed")
	}
	var idle qrView
	_ = json.Unmarshal(rec.Body.Bytes(), &idle)
	if idle.State != "none" || idle.Code != nil {
		t.Fatalf("unexpected idle %v", idle)
	}
	sess.mu.Lock()
	sess.pairing = true
	sess.mu.Unlock()
	sess.setQR("2@abc", time.Now().Add(20*time.Second))
	var live qrView
	_ = json.Unmarshal(call(h, http.MethodGet, "/api/session/qr", "Bearer secret").Body.Bytes(), &live)
	if live.State != "pairing" || live.Code == nil || *live.Code != "2@abc" || live.ExpiresAt == nil {
		t.Fatalf("unexpected live %v", live)
	}
	sess.setQR("2@abc", time.Now().Add(-time.Second))
	var expired qrView
	_ = json.Unmarshal(call(h, http.MethodGet, "/api/session/qr", "Bearer secret").Body.Bytes(), &expired)
	if expired.State != "pairing" || expired.Code != nil {
		t.Fatalf("unexpected expired %v", expired)
	}
}
