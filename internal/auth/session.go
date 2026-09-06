package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/zyvorai/fleet/internal/model"
)

type Claims struct {
	Subject string     `json:"sub"`
	Email   string     `json:"email"`
	Name    string     `json:"name"`
	Role    model.Role `json:"role"`
	Version int        `json:"ver,omitempty"`
	Issued  int64      `json:"iat"`
	Expires int64      `json:"exp"`
}

type SessionManager struct {
	key []byte
	ttl time.Duration
}

func NewSessionManager(secret []byte, ttl time.Duration) (*SessionManager, error) {
	if len(secret) < 32 {
		return nil, errors.New("session secret must be at least 32 bytes")
	}
	return &SessionManager{key: append([]byte(nil), secret...), ttl: ttl}, nil
}

func RandomSecret(n int) ([]byte, error) {
	b := make([]byte, n)
	_, err := rand.Read(b)
	return b, err
}

func RandomToken(prefix string) (string, error) {
	b, err := RandomSecret(32)
	if err != nil {
		return "", err
	}
	return prefix + base64.RawURLEncoding.EncodeToString(b), nil
}

func SHA256Token(token string) string {
	h := sha256.Sum256([]byte(token))
	return base64.RawURLEncoding.EncodeToString(h[:])
}

func (m *SessionManager) Sign(user model.User) (string, error) {
	now := time.Now().UTC()
	c := Claims{Subject: user.ID, Email: user.Email, Name: user.Name, Role: user.Role, Version: user.AuthVersion, Issued: now.Unix(), Expires: now.Add(m.ttl).Unix()}
	b, err := json.Marshal(c)
	if err != nil {
		return "", err
	}
	payload := base64.RawURLEncoding.EncodeToString(b)
	mac := hmac.New(sha256.New, m.key)
	_, _ = mac.Write([]byte(payload))
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return payload + "." + sig, nil
}

func (m *SessionManager) Verify(token string) (Claims, error) {
	var c Claims
	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		return c, errors.New("invalid session")
	}
	mac := hmac.New(sha256.New, m.key)
	_, _ = mac.Write([]byte(parts[0]))
	expected := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	if len(parts[1]) != len(expected) || subtle.ConstantTimeCompare([]byte(expected), []byte(parts[1])) != 1 {
		return c, errors.New("invalid session signature")
	}
	b, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil || json.Unmarshal(b, &c) != nil {
		return c, errors.New("invalid session payload")
	}
	if c.Expires <= time.Now().Unix() {
		return c, errors.New("session expired")
	}
	return c, nil
}
