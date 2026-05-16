package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// SessionCookie is the name of the platform session cookie.
const SessionCookie = "nv_session"

type ctxKey int

const userCtxKey ctxKey = iota

// RequireUser returns middleware that resolves the session cookie to a user
// and stores it in the request context. Requests with no valid session are
// rejected with 401 before the handler runs.
func RequireUser(pool *pgxpool.Pool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user, ok := resolveUser(r, pool)
			if !ok {
				writeUnauthorized(w)
				return
			}
			ctx := context.WithValue(r.Context(), userCtxKey, user)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func resolveUser(r *http.Request, pool *pgxpool.Pool) (User, bool) {
	c, err := r.Cookie(SessionCookie)
	if err != nil || c.Value == "" {
		return User{}, false
	}
	session, err := ResolveSession(r.Context(), pool, c.Value)
	if err != nil {
		return User{}, false
	}
	user, err := GetUserByID(r.Context(), pool, session.UserID)
	if err != nil {
		return User{}, false
	}
	return user, true
}

// UserFromContext returns the authenticated user stored by RequireUser.
func UserFromContext(ctx context.Context) (User, bool) {
	u, ok := ctx.Value(userCtxKey).(User)
	return u, ok
}

// CurrentUser resolves the session cookie to a user without rejecting the
// request. It is for routes that accept both authenticated and anonymous
// callers, such as accepting an invitation.
func CurrentUser(r *http.Request, pool *pgxpool.Pool) (User, bool) {
	return resolveUser(r, pool)
}

// SetSessionCookie writes the session cookie to the response.
func SetSessionCookie(w http.ResponseWriter, token string, ttl time.Duration) {
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookie,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(ttl.Seconds()),
	})
}

// ClearSessionCookie expires the session cookie on the client.
func ClearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookie,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
}

func writeUnauthorized(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"error":   "unauthenticated",
		"message": "a valid session is required",
	})
}
