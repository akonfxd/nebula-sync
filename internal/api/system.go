package api

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/lovelaze/nebula-sync/internal/config"
	"github.com/lovelaze/nebula-sync/internal/schedule"
	"github.com/lovelaze/nebula-sync/internal/store"
	"github.com/lovelaze/nebula-sync/internal/sync"
	"github.com/lovelaze/nebula-sync/version"
)

const reloadTimeout = 30 * time.Second

type sectionSchema struct {
	Name     string `json:"name"`
	Syncable bool   `json:"syncable"`
	Reason   string `json:"reason,omitempty"`
}

type schemaResponse struct {
	Sections          []sectionSchema `json:"sections"`
	Gravity           []string        `json:"gravity"`
	Filters           []string        `json:"filters"`
	UserRoles         []string        `json:"userRoles"`
	InstanceRoles     []string        `json:"instanceRoles"`
	PasswordMinLength int             `json:"passwordMinLength"`
}

type scheduleResponse struct {
	Cron    string     `json:"cron"`
	NextRun *time.Time `json:"nextRun"`
}

type statusResponse struct {
	Version  string            `json:"version"`
	Syncing  bool              `json:"syncing"`
	Schedule scheduleResponse  `json:"schedule"`
	Latest   *outcomeResponse  `json:"latest"`
	Outcomes []outcomeResponse `json:"outcomes"`
}

// schemaHandler publishes the vocabulary the console needs, so the ui never
// hardcodes a section name or a role.
func schemaHandler(w http.ResponseWriter, r *http.Request) {
	sections := make([]sectionSchema, 0, len(config.Sections()))
	for _, section := range config.Sections() {
		sections = append(sections, sectionSchema{
			Name:     section.Name,
			Syncable: section.Syncable,
			Reason:   section.Reason,
		})
	}

	writeJSON(w, http.StatusOK, schemaResponse{
		Sections:          sections,
		Gravity:           config.GravityTables(),
		Filters:           []string{filterNone, filterInclude, filterExclude},
		UserRoles:         []string{store.RoleAdmin, store.RoleViewer},
		InstanceRoles:     []string{"primary", "replica"},
		PasswordMinLength: store.MinPasswordLength,
	})
}

func (s *Server) statusHandler(w http.ResponseWriter, r *http.Request) {
	outcomes := s.state.Outcomes()

	status := statusResponse{
		Version:  version.Version,
		Schedule: s.scheduleState(),
		Outcomes: make([]outcomeResponse, 0, len(outcomes)),
	}

	if s.controller != nil {
		status.Syncing = s.controller.Syncing()
	}

	for _, outcome := range outcomes {
		status.Outcomes = append(status.Outcomes, newOutcomeResponse(outcome))
	}

	if len(status.Outcomes) > 0 {
		status.Latest = &status.Outcomes[0]
	}

	writeJSON(w, http.StatusOK, status)
}

func (s *Server) runsHandler(w http.ResponseWriter, r *http.Request) {
	limit := store.DefaultRunLimit

	if raw := r.URL.Query().Get("limit"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid limit")

			return
		}

		limit = parsed
	}

	runs, err := s.store.Runs(r.Context(), limit)
	if err != nil {
		writeStoreError(w, err)

		return
	}

	responses := make([]runResponse, 0, len(runs))
	for _, run := range runs {
		responses = append(responses, newRunResponse(run))
	}

	writeJSON(w, http.StatusOK, responses)
}

func (s *Server) scheduleHandler(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.scheduleState())
}

func (s *Server) updateScheduleHandler(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Cron *string `json:"cron"`
	}

	if !decodeJSON(w, r, &body) {
		return
	}

	cron := ""
	if body.Cron != nil {
		cron = *body.Cron
	}

	if err := schedule.Validate(cron); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())

		return
	}

	if err := s.store.SetSchedule(r.Context(), &cron); err != nil {
		writeStoreError(w, err)

		return
	}

	if !s.applyChange(w, r) {
		return
	}

	writeJSON(w, http.StatusOK, s.scheduleState())
}

func (s *Server) syncHandler(w http.ResponseWriter, r *http.Request) {
	if s.controller == nil {
		writeError(w, http.StatusServiceUnavailable, "no sync controller attached")

		return
	}

	session, _ := sessionFrom(r.Context())
	if session != nil {
		log.Info().Str("user", session.User.Username).Msg("Sync requested")
	}

	switch err := s.controller.Sync(r.Context()); {
	case errors.Is(err, sync.ErrInProgress):
		writeError(w, http.StatusConflict, err.Error())
	case err != nil:
		writeStoreError(w, err)
	default:
		writeJSON(w, http.StatusAccepted, map[string]string{"status": "started"})
	}
}

func (s *Server) scheduleState() scheduleResponse {
	state := scheduleResponse{}
	if s.controller == nil {
		return state
	}

	state.Cron = s.controller.Schedule()

	if next := s.controller.NextRun(); !next.IsZero() {
		state.NextRun = &next
	}

	return state
}

// applyChange pushes a stored change into the running service. It answers the
// request when that fails, because a saved change that is not running is a
// difference the operator has to know about.
func (s *Server) applyChange(w http.ResponseWriter, r *http.Request) bool {
	if s.controller == nil {
		return true
	}

	ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), reloadTimeout)
	defer cancel()

	if err := s.controller.Reload(ctx); err != nil {
		log.Error().Err(err).Msg("Failed to apply the stored configuration")
		writeError(w, http.StatusInternalServerError, "change was saved but could not be applied: "+err.Error())

		return false
	}

	return true
}
