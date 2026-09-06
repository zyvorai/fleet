package auth

import (
	"strings"
	"testing"
	"time"

	"github.com/zyvorai/zyvor-fleet/internal/model"
)

func TestSessionRoundTripAndTamper(t *testing.T) {
	m, err := NewSessionManager([]byte(strings.Repeat("k", 32)), time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	tok, err := m.Sign(model.User{ID: "u1", Email: "admin@example.test", Name: "Admin", Role: model.RoleAdmin})
	if err != nil {
		t.Fatal(err)
	}
	c, err := m.Verify(tok)
	if err != nil {
		t.Fatal(err)
	}
	if c.Subject != "u1" || c.Role != model.RoleAdmin {
		t.Fatalf("unexpected claims: %+v", c)
	}
	last := "A"
	if strings.HasSuffix(tok, "A") {
		last = "B"
	}
	if _, err := m.Verify(tok[:len(tok)-1] + last); err == nil {
		t.Fatal("tampered token verified")
	}
}
