package security

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
	"time"
)

type Signer struct {
	key []byte
}

func NewSigner(key string) *Signer {
	return &Signer{key: []byte(key)}
}

func (s *Signer) Sign(trackID, format string, expiresAt time.Time) string {
	payload := fmt.Sprintf("%s|%s|%d", trackID, format, expiresAt.Unix())
	mac := hmac.New(sha256.New, s.key)
	_, _ = mac.Write([]byte(payload))
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return sig
}

func (s *Signer) Verify(trackID, format, signature string, expiresUnix int64, now time.Time) bool {
	if now.Unix() > expiresUnix {
		return false
	}
	expected := s.Sign(trackID, format, time.Unix(expiresUnix, 0))
	return hmac.Equal([]byte(expected), []byte(signature))
}

func ParseExpires(v string) (int64, error) {
	if strings.TrimSpace(v) == "" {
		return 0, fmt.Errorf("missing expires")
	}
	e, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid expires: %w", err)
	}
	return e, nil
}
