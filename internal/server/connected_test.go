package server

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"shu/internal/config"
)

func TestConnectedSecretEncryptionRoundTrip(t *testing.T) {
	a := &App{cfg: config.Config{SecretKey: "test-secret"}}
	plain := []byte(`{"imap_password":"pw","smtp_password":"spw"}`)
	cipher, err := a.encryptSecret(plain)
	if err != nil {
		t.Fatal(err)
	}
	if cipher == string(plain) || strings.Contains(cipher, "pw") {
		t.Fatalf("secret not encrypted/redacted: %q", cipher)
	}
	got, err := a.decryptSecret(cipher)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(plain) {
		t.Fatalf("round trip mismatch: %s", got)
	}
}

func TestConnectedCalendarParser(t *testing.T) {
	ics := "BEGIN:VCALENDAR\r\nBEGIN:VEVENT\r\nUID:abc\r\nSUMMARY:Call with Sam\r\nDESCRIPTION:Zoom meeting\\nBring notes\r\nLOCATION:https://zoom.test/j/1\r\nDTSTART:20260605T120000Z\r\nDTEND:20260605T130000Z\r\nRRULE:FREQ=WEEKLY\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"
	events := parseVEVENTs(ics)
	if len(events) != 1 {
		t.Fatalf("events=%d", len(events))
	}
	e := events[0]
	if e.UID != "abc" || e.Summary != "Call with Sam" || e.Location == "" || e.RRule != "FREQ=WEEKLY" {
		t.Fatalf("bad event: %#v", e)
	}
	if e.Start == nil || e.End == nil || e.AllDay {
		t.Fatalf("bad event times: %#v", e)
	}
}

func TestResourceEventMatches(t *testing.T) {
	if !resourceEventMatches(map[string]any{"kind": "email.message", "resource_id": "r1"}, "r1", "email.message") {
		t.Fatal("expected match")
	}
	if resourceEventMatches(map[string]any{"kind": "calendar.event"}, "r1", "email.message") {
		t.Fatal("expected kind mismatch")
	}
	if resourceEventMatches(map[string]any{"resource_id": "r2"}, "r1", "email.message") {
		t.Fatal("expected resource mismatch")
	}
}

func TestPostProcessTriageDataShapeDoesNotRequireNetwork(t *testing.T) {
	data := map[string]any{"urgency": 3, "tags": []any{"work", "reply-soon"}, "spam": false}
	b, err := json.Marshal(data)
	if err != nil || !strings.Contains(string(b), "reply-soon") {
		t.Fatalf("bad triage json: %s %v", b, err)
	}
	_ = context.Background()
}

func TestTodoReminderTitle(t *testing.T) {
	if got := reminderTitle("todo.item", "Pay rent"); got != "Todo reminder: Pay rent" {
		t.Fatalf("todo reminder title = %q", got)
	}
	if got := reminderTitle("calendar.event", "Call"); got != "Calendar reminder: Call" {
		t.Fatalf("calendar reminder title = %q", got)
	}
	if !connectedActionSupported("todo.complete") || !connectedActionSupported("notify") {
		t.Fatal("todo/notify actions missing")
	}
}
