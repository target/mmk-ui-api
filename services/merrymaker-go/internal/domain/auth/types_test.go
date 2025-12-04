package auth

import (
	"testing"
	"time"
)

func TestSession_IsGuest(t *testing.T) {
	s := Session{Role: RoleGuest}
	if !s.IsGuest() {
		t.Fatalf("expected guest")
	}
	if (Session{Role: RoleUser}).IsGuest() {
		t.Fatalf("did not expect guest")
	}
}

func TestIdentity_SimpleFields(t *testing.T) {
	id := Identity{UserID: "u", Email: "e", ExpiresAt: time.Now().Add(time.Hour)}
	if id.UserID != "u" || id.Email != "e" {
		t.Fatalf("unexpected identity: %+v", id)
	}
}
