package auth

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"net/http"
	"strings"
)

const CookieName = "user_id"

const secret = "shortener-auth-cookie-secret"

type userIDKey struct{}

func Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if cookie, err := r.Cookie(CookieName); err == nil {
			userID, ok := Parse(cookie.Value)
			if ok {
				if userID != "" {
					r = r.WithContext(WithUserID(r.Context(), userID))
				}
				next.ServeHTTP(w, r)
				return
			}
		}

		userID, err := newUserID()
		if err != nil {
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			return
		}

		http.SetCookie(w, NewCookie(userID))
		r = r.WithContext(WithUserID(r.Context(), userID))
		next.ServeHTTP(w, r)
	})
}

func WithUserID(ctx context.Context, userID string) context.Context {
	return context.WithValue(ctx, userIDKey{}, userID)
}

func UserIDFromContext(ctx context.Context) (string, bool) {
	userID, ok := ctx.Value(userIDKey{}).(string)
	if !ok || userID == "" {
		return "", false
	}

	return userID, true
}

func NewCookie(userID string) *http.Cookie {
	return &http.Cookie{
		Name:     CookieName,
		Value:    signedValue(userID),
		Path:     "/",
		HttpOnly: true,
	}
}

func Parse(value string) (string, bool) {
	encodedUserID, signature, ok := strings.Cut(value, ".")
	if !ok {
		return "", false
	}

	want := sign(encodedUserID)
	got, err := hex.DecodeString(signature)
	if err != nil || !hmac.Equal(got, want) {
		return "", false
	}

	data, err := base64.RawURLEncoding.DecodeString(encodedUserID)
	if err != nil {
		return "", false
	}

	return string(data), true
}

func signedValue(userID string) string {
	encodedUserID := base64.RawURLEncoding.EncodeToString([]byte(userID))
	signature := hex.EncodeToString(sign(encodedUserID))

	return encodedUserID + "." + signature
}

func sign(value string) []byte {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(value))

	return mac.Sum(nil)
}

func newUserID() (string, error) {
	data := make([]byte, 16)
	if _, err := rand.Read(data); err != nil {
		return "", err
	}

	return base64.RawURLEncoding.EncodeToString(data), nil
}
