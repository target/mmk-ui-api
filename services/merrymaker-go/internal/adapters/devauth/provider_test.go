package devauth

import (
	"context"
	"strings"
	"testing"

	"github.com/target/mmk-ui-api/internal/ports"
)

func TestProvider_BeginAndExchange(t *testing.T) {
	prov, err := NewProvider(Config{UserID: "dev-user", Email: "dev@example.com", Groups: []string{"users"}})
	if err != nil {
		t.Fatalf("NewProvider error: %v", err)
	}
	url, state, nonce, err := prov.Begin(context.Background(), ports.BeginInput{RedirectURL: "/"})
	if err != nil {
		t.Fatalf("Begin error: %v", err)
	}
	if !strings.HasPrefix(url, "/auth/callback?") {
		t.Fatalf("unexpected authURL: %s", url)
	}
	if state == "" || nonce == "" {
		t.Fatal("state and nonce should be generated")
	}
	id, err := prov.Exchange(context.Background(), ports.ExchangeInput{Code: "dev", State: state, Nonce: nonce})
	if err != nil {
		t.Fatalf("Exchange error: %v", err)
	}
	if id.UserID != "dev-user" || id.Email != "dev@example.com" {
		t.Fatalf("unexpected identity: %+v", id)
	}
}
