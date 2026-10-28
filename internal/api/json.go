package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog/log"

	"github.com/lovelaze/nebula-sync/internal/store"
)

const maxRequestBytes = 1 << 20

var (
	notFoundErrors = []error{
		store.ErrUserNotFound,
		store.ErrInstanceNotFound,
		store.ErrProfileNotFound,
	}

	conflictErrors = []error{
		store.ErrUserExists,
		store.ErrProfileExists,
		store.ErrProfileInUse,
		store.ErrPrimaryExists,
		store.ErrPrimaryRequired,
		store.ErrDefaultProfile,
		store.ErrLastAdmin,
	}

	badRequestErrors = []error{
		store.ErrInvalidRole,
		store.ErrInvalidUsername,
		store.ErrWeakPassword,
		store.ErrPasswordTooLong,
		store.ErrInvalidInstanceRole,
		store.ErrInvalidInstanceURL,
		store.ErrInstanceName,
		store.ErrPasswordRequired,
		store.ErrProfileName,
		errUnknownGravityTable,
		errUnknownSection,
		errUnknownFilter,
	}
)

type errorResponse struct {
	Error string `json:"error"`
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)

	if payload == nil {
		return
	}

	if err := json.NewEncoder(w).Encode(payload); err != nil {
		log.Debug().Err(err).Msg("Failed to write api response")
	}
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, errorResponse{Error: message})
}

// writeStoreError turns a store error into the status code that fits it.
func writeStoreError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, store.ErrInvalidCredentials), errors.Is(err, store.ErrSessionInvalid):
		writeError(w, http.StatusUnauthorized, err.Error())
	case matches(err, notFoundErrors):
		writeError(w, http.StatusNotFound, err.Error())
	case matches(err, conflictErrors):
		writeError(w, http.StatusConflict, err.Error())
	case matches(err, badRequestErrors):
		writeError(w, http.StatusBadRequest, err.Error())
	default:
		log.Error().Err(err).Msg("Api request failed")
		writeError(w, http.StatusInternalServerError, "internal error")
	}
}

func matches(err error, candidates []error) bool {
	for _, candidate := range candidates {
		if errors.Is(err, candidate) {
			return true
		}
	}

	return false
}

func decodeJSON(w http.ResponseWriter, r *http.Request, target any) bool {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxRequestBytes))
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(target); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())

		return false
	}

	return true
}

// pathID reads the {id} url parameter, answering the request itself when it is unusable.
func pathID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "invalid id")

		return 0, false
	}

	return id, true
}
