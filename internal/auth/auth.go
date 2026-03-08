package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/coreos/go-oidc/v3/oidc"
)

type Claims struct {
	Subject  string
	Email    string
	Username string
	Roles    []string
}

type ctxKey int

const claimsKey ctxKey = 1

type Verifier struct {
	verifier *oidc.IDTokenVerifier
}

func NewVerifier(ctx context.Context, issuer, audience string) (*Verifier, error) {
	provider, err := oidc.NewProvider(ctx, issuer)
	if err != nil {
		return nil, fmt.Errorf("oidc provider: %w", err)
	}
	cfg := &oidc.Config{ClientID: strings.TrimSpace(audience)}
	if cfg.ClientID == "" {
		cfg.SkipClientIDCheck = true
	}
	v := provider.Verifier(cfg)
	return &Verifier{verifier: v}, nil
}

func (v *Verifier) Verify(ctx context.Context, token string) (Claims, error) {
	idToken, err := v.verifier.Verify(ctx, token)
	if err != nil {
		return Claims{}, err
	}
	var raw struct {
		Sub           string `json:"sub"`
		Email         string `json:"email"`
		PreferredName string `json:"preferred_username"`
		RealmAccess   struct {
			Roles []string `json:"roles"`
		} `json:"realm_access"`
	}
	if err := idToken.Claims(&raw); err != nil {
		return Claims{}, err
	}
	return Claims{
		Subject:  raw.Sub,
		Email:    raw.Email,
		Username: raw.PreferredName,
		Roles:    raw.RealmAccess.Roles,
	}, nil
}

func WithClaims(ctx context.Context, c Claims) context.Context {
	return context.WithValue(ctx, claimsKey, c)
}

func FromContext(ctx context.Context) (Claims, bool) {
	v := ctx.Value(claimsKey)
	if v == nil {
		return Claims{}, false
	}
	c, ok := v.(Claims)
	return c, ok
}

func HasRole(c Claims, role string) bool {
	for _, r := range c.Roles {
		if strings.EqualFold(r, role) {
			return true
		}
	}
	return false
}

func WriteJSON(w http.ResponseWriter, code int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(payload)
}
