package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMiddlewareCreatesCookie(t *testing.T) {
	var gotUserID string
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var ok bool
		gotUserID, ok = UserIDFromContext(r.Context())
		if !ok {
			t.Fatal("user id was not stored in context")
		}
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	Middleware(next).ServeHTTP(rec, req)

	cookie := findCookie(rec.Result().Cookies())
	if cookie == nil {
		t.Fatal("cookie was not set")
	}

	userID, ok := Parse(cookie.Value)
	if !ok {
		t.Fatal("cookie signature is invalid")
	}
	if userID == "" {
		t.Fatal("cookie user id is empty")
	}
	if gotUserID != userID {
		t.Fatalf("context user id = %q, want %q", gotUserID, userID)
	}
}

func TestMiddlewareUsesValidCookie(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userID, ok := UserIDFromContext(r.Context())
		if !ok {
			t.Fatal("user id was not stored in context")
		}
		if userID != "user-1" {
			t.Fatalf("user id = %q, want %q", userID, "user-1")
		}
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(NewCookie("user-1"))
	rec := httptest.NewRecorder()

	Middleware(next).ServeHTTP(rec, req)

	if cookie := findCookie(rec.Result().Cookies()); cookie != nil {
		t.Fatalf("new cookie was set: %+v", cookie)
	}
}

func TestMiddlewareReplacesInvalidCookie(t *testing.T) {
	var gotUserID string
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var ok bool
		gotUserID, ok = UserIDFromContext(r.Context())
		if !ok {
			t.Fatal("user id was not stored in context")
		}
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: CookieName, Value: "bad-cookie"})
	rec := httptest.NewRecorder()

	Middleware(next).ServeHTTP(rec, req)

	cookie := findCookie(rec.Result().Cookies())
	if cookie == nil {
		t.Fatal("new cookie was not set")
	}

	userID, ok := Parse(cookie.Value)
	if !ok {
		t.Fatal("new cookie signature is invalid")
	}
	if userID == "" || gotUserID == "" {
		t.Fatalf("user ids are empty: cookie=%q context=%q", userID, gotUserID)
	}
	if userID != gotUserID {
		t.Fatalf("context user id = %q, want %q", gotUserID, userID)
	}
}

func TestMiddlewareKeepsSignedEmptyUserID(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if userID, ok := UserIDFromContext(r.Context()); ok {
			t.Fatalf("user id = %q, want no user id", userID)
		}
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(NewCookie(""))
	rec := httptest.NewRecorder()

	Middleware(next).ServeHTTP(rec, req)

	if cookie := findCookie(rec.Result().Cookies()); cookie != nil {
		t.Fatalf("new cookie was set: %+v", cookie)
	}
}

func findCookie(cookies []*http.Cookie) *http.Cookie {
	for _, cookie := range cookies {
		if cookie.Name == CookieName {
			return cookie
		}
	}

	return nil
}
