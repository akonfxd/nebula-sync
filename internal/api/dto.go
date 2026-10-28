package api

import (
	"errors"
	"fmt"
	"time"

	"github.com/lovelaze/nebula-sync/internal/config"
	"github.com/lovelaze/nebula-sync/internal/store"
	"github.com/lovelaze/nebula-sync/internal/sync"
	"github.com/lovelaze/nebula-sync/internal/sync/filter"
)

const (
	filterNone    = "none"
	filterInclude = "include"
	filterExclude = "exclude"
)

var (
	errUnknownGravityTable = errors.New("unknown gravity table")
	errUnknownSection      = errors.New("unknown configuration section")
	errUnknownFilter       = errors.New("filter must be none, include or exclude")
)

type userResponse struct {
	ID          int64      `json:"id"`
	Username    string     `json:"username"`
	Role        string     `json:"role"`
	CreatedAt   time.Time  `json:"createdAt"`
	LastLoginAt *time.Time `json:"lastLoginAt"`
}

func newUserResponse(user store.User) userResponse {
	return userResponse{
		ID:          user.ID,
		Username:    user.Username,
		Role:        user.Role,
		CreatedAt:   user.CreatedAt,
		LastLoginAt: user.LastLoginAt,
	}
}

type instanceResponse struct {
	ID          int64     `json:"id"`
	Role        string    `json:"role"`
	Name        string    `json:"name"`
	URL         string    `json:"url"`
	HasPassword bool      `json:"hasPassword"`
	Enabled     bool      `json:"enabled"`
	ProfileID   *int64    `json:"profileId"`
	Position    int       `json:"position"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

// newInstanceResponse deliberately has nowhere to put a password: the stored
// credential must never travel back to a browser.
func newInstanceResponse(instance store.Instance) instanceResponse {
	return instanceResponse{
		ID:          instance.ID,
		Role:        instance.Role,
		Name:        instance.Name,
		URL:         instance.URL,
		HasPassword: instance.HasPassword,
		Enabled:     instance.Enabled,
		ProfileID:   instance.ProfileID,
		Position:    instance.Position,
		UpdatedAt:   instance.UpdatedAt,
	}
}

type instanceRequest struct {
	Role      string  `json:"role"`
	Name      string  `json:"name"`
	URL       string  `json:"url"`
	Password  *string `json:"password"`
	Enabled   *bool   `json:"enabled"`
	ProfileID *int64  `json:"profileId"`
	Position  *int    `json:"position"`
}

func (r instanceRequest) input() store.InstanceInput {
	input := store.InstanceInput{
		Role:      r.Role,
		Name:      r.Name,
		URL:       r.URL,
		Password:  r.Password,
		Enabled:   true,
		ProfileID: r.ProfileID,
	}

	if r.Enabled != nil {
		input.Enabled = *r.Enabled
	}

	if r.Position != nil {
		input.Position = *r.Position
	}

	return input
}

type sectionPayload struct {
	Enabled bool     `json:"enabled"`
	Filter  string   `json:"filter"`
	Keys    []string `json:"keys"`
}

type profileResponse struct {
	ID         int64                     `json:"id"`
	Name       string                    `json:"name"`
	Default    bool                      `json:"default"`
	InUse      int                       `json:"inUse"`
	FullSync   bool                      `json:"fullSync"`
	RunGravity bool                      `json:"runGravity"`
	Gravity    map[string]bool           `json:"gravity"`
	Sections   map[string]sectionPayload `json:"sections"`
}

func newProfileResponse(record store.ProfileRecord) profileResponse {
	return profileResponse{
		ID:         record.ID,
		Name:       record.Profile.Name,
		Default:    record.Default,
		InUse:      record.InUse,
		FullSync:   record.Profile.FullSync,
		RunGravity: record.Profile.RunGravity,
		Gravity:    gravityPayload(record.Profile.GravitySettings),
		Sections:   sectionsPayload(record.Profile.ConfigSettings),
	}
}

type profileRequest struct {
	Name       string                    `json:"name"`
	FullSync   bool                      `json:"fullSync"`
	RunGravity bool                      `json:"runGravity"`
	Gravity    map[string]bool           `json:"gravity"`
	Sections   map[string]sectionPayload `json:"sections"`
}

func (r profileRequest) profile() (*config.Profile, error) {
	gravity, err := gravitySettings(r.Gravity)
	if err != nil {
		return nil, err
	}

	sections, err := configSettings(r.Sections)
	if err != nil {
		return nil, err
	}

	return &config.Profile{
		Name:            r.Name,
		FullSync:        r.FullSync,
		RunGravity:      r.RunGravity,
		GravitySettings: gravity,
		ConfigSettings:  sections,
	}, nil
}

func gravityPayload(settings *config.GravitySettings) map[string]bool {
	if settings == nil {
		settings = &config.GravitySettings{}
	}

	payload := map[string]bool{}
	for table, value := range settings.Tables() {
		payload[table] = *value
	}

	return payload
}

func gravitySettings(payload map[string]bool) (*config.GravitySettings, error) {
	settings := config.GravitySettings{}
	tables := settings.Tables()

	for table, value := range payload {
		field, found := tables[table]
		if !found {
			return nil, fmt.Errorf("%w: %s", errUnknownGravityTable, table)
		}

		*field = value
	}

	return &settings, nil
}

func sectionsPayload(settings *config.ConfigSettings) map[string]sectionPayload {
	if settings == nil {
		settings = &config.ConfigSettings{}
	}

	payload := map[string]sectionPayload{}

	for _, section := range settings.Fields() {
		payload[section.Name] = newSectionPayload(*section.Value)
	}

	return payload
}

func newSectionPayload(setting *config.ConfigSetting) sectionPayload {
	payload := sectionPayload{Filter: filterNone, Keys: []string{}}
	if setting == nil {
		return payload
	}

	payload.Enabled = setting.Enabled

	if setting.Filter == nil || len(setting.Filter.Keys) == 0 {
		return payload
	}

	payload.Keys = setting.Filter.Keys

	switch setting.Filter.Type {
	case filter.Include:
		payload.Filter = filterInclude
	case filter.Exclude:
		payload.Filter = filterExclude
	}

	return payload
}

func configSettings(payload map[string]sectionPayload) (*config.ConfigSettings, error) {
	settings := config.ConfigSettings{}
	fields := map[string]**config.ConfigSetting{}

	for _, section := range settings.Fields() {
		*section.Value = config.NewConfigSetting(false, nil, nil)
		fields[section.Name] = section.Value
	}

	for name, section := range payload {
		field, found := fields[name]
		if !found {
			return nil, fmt.Errorf("%w: %s", errUnknownSection, name)
		}

		setting, err := configSetting(name, section)
		if err != nil {
			return nil, err
		}

		*field = setting
	}

	return &settings, nil
}

func configSetting(name string, section sectionPayload) (*config.ConfigSetting, error) {
	if section.Enabled && !config.SectionSyncable(name) {
		return nil, fmt.Errorf("section %s cannot be synced: %s", name, sectionReason(name))
	}

	switch {
	case len(section.Keys) == 0:
		return config.NewConfigSetting(section.Enabled, nil, nil), nil
	case section.Filter == filterInclude:
		return config.NewConfigSetting(section.Enabled, section.Keys, nil), nil
	case section.Filter == filterExclude:
		return config.NewConfigSetting(section.Enabled, nil, section.Keys), nil
	case section.Filter == filterNone || section.Filter == "":
		return config.NewConfigSetting(section.Enabled, nil, nil), nil
	default:
		return nil, errUnknownFilter
	}
}

func sectionReason(name string) string {
	for _, section := range config.Sections() {
		if section.Name == name {
			return section.Reason
		}
	}

	return ""
}

type replicaOutcome struct {
	Target   string `json:"target"`
	Profile  string `json:"profile"`
	Mode     string `json:"mode"`
	Duration int64  `json:"durationMs"`
	Error    string `json:"error,omitempty"`
}

type outcomeResponse struct {
	Timestamp time.Time        `json:"timestamp"`
	Success   bool             `json:"success"`
	Mode      string           `json:"mode"`
	Duration  int64            `json:"durationMs"`
	Error     string           `json:"error,omitempty"`
	Replicas  []replicaOutcome `json:"replicas"`
}

func newOutcomeResponse(outcome sync.Outcome) outcomeResponse {
	response := outcomeResponse{
		Timestamp: outcome.Timestamp,
		Success:   outcome.Success,
		Mode:      outcome.Mode,
		Duration:  outcome.Duration.Milliseconds(),
		Replicas:  make([]replicaOutcome, 0, len(outcome.Replicas)),
	}

	if outcome.Err != nil {
		response.Error = outcome.Err.Error()
	}

	for _, replica := range outcome.Replicas {
		response.Replicas = append(response.Replicas, replicaOutcome{
			Target:   replica.Target,
			Profile:  replica.Profile,
			Mode:     replica.Mode,
			Duration: replica.Duration.Milliseconds(),
			Error:    errorText(replica.Err),
		})
	}

	return response
}

type runResponse struct {
	ID         int64            `json:"id"`
	StartedAt  time.Time        `json:"startedAt"`
	FinishedAt time.Time        `json:"finishedAt"`
	Duration   int64            `json:"durationMs"`
	Mode       string           `json:"mode"`
	Source     string           `json:"source"`
	Success    bool             `json:"success"`
	Error      string           `json:"error,omitempty"`
	Replicas   []replicaOutcome `json:"replicas"`
}

func newRunResponse(run store.Run) runResponse {
	response := runResponse{
		ID:         run.ID,
		StartedAt:  run.StartedAt,
		FinishedAt: run.FinishedAt,
		Duration:   run.Duration.Milliseconds(),
		Mode:       run.Mode,
		Source:     run.Source,
		Success:    run.Success,
		Error:      run.Error,
		Replicas:   make([]replicaOutcome, 0, len(run.Replicas)),
	}

	for _, replica := range run.Replicas {
		response.Replicas = append(response.Replicas, replicaOutcome{
			Target:   replica.Target,
			Profile:  replica.Profile,
			Mode:     replica.Mode,
			Duration: replica.Duration.Milliseconds(),
			Error:    replica.Error,
		})
	}

	return response
}

func errorText(err error) string {
	if err == nil {
		return ""
	}

	return err.Error()
}
