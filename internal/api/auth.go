package api

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/lovelaze/nebula-sync/internal/store"
)

type credentials struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type sessionResponse struct {
	User      userResponse `json:"user"`
	ExpiresAt time.Time    `json:"expiresAt"`
}

func (s *Server) loginHandler(w http.ResponseWriter, r *http.Request) {
	var creds credentials
	if !decodeJSON(w, r, &creds) {
		return
	}

	key := clientIP(r) + "|" + strings.ToLower(strings.TrimSpace(creds.Username))
	if !s.allowAttempt(w, key) {
		return
	}

	user, err := s.store.Authenticate(r.Context(), creds.Username, creds.Password)
	if err != nil {
		log.Warn().Str("username", creds.Username).Str("client", clientIP(r)).Msg("Failed sign in")
		writeStoreError(w, err)

		return
	}

	s.logins.forget(key)
	s.issueSession(w, r, user.ID)
}

// bootstrapHandler creates the first administrator, and only while there is none.
func (s *Server) bootstrapHandler(w http.ResponseWriter, r *http.Request) {
	if !s.allowAttempt(w, clientIP(r)) {
		return
	}

	count, err := s.store.UserCount(r.Context())
	if err != nil {
		writeStoreError(w, err)

		return
	}

	if count > 0 {
		writeError(w, http.StatusConflict, "an account already exists, sign in instead")

		return
	}

	var creds credentials
	if !decodeJSON(w, r, &creds) {
		return
	}

	user, err := s.store.CreateUser(r.Context(), creds.Username, creds.Password, store.RoleAdmin)
	if err != nil {
		writeStoreError(w, err)

		return
	}

	log.Info().Str("user", user.Username).Msg("Created the first administrator")
	s.issueSession(w, r, user.ID)
}

func (s *Server) logoutHandler(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(sessionCookieName); err == nil {
		if err := s.store.DeleteSession(r.Context(), cookie.Value); err != nil {
			writeStoreError(w, err)

			return
		}
	}

	clearSessionCookie(w, r)
	w.WriteHeader(http.StatusNoContent)
}

func sessionHandler(w http.ResponseWriter, r *http.Request) {
	session, ok := sessionFrom(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "authentication required")

		return
	}

	writeJSON(w, http.StatusOK, sessionResponse{
		User:      newUserResponse(session.User),
		ExpiresAt: session.ExpiresAt,
	})
}

// passwordHandler changes the signed in user's own password and replaces every
// session they hold, so a stolen cookie dies with the old password.
func (s *Server) passwordHandler(w http.ResponseWriter, r *http.Request) {
	session, ok := sessionFrom(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "authentication required")

		return
	}

	var body struct {
		CurrentPassword string `json:"currentPassword"`
		NewPassword     string `json:"newPassword"`
	}

	if !decodeJSON(w, r, &body) {
		return
	}

	if _, err := s.store.Authenticate(r.Context(), session.User.Username, body.CurrentPassword); err != nil {
		writeStoreError(w, err)

		return
	}

	if err := s.store.SetPassword(r.Context(), session.User.ID, body.NewPassword); err != nil {
		writeStoreError(w, err)

		return
	}

	if err := s.store.DeleteUserSessions(r.Context(), session.User.ID); err != nil {
		writeStoreError(w, err)

		return
	}

	log.Info().Str("user", session.User.Username).Msg("Password changed")
	s.issueSession(w, r, session.User.ID)
}

func (s *Server) issueSession(w http.ResponseWriter, r *http.Request, userID int64) {
	token, session, err := s.store.CreateSession(r.Context(), userID, s.sessionTTL)
	if err != nil {
		writeStoreError(w, err)

		return
	}

	setSessionCookie(w, r, token, session.ExpiresAt)
	writeJSON(w, http.StatusOK, sessionResponse{
		User:      newUserResponse(session.User),
		ExpiresAt: session.ExpiresAt,
	})
}

// allowAttempt answers the request itself when the client has run out of attempts.
func (s *Server) allowAttempt(w http.ResponseWriter, key string) bool {
	allowed, retryAfter := s.logins.allow(key)
	if allowed {
		return true
	}

	w.Header().Set("Retry-After", strconv.Itoa(int(retryAfter.Seconds())+1))
	writeError(w, http.StatusTooManyRequests, "too many attempts, try again later")

	return false
}
