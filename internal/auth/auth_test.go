package auth

import "testing"

func TestHasRoleCaseInsensitive(t *testing.T) {
	c := Claims{Roles: []string{"User", "Admin"}}
	if !HasRole(c, "admin") {
		t.Fatalf("expected role to match")
	}
	if HasRole(c, "moderator") {
		t.Fatalf("did not expect moderator role")
	}
}
