package auth

import (
	"net/http"
	"strings"
)

func Optional(verifier *Verifier) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h := r.Header.Get("Authorization")
			if h == "" || !strings.HasPrefix(strings.ToLower(h), "bearer ") {
				next.ServeHTTP(w, r)
				return
			}
			tok := strings.TrimSpace(h[len("Bearer "):])
			claims, err := verifier.Verify(r.Context(), tok)
			if err != nil {
				WriteJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid token"})
				return
			}
			next.ServeHTTP(w, r.WithContext(WithClaims(r.Context(), claims)))
		})
	}
}

func Required(verifier *Verifier) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if verifier == nil {
				WriteJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "auth verifier not configured"})
				return
			}
			h := r.Header.Get("Authorization")
			if h == "" || !strings.HasPrefix(strings.ToLower(h), "bearer ") {
				WriteJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing bearer token"})
				return
			}
			tok := strings.TrimSpace(h[len("Bearer "):])
			claims, err := verifier.Verify(r.Context(), tok)
			if err != nil {
				WriteJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid token"})
				return
			}
			next.ServeHTTP(w, r.WithContext(WithClaims(r.Context(), claims)))
		})
	}
}

func RequireRole(role string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims, ok := FromContext(r.Context())
			if !ok {
				WriteJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthenticated"})
				return
			}
			if !HasRole(claims, role) {
				WriteJSON(w, http.StatusForbidden, map[string]string{"error": "insufficient role"})
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func RequireAnyRole(roles ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims, ok := FromContext(r.Context())
			if !ok {
				WriteJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthenticated"})
				return
			}
			for _, role := range roles {
				if HasRole(claims, role) {
					next.ServeHTTP(w, r)
					return
				}
			}
			WriteJSON(w, http.StatusForbidden, map[string]string{"error": "insufficient role"})
		})
	}
}

func RequireAuthWhen(enabled bool, verifier *Verifier) func(http.Handler) http.Handler {
	if !enabled || verifier == nil {
		return func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				next.ServeHTTP(w, r)
			})
		}
	}
	return Required(verifier)
}
