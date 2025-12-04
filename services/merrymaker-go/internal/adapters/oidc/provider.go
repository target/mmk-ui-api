package oidc

// Package oidc provides OIDC/OAuth authentication adapters for the merrymaker system.

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	gooidc "github.com/coreos/go-oidc/v3/oidc"
	domainauth "github.com/target/mmk-ui-api/internal/domain/auth"
	"github.com/target/mmk-ui-api/internal/ports"
	"golang.org/x/oauth2"
)

// Provider implements the AuthProvider interface using OIDC/OAuth2.
type Provider struct {
	config     *oauth2.Config
	logoutURL  string
	httpClient *http.Client

	// go-oidc provider and verifier
	oidcProvider *gooidc.Provider
	verifier     *gooidc.IDTokenVerifier
}

// ProviderConfig holds configuration for the OIDC provider.
type ProviderConfig struct {
	ClientID     string
	ClientSecret string
	RedirectURL  string
	Scope        string
	DiscoveryURL string
	LogoutURL    string
	HTTPClient   *http.Client // Optional, defaults to http.DefaultClient
}

// DiscoveryDocument represents the OIDC discovery document.
type DiscoveryDocument struct {
	Issuer                string `json:"issuer"`
	AuthorizationEndpoint string `json:"authorization_endpoint"`
	TokenEndpoint         string `json:"token_endpoint"`
	UserinfoEndpoint      string `json:"userinfo_endpoint"`
	JwksURI               string `json:"jwks_uri"`
}

// NewProvider creates a new OIDC provider.
func NewProvider(config ProviderConfig) (*Provider, error) {
	if config.ClientID == "" {
		return nil, errors.New("client ID is required")
	}
	if config.ClientSecret == "" {
		return nil, errors.New("client secret is required")
	}
	if config.RedirectURL == "" {
		return nil, errors.New("redirect URL is required")
	}
	if config.DiscoveryURL == "" {
		return nil, errors.New("discovery URL is required")
	}

	httpClient := config.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}

	p := &Provider{
		logoutURL:  config.LogoutURL,
		httpClient: httpClient,
	}

	// Initialize go-oidc provider and verifier (single discovery fetch)
	ctx := context.WithValue(context.Background(), oauth2.HTTPClient, httpClient)
	issuer := strings.TrimSuffix(config.DiscoveryURL, "/")
	issuer = strings.TrimSuffix(issuer, "/.well-known/openid-configuration")
	issuer = strings.TrimSuffix(issuer, ".well-known/openid-configuration")
	op, err := gooidc.NewProvider(ctx, issuer)
	if err != nil {
		return nil, fmt.Errorf("oidc new provider: %w", err)
	}
	p.oidcProvider = op
	p.verifier = op.Verifier(&gooidc.Config{ClientID: config.ClientID})

	// Configure OAuth2 using discovered endpoints
	endpoint := op.Endpoint()
	p.config = &oauth2.Config{
		ClientID:     config.ClientID,
		ClientSecret: config.ClientSecret,
		RedirectURL:  config.RedirectURL,
		Scopes:       strings.Fields(config.Scope),
		Endpoint:     endpoint,
	}

	return p, nil
}

func (p *Provider) Begin(_ context.Context, in ports.BeginInput) (string, string, string, error) {
	if in.RedirectURL == "" {
		return "", "", "", errors.New("redirect URL is required")
	}

	// Generate cryptographically secure state and nonce
	state, err := generateRandomString(32)
	if err != nil {
		return "", "", "", fmt.Errorf("generate state: %w", err)
	}

	nonce, err := generateRandomString(32)
	if err != nil {
		return "", "", "", fmt.Errorf("generate nonce: %w", err)
	}

	// Build auth URL with OIDC parameters
	// Note: Don't override redirect_uri here as it should match the configured RedirectURL exactly
	authURL := p.config.AuthCodeURL(state,
		oauth2.SetAuthURLParam("nonce", nonce),
		oauth2.SetAuthURLParam("response_type", "code"),
		oauth2.SetAuthURLParam("prompt", "select_account"),
	)

	return authURL, state, nonce, nil
}

func (p *Provider) Exchange(ctx context.Context, in ports.ExchangeInput) (domainauth.Identity, error) {
	if in.Code == "" {
		return domainauth.Identity{}, errors.New("authorization code is required")
	}
	if in.State == "" {
		return domainauth.Identity{}, errors.New("state is required")
	}
	if in.Nonce == "" {
		return domainauth.Identity{}, errors.New("nonce is required")
	}

	// Exchange code for token
	token, err := p.config.Exchange(ctx, in.Code)
	if err != nil {
		return domainauth.Identity{}, fmt.Errorf("exchange code for token: %w", err)
	}

	// Extract from ID token when openid is present
	fields, err := p.extractFromIDToken(ctx, token, in.Nonce)
	if err != nil {
		return domainauth.Identity{}, fmt.Errorf("extract id_token: %w", err)
	}

	// Fill missing fields from UserInfo
	if fields.email == "" || fields.userID == "" {
		if fillErr := p.fillFromUserInfo(ctx, token.AccessToken, &fields); fillErr != nil {
			return domainauth.Identity{}, fmt.Errorf("get user info: %w", fillErr)
		}
	}

	expiresAt := time.Now().Add(time.Hour)
	if !token.Expiry.IsZero() {
		expiresAt = token.Expiry
	}

	return domainauth.Identity{
		UserID:    fields.userID,
		FirstName: fields.givenName,
		LastName:  fields.familyName,
		Email:     fields.email,
		Groups:    fields.groups,
		ExpiresAt: expiresAt,
	}, nil
}

// UserInfo represents the user information from the OIDC userinfo endpoint.
type UserInfo struct {
	Subject        string   `json:"sub"`
	SamAccountName string   `json:"samaccountname"`
	FirstName      string   `json:"firstname"`
	LastName       string   `json:"lastname"`
	Mail           string   `json:"mail"`
	MemberOf       []string `json:"memberof"`
}

func (p *Provider) getUserInfo(ctx context.Context, accessToken string) (*UserInfo, error) {
	ui, err := p.oidcProvider.UserInfo(ctx, oauth2.StaticTokenSource(&oauth2.Token{AccessToken: accessToken}))
	if err != nil {
		return nil, fmt.Errorf("fetch user info: %w", err)
	}
	var userInfo UserInfo
	if claimsErr := ui.Claims(&userInfo); claimsErr != nil {
		return nil, fmt.Errorf("decode user info: %w", claimsErr)
	}
	return &userInfo, nil
}

// internal helper types and functions to keep Exchange small

type idFields struct {
	userID     string
	email      string
	givenName  string
	familyName string
	groups     []string
}

func (p *Provider) extractFromIDToken(ctx context.Context, tok *oauth2.Token, expectedNonce string) (idFields, error) {
	var f idFields
	if !p.hasOpenIDScope() {
		return f, nil
	}
	rawID, err := getIDTokenFromToken(tok)
	if err != nil {
		return f, err
	}
	idTok, err := p.verifier.Verify(ctx, rawID)
	if err != nil {
		return f, fmt.Errorf("verify id_token: %w", err)
	}
	var claims idTokenADClaims
	if claimsErr := idTok.Claims(&claims); claimsErr != nil {
		return f, fmt.Errorf("parse id_token claims: %w", claimsErr)
	}
	if expectedNonce != "" && claims.Nonce != expectedNonce {
		return f, errors.New("invalid nonce")
	}
	// Map claims to fields using shared helper
	f = mapIDTokenClaims(claims)
	return f, nil
}

func (p *Provider) fillFromUserInfo(ctx context.Context, accessToken string, f *idFields) error {
	ui, err := p.getUserInfo(ctx, accessToken)
	if err != nil {
		return err
	}
	fillFromUserInfoClaims(f, *ui)
	return nil
}

// Internal helper types to make claim mapping testable and consistent.
// idTokenADClaims represents a superset of OIDC and AD/ADFS claim shapes.
type idTokenADClaims struct {
	Sub            string   `json:"sub"`
	SamAccountName string   `json:"samaccountname"`
	FirstName      string   `json:"firstname"`
	LastName       string   `json:"lastname"`
	Mail           string   `json:"mail"`
	MemberOf       []string `json:"memberof"`
	ExpiresAt      int64    `json:"exp"`
	Nonce          string   `json:"nonce"`
}

// mapIDTokenClaims maps raw id token claims into idFields using precedence rules.
func mapIDTokenClaims(c idTokenADClaims) idFields {
	var f idFields
	// Map claims using AD/ADFS shape only
	f.userID = firstNonEmpty(c.SamAccountName, c.Sub)
	f.email = c.Mail
	f.givenName = c.FirstName
	f.familyName = c.LastName
	f.groups = c.MemberOf
	return f
}

// fillFromUserInfoClaims fills missing fields from a UserInfo payload using precedence rules.
func fillFromUserInfoClaims(f *idFields, ui UserInfo) {
	if f.userID == "" {
		f.userID = firstNonEmpty(ui.SamAccountName, ui.Subject)
	}
	if f.email == "" {
		f.email = ui.Mail
	}
	if f.givenName == "" {
		f.givenName = ui.FirstName
	}
	if f.familyName == "" {
		f.familyName = ui.LastName
	}
	if len(f.groups) == 0 {
		f.groups = ui.MemberOf
	}
}

// firstNonEmpty returns the first non-empty string from vals, or empty string if none.
func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// Helpers retained after migration to go-oidc

// generateRandomString generates a cryptographically secure URL-safe random string of exact length.
func generateRandomString(length int) (string, error) {
	if length <= 0 {
		return "", nil
	}
	// Compute number of random bytes needed to produce at least 'length' base64 URL-safe chars
	nBytes := (length*3 + 3) / 4
	b := make([]byte, nBytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	s := base64.RawURLEncoding.EncodeToString(b)
	if len(s) < length {
		extra := make([]byte, 1)
		if _, err := rand.Read(extra); err != nil {
			return "", err
		}
		s += base64.RawURLEncoding.EncodeToString(extra)
	}
	return s[:length], nil
}

// hasOpenIDScope reports whether the configured scopes include "openid".
func (p *Provider) hasOpenIDScope() bool {
	for _, sc := range p.config.Scopes {
		if sc == "openid" {
			return true
		}
	}
	return false
}

// getIDTokenFromToken extracts the id_token from oauth2.Token.
func getIDTokenFromToken(tok *oauth2.Token) (string, error) {
	if tok == nil {
		return "", errors.New("nil token")
	}
	raw := tok.Extra("id_token")
	s, ok := raw.(string)
	if !ok || s == "" {
		return "", errors.New("missing id_token in token response")
	}
	return s, nil
}
