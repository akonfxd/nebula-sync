package api

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/rs/zerolog/log"

	"github.com/lovelaze/nebula-sync/internal/store"
)

const sessionCookieName = "nebula_session"

type contextKey int

const sessionContextKey contextKey = iota

// sessionFrom returns the session the authentication middleware attached to the request.
func sessionFrom(ctx context.Context) (*store.Session, bool) {
	session, ok := ctx.Value(sessionContextKey).(*store.Session)

	return session, ok
}

// authenticated rejects any request without a live session cookie.
func (s *Server) authenticated(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(sessionCookieName)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "authentication required")

			return
		}

		session, err := s.store.Session(r.Context(), cookie.Value)
		if err != nil {
			clearSessionCookie(w, r)
			writeStoreError(w, err)

			return
		}

		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), sessionContextKey, session)))
	})
}

// adminOnly keeps viewers out of every route that changes something.
func adminOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		session, ok := sessionFrom(r.Context())
		if !ok || session.User.Role != store.RoleAdmin {
			writeError(w, http.StatusForbidden, "administrator access required")

			return
		}

		next.ServeHTTP(w, r)
	})
}

// sameOrigin is the csrf defence: a cookie authenticated write must come from a page we serve.
func (s *Server) sameOrigin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if safeMethod(r.Method) || s.originAllowed(r) {
			next.ServeHTTP(w, r)

			return
		}

		log.Debug().Str("origin", r.Header.Get("Origin")).Str("host", r.Host).Msg("Rejected cross-origin request")
		writeError(w, http.StatusForbidden, "request origin is not allowed")
	})
}

func (s *Server) originAllowed(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		origin = r.Header.Get("Referer")
	}

	if origin == "" {
		return false
	}

	parsed, err := url.Parse(origin)
	if err != nil || parsed.Host == "" {
		return false
	}

	if strings.EqualFold(parsed.Host, r.Host) {
		return true
	}

	for _, allowed := range s.origins {
		if strings.EqualFold(allowed, parsed.Host) || strings.EqualFold(allowed, parsed.Scheme+"://"+parsed.Host) {
			return true
		}
	}

	return false
}

func safeMethod(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return true
	default:
		return false
	}
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header := w.Header()
		header.Set("X-Content-Type-Options", "nosniff")
		header.Set("X-Frame-Options", "DENY")
		header.Set("Referrer-Policy", "same-origin")

		next.ServeHTTP(w, r)
	})
}

func noStore(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")

		next.ServeHTTP(w, r)
	})
}

func requestLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		wrapped := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		started := time.Now()

		next.ServeHTTP(wrapped, r)

		log.Debug().
			Str("method", r.Method).
			Str("path", r.URL.Path).
			Int("status", wrapped.Status()).
			Dur("duration", time.Since(started)).
			Msg("Api request")
	})
}

// recoverer keeps a panicking handler from taking the sync process down with it.
func recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if recovered := recover(); recovered != nil {
				log.Error().Interface("panic", recovered).Str("path", r.URL.Path).Msg("Api handler panicked")
				writeError(w, http.StatusInternalServerError, "internal error")
			}
		}()

		next.ServeHTTP(w, r)
	})
}

func setSessionCookie(w http.ResponseWriter, r *http.Request, token string, expires time.Time) {
	//nolint:gosec // Secure follows the request scheme; forcing it would break plain http on a lan.
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		Expires:  expires,
		HttpOnly: true,
		Secure:   requestIsTLS(r),
		SameSite: http.SameSiteLaxMode,
	})
}

func clearSessionCookie(w http.ResponseWriter, r *http.Request) {
	//nolint:gosec // Secure follows the request scheme; forcing it would break plain http on a lan.
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   requestIsTLS(r),
		SameSite: http.SameSiteLaxMode,
	})
}

func requestIsTLS(r *http.Request) bool {
	return r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}
