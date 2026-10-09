package main

import (
	"testing"
	"time"

	waProto "go.mau.fi/whatsmeow/binary/proto"
	"google.golang.org/protobuf/proto"
)

func TestSendLimiterPerRecipient(t *testing.T) {
	l := NewSendLimiter(3*time.Second, 20)
	now := time.Now()
	if !l.Allow("a", now) {
		t.Fatal("first send should pass")
	}
	if l.Allow("a", now.Add(2*time.Second)) {
		t.Fatal("same recipient within 3s should be blocked")
	}
	if !l.Allow("a", now.Add(4*time.Second)) {
		t.Fatal("same recipient after 3s should pass")
	}
}

func TestSendLimiterPerMinute(t *testing.T) {
	l := NewSendLimiter(3*time.Second, 20)
	now := time.Now()
	for i := 0; i < 20; i++ {
		if !l.Allow(string(rune('a'+i)), now) {
			t.Fatalf("send %d should pass", i)
		}
	}
	if l.Allow("z", now.Add(time.Second)) {
		t.Fatal("21st send within a minute should be blocked")
	}
	if !l.Allow("z", now.Add(61*time.Second)) {
		t.Fatal("send after the window should pass")
	}
}

func TestBearerOK(t *testing.T) {
	if !bearerOK("", "") {
		t.Fatal("no token configured should allow")
	}
	if bearerOK("", "s3") || bearerOK("Bearer x", "s3") {
		t.Fatal("missing or wrong token should be rejected")
	}
	if !bearerOK("Bearer s3", "s3") {
		t.Fatal("correct token should pass")
	}
}

func TestBuildMsgContext(t *testing.T) {
	msg := &waProto.Message{ExtendedTextMessage: &waProto.ExtendedTextMessage{
		Text: proto.String("hi"),
		ContextInfo: &waProto.ContextInfo{
			StanzaID:     proto.String("ABC"),
			MentionedJID: []string{"1@s.whatsapp.net", "2@lid"},
		},
	}}
	mc := buildMsgContext(msg, "Ana")
	if mc.ReplyToID != "ABC" || mc.PushName != "Ana" || mc.Mentions != `["1@s.whatsapp.net","2@lid"]` {
		t.Fatalf("unexpected context: %+v", mc)
	}
	if empty := buildMsgContext(&waProto.Message{Conversation: proto.String("x")}, ""); empty.Mentions != "" || empty.ReplyToID != "" {
		t.Fatalf("expected empty context: %+v", empty)
	}
}

func TestNextBackoff(t *testing.T) {
	d := time.Second
	for i := 0; i < 10; i++ {
		d = nextBackoff(d)
	}
	if d != 60*time.Second {
		t.Fatalf("backoff should cap at 60s, got %s", d)
	}
}

func TestMigrateIdempotent(t *testing.T) {
	t.Chdir(t.TempDir())
	for i := 0; i < 2; i++ {
		s, err := NewMessageStore()
		if err != nil {
			t.Fatal(err)
		}
		if err := s.StoreChat("c@g.us", "c", time.Now()); err != nil {
			t.Fatal(err)
		}
		err = s.StoreMessage("1", "c@g.us", "s", "hola", time.Now(), false, "", "", "", nil, nil, nil, 0, MsgContext{ReplyToID: "r", Mentions: "[]", PushName: "p"})
		if err != nil {
			t.Fatal(err)
		}
		var pn string
		if err := s.db.QueryRow("SELECT push_name FROM messages WHERE id='1'").Scan(&pn); err != nil || pn != "p" {
			t.Fatalf("push_name %q %v", pn, err)
		}
		s.Close()
	}
}
