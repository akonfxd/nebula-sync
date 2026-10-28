package api

import (
	"net/http"

	"github.com/lovelaze/nebula-sync/version"
)

type infoResponse struct {
	Version       string `json:"version"`
	Initialized   bool   `json:"initialized"`
	Authenticated bool   `json:"authenticated"`
}

func (s *Server) healthHandler(w http.ResponseWriter, r *http.Request) {
	if s.healthy() {
		w.WriteHeader(http.StatusOK)
	} else {
		w.WriteHeader(http.StatusInternalServerError)
	}
}

func (s *Server) healthy() bool {
	latest, ok := s.state.Latest()

	return ok && latest.Success
}

// infoHandler is the only unauthenticated api route: it tells the console whether
// anyone has signed up yet and whether this browser is already signed in.
func (s *Server) infoHandler(w http.ResponseWriter, r *http.Request) {
	count, err := s.store.UserCount(r.Context())
	if err != nil {
		writeStoreError(w, err)

		return
	}

	info := infoResponse{Version: version.Version, Initialized: count > 0}

	if cookie, err := r.Cookie(sessionCookieName); err == nil {
		if _, err := s.store.Session(r.Context(), cookie.Value); err == nil {
			info.Authenticated = true
		}
	}

	writeJSON(w, http.StatusOK, info)
}
