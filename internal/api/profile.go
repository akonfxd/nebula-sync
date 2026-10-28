package api

import (
	"net/http"

	"github.com/lovelaze/nebula-sync/internal/config"
)

func (s *Server) profilesHandler(w http.ResponseWriter, r *http.Request) {
	records, err := s.store.Profiles(r.Context())
	if err != nil {
		writeStoreError(w, err)

		return
	}

	responses := make([]profileResponse, 0, len(records))
	for _, record := range records {
		responses = append(responses, newProfileResponse(record))
	}

	writeJSON(w, http.StatusOK, responses)
}

func (s *Server) createProfileHandler(w http.ResponseWriter, r *http.Request) {
	profile, ok := decodeProfile(w, r)
	if !ok {
		return
	}

	record, err := s.store.CreateProfile(r.Context(), profile)
	if err != nil {
		writeStoreError(w, err)

		return
	}

	if !s.applyChange(w, r) {
		return
	}

	writeJSON(w, http.StatusCreated, newProfileResponse(*record))
}

func (s *Server) updateProfileHandler(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}

	profile, ok := decodeProfile(w, r)
	if !ok {
		return
	}

	record, err := s.store.UpdateProfile(r.Context(), id, profile)
	if err != nil {
		writeStoreError(w, err)

		return
	}

	if !s.applyChange(w, r) {
		return
	}

	writeJSON(w, http.StatusOK, newProfileResponse(*record))
}

func (s *Server) deleteProfileHandler(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}

	if err := s.store.DeleteProfile(r.Context(), id); err != nil {
		writeStoreError(w, err)

		return
	}

	if !s.applyChange(w, r) {
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func decodeProfile(w http.ResponseWriter, r *http.Request) (*config.Profile, bool) {
	var body profileRequest
	if !decodeJSON(w, r, &body) {
		return nil, false
	}

	profile, err := body.profile()
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())

		return nil, false
	}

	return profile, true
}
