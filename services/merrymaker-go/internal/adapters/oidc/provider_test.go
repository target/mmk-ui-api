package oidc

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/target/mmk-ui-api/internal/ports"
	"golang.org/x/oauth2"
)

func TestNewProvider_Success(t *testing.T) {
	// Create a mock OIDC discovery server
	issuer := ""
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		doc := DiscoveryDocument{
			Issuer:                issuer,
			AuthorizationEndpoint: "https://example.com/auth",
			TokenEndpoint:         "https://example.com/token",
			UserinfoEndpoint:      "https://example.com/userinfo",
			JwksURI:               "https://example.com/jwks",
		}
		_ = json.NewEncoder(w).Encode(doc)
	})
	discoveryServer := httptest.NewServer(handler)
	defer discoveryServer.Close()
	issuer = discoveryServer.URL

	config := ProviderConfig{
		ClientID:     "test-client",
		ClientSecret: "test-secret",
		RedirectURL:  "http://localhost:8080/callback",
		Scope:        "openid profile email groups",
		DiscoveryURL: discoveryServer.URL,
		LogoutURL:    "https://example.com/logout",
	}

	provider, err := NewProvider(config)
	require.NoError(t, err)
	assert.NotNil(t, provider)
	assert.Equal(t, "https://example.com/auth", provider.config.Endpoint.AuthURL)
	assert.Equal(t, "https://example.com/token", provider.config.Endpoint.TokenURL)
}

func TestNewProvider_ValidationErrors(t *testing.T) {
	tests := []struct {
		name   string
		config ProviderConfig
		errMsg string
	}{
		{
			name: "missing client ID",
			config: ProviderConfig{
				ClientSecret: "secret",
				RedirectURL:  "http://localhost/callback",
				DiscoveryURL: "http://example.com",
			},
			errMsg: "client ID is required",
		},
		{
			name: "missing client secret",
			config: ProviderConfig{
				ClientID:     "client",
				RedirectURL:  "http://localhost/callback",
				DiscoveryURL: "http://example.com",
			},
			errMsg: "client secret is required",
		},
		{
			name:   "missing redirect URL",
			config: ProviderConfig{ClientID: "client", ClientSecret: "secret", DiscoveryURL: "http://example.com"},
			errMsg: "redirect URL is required",
		},
		{
			name: "missing discovery URL",
			config: ProviderConfig{
				ClientID:     "client",
				ClientSecret: "secret",
				RedirectURL:  "http://localhost/callback",
			},
			errMsg: "discovery URL is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewProvider(tt.config)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.errMsg)
		})
	}
}

func TestProvider_Begin(t *testing.T) {
	provider := createTestProvider(t)
	ctx := context.Background()

	input := ports.BeginInput{RedirectURL: "http://localhost:8080/callback"}
	authURL, state, nonce, err := provider.Begin(ctx, input)

	require.NoError(t, err)
	assert.NotEmpty(t, authURL)
	assert.NotEmpty(t, state)
	assert.NotEmpty(t, nonce)
	assert.Contains(t, authURL, "https://example.com/auth")
	assert.Contains(t, authURL, "client_id=test-client")
	assert.Contains(t, authURL, "state="+state)
	assert.Contains(t, authURL, "nonce="+nonce)
}

func TestProvider_Begin_EmptyRedirectURL(t *testing.T) {
	provider := createTestProvider(t)
	ctx := context.Background()

	input := ports.BeginInput{RedirectURL: ""}
	_, _, _, err := provider.Begin(ctx, input)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "redirect URL is required")
}

func TestProvider_Exchange_ValidationErrors(t *testing.T) {
	provider := createTestProvider(t)
	ctx := context.Background()

	tests := []struct {
		name   string
		input  ports.ExchangeInput
		errMsg string
	}{
		{
			name:   "missing code",
			input:  ports.ExchangeInput{State: "state", Nonce: "nonce"},
			errMsg: "authorization code is required",
		},
		{
			name:   "missing state",
			input:  ports.ExchangeInput{Code: "code", Nonce: "nonce"},
			errMsg: "state is required",
		},
		{
			name:   "missing nonce",
			input:  ports.ExchangeInput{Code: "code", State: "state"},
			errMsg: "nonce is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := provider.Exchange(ctx, tt.input)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.errMsg)
		})
	}
}

func TestGenerateRandomString(t *testing.T) {
	// Test that it generates strings of the correct length
	str1, err := generateRandomString(16)
	require.NoError(t, err)
	assert.Len(t, str1, 16)

	str2, err := generateRandomString(32)
	require.NoError(t, err)
	assert.Len(t, str2, 32)

	// Test that it generates different strings
	assert.NotEqual(t, str1, str2)

	// Test multiple calls produce different results
	str3, err := generateRandomString(16)
	require.NoError(t, err)
	assert.NotEqual(t, str1, str3)
}

// createTestProvider creates a test provider with mocked discovery endpoint.
func createTestProvider(t *testing.T) *Provider {
	t.Helper()

	// Create a mock OIDC discovery server
	issuer := ""
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		doc := DiscoveryDocument{
			Issuer:                issuer,
			AuthorizationEndpoint: "https://example.com/auth",
			TokenEndpoint:         "https://example.com/token",
			UserinfoEndpoint:      "https://example.com/userinfo",
			JwksURI:               "https://example.com/jwks",
		}
		_ = json.NewEncoder(w).Encode(doc)
	})
	discoveryServer := httptest.NewServer(handler)
	t.Cleanup(discoveryServer.Close)
	issuer = discoveryServer.URL

	config := ProviderConfig{
		ClientID:     "test-client",
		ClientSecret: "test-secret",
		RedirectURL:  "http://localhost:8080/callback",
		Scope:        "openid profile email groups",
		DiscoveryURL: discoveryServer.URL,
		LogoutURL:    "https://example.com/logout",
	}

	provider, err := NewProvider(config)
	require.NoError(t, err)
	return provider
}

// Test that the provider implements the AuthProvider interface.
func TestProvider_ImplementsInterface(t *testing.T) {
	provider := createTestProvider(t)
	var _ ports.AuthProvider = provider
}

func TestProvider_Exchange_MockSuccess(t *testing.T) {
	// This test would require more complex mocking of the OAuth2 flow
	// For now, we'll just test the validation logic
	provider := createTestProvider(t)
	ctx := context.Background()

	input := ports.ExchangeInput{
		Code:  "test-code",
		State: "test-state",
		Nonce: "test-nonce",
	}

	// This will fail because we don't have a real token endpoint,
	// but it should pass validation and attempt the exchange
	_, err := provider.Exchange(ctx, input)
	require.Error(t, err) // Expected to fail due to mock setup
	assert.Contains(t, err.Error(), "exchange code for token")
}

func TestGetIDTokenFromToken_Success(t *testing.T) {
	tok := (&oauth2.Token{}).WithExtra(map[string]any{"id_token": "abc.def.ghi"})
	idTok, err := getIDTokenFromToken(tok)
	require.NoError(t, err)
	assert.Equal(t, "abc.def.ghi", idTok)
}

func TestGetIDTokenFromToken_Missing(t *testing.T) {
	tok := (&oauth2.Token{}).WithExtra(map[string]any{"not_id": "x"})
	_, err := getIDTokenFromToken(tok)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing id_token")
}

func TestGetIDTokenFromToken_Nil(t *testing.T) {
	_, err := getIDTokenFromToken(nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "nil token")
}

func Test_mapIDTokenClaims_ADShape(t *testing.T) {
	claims := idTokenADClaims{
		Sub:            "sub-123",
		SamAccountName: "sammy",
		FirstName:      "First",
		LastName:       "Last",
		Mail:           "mail@example.com",
		MemberOf:       []string{"CN=APP-Oauth2-MerryMaker-User,OU=Application,OU=Groupings,DC=corp,DC=target,DC=com"},
	}
	f := mapIDTokenClaims(claims)
	assert.Equal(t, "sammy", f.userID)
	assert.Equal(t, "mail@example.com", f.email)
	assert.Equal(t, "First", f.givenName)
	assert.Equal(t, "Last", f.familyName)
	assert.Equal(t, claims.MemberOf, f.groups)
}

func Test_fillFromUserInfoClaims_ADShape(t *testing.T) {
	ui := UserInfo{
		Subject:        "sub-abc",
		SamAccountName: "sammy",
		FirstName:      "First",
		LastName:       "Last",
		Mail:           "mail@example.com",
		MemberOf:       []string{"CN=APP-Oauth2-MerryMaker-Admin,OU=Application,OU=Groupings,DC=corp,DC=target,DC=com"},
	}
	var f idFields
	fillFromUserInfoClaims(&f, ui)
	assert.Equal(t, "sammy", f.userID)
	assert.Equal(t, "mail@example.com", f.email)
	assert.Equal(t, "First", f.givenName)
	assert.Equal(t, "Last", f.familyName)
	assert.Equal(t, ui.MemberOf, f.groups)

	// Verify that existing fields are not overwritten
	f2 := idFields{
		userID:     "keep",
		email:      "keep@example.com",
		givenName:  "Keep",
		familyName: "Keep",
		groups:     []string{"x"},
	}
	fillFromUserInfoClaims(&f2, ui)
	assert.Equal(t, "keep", f2.userID)
	assert.Equal(t, "keep@example.com", f2.email)
	assert.Equal(t, "Keep", f2.givenName)
	assert.Equal(t, "Keep", f2.familyName)
	assert.Equal(t, []string{"x"}, f2.groups)
}
