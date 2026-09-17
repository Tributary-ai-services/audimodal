package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const (
	testSecret = "test-jwt-secret-at-least-32-chars-long-xxxx"
	testAPIKey = "service-account-key-at-least-32-chars"
)

func authTestConfig() *Config {
	c := GetDefaultConfig()
	c.AuthEnabled = true
	c.JWTSecret = testSecret
	c.APIKeys = []string{testAPIKey}
	return c
}

// signed returns a token signed with secret, using the given method and claims.
func signed(t *testing.T, method jwt.SigningMethod, secret string, claims jwt.MapClaims) string {
	t.Helper()
	tok := jwt.NewWithClaims(method, claims)
	s, err := tok.SignedString([]byte(secret))
	if err != nil {
		t.Fatalf("signing token: %v", err)
	}
	return s
}

func validClaims() jwt.MapClaims {
	return jwt.MapClaims{"sub": "svc", "exp": time.Now().Add(time.Hour).Unix()}
}

// serve runs one request through the middleware and reports the status code.
// A 200 means the request reached the handler, i.e. it authenticated.
func serve(t *testing.T, c *Config, mutate func(*http.Request)) int {
	t.Helper()
	handler := AuthenticationMiddleware(c, nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/tenants/x/files", nil)
	if mutate != nil {
		mutate(req)
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec.Code
}

func TestAuthenticationMiddleware(t *testing.T) {
	c := authTestConfig()

	cases := []struct {
		name   string
		mutate func(*http.Request)
		want   int
	}{
		// The two live bypasses this fix closes (SEC-3). Both returned 200
		// against production on 2026-09-17.
		{"any 32-char api key is rejected", func(r *http.Request) {
			r.Header.Set("X-API-Key", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
		}, http.StatusUnauthorized},
		{"unsigned garbage bearer token is rejected", func(r *http.Request) {
			r.Header.Set("Authorization", "Bearer not.a.jwt")
		}, http.StatusUnauthorized},

		{"no credentials", nil, http.StatusUnauthorized},
		{"empty api key header falls through to 401", func(r *http.Request) {
			r.Header.Set("X-API-Key", "")
		}, http.StatusUnauthorized},
		{"configured api key is accepted", func(r *http.Request) {
			r.Header.Set("X-API-Key", testAPIKey)
		}, http.StatusOK},
		{"api key differing in one byte is rejected", func(r *http.Request) {
			r.Header.Set("X-API-Key", testAPIKey[:len(testAPIKey)-1]+"X")
		}, http.StatusUnauthorized},
		{"valid HS256 token is accepted", func(r *http.Request) {
			r.Header.Set("Authorization", "Bearer "+signed(t, jwt.SigningMethodHS256, testSecret, validClaims()))
		}, http.StatusOK},
		{"token signed with another secret is rejected", func(r *http.Request) {
			r.Header.Set("Authorization", "Bearer "+signed(t, jwt.SigningMethodHS256, "a-different-secret-of-sufficient-length", validClaims()))
		}, http.StatusUnauthorized},
		{"expired token is rejected", func(r *http.Request) {
			r.Header.Set("Authorization", "Bearer "+signed(t, jwt.SigningMethodHS256, testSecret,
				jwt.MapClaims{"sub": "svc", "exp": time.Now().Add(-time.Minute).Unix()}))
		}, http.StatusUnauthorized},
		{"token without exp is rejected", func(r *http.Request) {
			r.Header.Set("Authorization", "Bearer "+signed(t, jwt.SigningMethodHS256, testSecret, jwt.MapClaims{"sub": "svc"}))
		}, http.StatusUnauthorized},
		{"bearer prefix with empty token", func(r *http.Request) {
			r.Header.Set("Authorization", "Bearer ")
		}, http.StatusUnauthorized},
		{"non-bearer authorization scheme", func(r *http.Request) {
			r.Header.Set("Authorization", "Basic YWRtaW46YWRtaW4=")
		}, http.StatusUnauthorized},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := serve(t, c, tc.mutate); got != tc.want {
				t.Errorf("status = %d, want %d", got, tc.want)
			}
		})
	}
}

// alg:none is the classic way to forge a token when the server trusts the
// token's own header. jwt/v5 refuses to sign one, so the token is assembled by
// hand exactly as an attacker would send it.
func TestAuthenticationMiddlewareRejectsAlgNone(t *testing.T) {
	// {"alg":"none","typ":"JWT"} / {"sub":"svc","exp":<future>}
	header := "eyJhbGciOiJub25lIiwidHlwIjoiSldUIn0"
	claims := "eyJzdWIiOiJzdmMiLCJleHAiOjQxMDI0NDQ4MDB9"
	if got := serve(t, authTestConfig(), func(r *http.Request) {
		r.Header.Set("Authorization", "Bearer "+header+"."+claims+".")
	}); got != http.StatusUnauthorized {
		t.Errorf("alg:none token accepted (status %d) -- signature checking is bypassable", got)
	}
}

// With no API keys configured, API-key auth must be off rather than open.
func TestAuthenticationMiddlewareNoKeysConfigured(t *testing.T) {
	c := authTestConfig()
	c.APIKeys = nil
	if got := serve(t, c, func(r *http.Request) {
		r.Header.Set("X-API-Key", testAPIKey)
	}); got != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401 when no API keys are configured", got)
	}
}

// Without a JWT secret the bearer path cannot verify anything, so it must reject.
func TestAuthenticationMiddlewareNoJWTSecret(t *testing.T) {
	c := authTestConfig()
	tok := signed(t, jwt.SigningMethodHS256, testSecret, validClaims())
	c.JWTSecret = ""
	if got := serve(t, c, func(r *http.Request) {
		r.Header.Set("Authorization", "Bearer "+tok)
	}); got != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401 when no JWT secret is configured", got)
	}
}

// Health and metrics must stay reachable: they report that the service is up,
// including when auth is misconfigured.
func TestAuthenticationMiddlewareExemptPaths(t *testing.T) {
	c := authTestConfig()
	for _, path := range []string{c.HealthCheckPath, c.MetricsPath} {
		handler := AuthenticationMiddleware(c, nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusOK {
			t.Errorf("%s = %d, want 200 without credentials", path, rec.Code)
		}
	}
}

// AuthEnabled=false is a deliberate local-development setting and must stay
// permissive, so that turning auth off is a decision rather than a surprise.
func TestAuthenticationMiddlewareDisabled(t *testing.T) {
	c := authTestConfig()
	c.AuthEnabled = false
	if got := serve(t, c, nil); got != http.StatusOK {
		t.Errorf("status = %d, want 200 when auth is disabled", got)
	}
}
