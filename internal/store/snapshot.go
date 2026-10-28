package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/lovelaze/nebula-sync/internal/config"
	"github.com/lovelaze/nebula-sync/internal/pihole/model"
	"github.com/lovelaze/nebula-sync/internal/sync/filter"
)

const (
	rolePrimary = "primary"
	roleReplica = "replica"

	outcomeSuccess = "success"
	outcomeFailure = "failure"

	filterNone    = "none"
	filterInclude = "include"
	filterExclude = "exclude"
)

var errNoProfile = errors.New("no sync profile stored")

type Replica struct {
	PiHole  model.PiHole
	Profile *config.Profile
}

type Snapshot struct {
	Primary  model.PiHole
	Replicas []Replica
	Sync     *config.Sync
}

func (s *Store) Seeded(ctx context.Context) (bool, error) {
	_, found, err := getSetting(ctx, s.db, settingSeededFromEnv)

	return found, err
}

func (s *Store) SeedFromEnv(ctx context.Context, conf *config.Config) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin seed: %w", err)
	}

	defer func() {
		_ = tx.Rollback()
	}()

	now := time.Now().UTC().Format(time.RFC3339)

	profileID, err := insertProfile(ctx, tx, conf.Sync.Profile(), now)
	if err != nil {
		return err
	}

	if err := s.insertPiHole(ctx, tx, rolePrimary, conf.Primary, nil, 0, now); err != nil {
		return err
	}

	for position, replica := range conf.Replicas {
		if err := s.insertPiHole(ctx, tx, roleReplica, replica, &profileID, position, now); err != nil {
			return err
		}
	}

	if err := insertSchedule(ctx, tx, conf.Sync.Cron); err != nil {
		return err
	}

	if err := s.insertWebhooks(ctx, tx, conf.Sync.WebhookSettings); err != nil {
		return err
	}

	if err := setSetting(ctx, tx, settingSeededFromEnv, now); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit seed: %w", err)
	}

	return nil
}

// SeedEmpty prepares a database that nothing has configured yet: a default
// profile for replicas to be attached to, and no instances at all. It counts as
// the one seed, so a later environment cannot overwrite what the operator then
// enters in the console.
func (s *Store) SeedEmpty(ctx context.Context) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin seed: %w", err)
	}

	defer func() {
		_ = tx.Rollback()
	}()

	now := time.Now().UTC().Format(time.RFC3339)

	profile := config.Profile{Name: config.DefaultProfileName, FullSync: true}
	if _, err := insertProfile(ctx, tx, &profile, now); err != nil {
		return err
	}

	if err := insertSchedule(ctx, tx, nil); err != nil {
		return err
	}

	if err := setSetting(ctx, tx, settingSeededFromEnv, now); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit seed: %w", err)
	}

	return nil
}

func insertProfile(ctx context.Context, tx *sql.Tx, profile *config.Profile, now string) (int64, error) {
	query := `INSERT INTO profile (name, full_sync, run_gravity, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`

	result, err := tx.ExecContext(ctx, query, profile.Name, profile.FullSync, profile.RunGravity, now, now)
	if err != nil {
		return 0, fmt.Errorf("insert profile: %w", err)
	}

	profileID, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("insert profile: %w", err)
	}

	if err := insertProfileGravity(ctx, tx, profileID, profile.GravitySettings); err != nil {
		return 0, err
	}

	if err := insertProfileSections(ctx, tx, profileID, profile.ConfigSettings); err != nil {
		return 0, err
	}

	return profileID, nil
}

func insertProfileGravity(ctx context.Context, q querier, profileID int64, gravity *config.GravitySettings) error {
	if gravity == nil {
		gravity = &config.GravitySettings{}
	}

	query := `INSERT INTO profile_gravity (profile_id, dhcp_leases, "group", adlist, adlist_by_group,
		domainlist, domainlist_by_group, client, client_by_group) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`

	_, err := q.ExecContext(ctx, query, profileID,
		gravity.DHCPLeases, gravity.Group, gravity.Adlist, gravity.AdlistByGroup,
		gravity.Domainlist, gravity.DomainlistByGroup, gravity.Client, gravity.ClientByGroup)
	if err != nil {
		return fmt.Errorf("insert profile gravity: %w", err)
	}

	return nil
}

func insertProfileSections(ctx context.Context, q querier, profileID int64, settings *config.ConfigSettings) error {
	if settings == nil {
		settings = &config.ConfigSettings{}
	}

	sectionQuery := `INSERT INTO profile_section (profile_id, section, enabled, filter_type) VALUES (?, ?, ?, ?)`
	keyQuery := `INSERT INTO profile_key (profile_id, section, key, position) VALUES (?, ?, ?, ?)`

	for _, section := range settings.Fields() {
		setting := *section.Value
		enabled := setting != nil && setting.Enabled

		if _, err := q.ExecContext(ctx, sectionQuery, profileID, section.Name, enabled, filterTypeName(setting)); err != nil {
			return fmt.Errorf("insert profile section %s: %w", section.Name, err)
		}

		if setting == nil || setting.Filter == nil {
			continue
		}

		for position, key := range setting.Filter.Keys {
			if _, err := q.ExecContext(ctx, keyQuery, profileID, section.Name, key, position); err != nil {
				return fmt.Errorf("insert profile key %s.%s: %w", section.Name, key, err)
			}
		}
	}

	return nil
}

func (s *Store) insertPiHole(
	ctx context.Context,
	tx *sql.Tx,
	role string,
	piHole model.PiHole,
	profileID *int64,
	position int,
	now string,
) error {
	password, err := s.cipher.seal(piHole.Password)
	if err != nil {
		return err
	}

	query := `INSERT INTO pihole (role, name, url, password_enc, enabled, profile_id, position, created_at, updated_at)
		VALUES (?, ?, ?, ?, 1, ?, ?, ?, ?)`

	if _, err := tx.ExecContext(ctx, query, role, piHoleName(piHole), piHole.URL.String(), password,
		profileID, position, now, now); err != nil {
		return fmt.Errorf("insert %s: %w", role, err)
	}

	return nil
}

func insertSchedule(ctx context.Context, tx *sql.Tx, cron *string) error {
	value := sql.NullString{}
	if cron != nil {
		value = sql.NullString{String: *cron, Valid: true}
	}

	if _, err := tx.ExecContext(ctx, `INSERT INTO schedule (id, cron) VALUES (1, ?)`, value); err != nil {
		return fmt.Errorf("insert schedule: %w", err)
	}

	return nil
}

func (s *Store) insertWebhooks(ctx context.Context, tx *sql.Tx, webhooks *config.WebhookSettings) error {
	if webhooks == nil {
		webhooks = &config.WebhookSettings{}
	}

	requests := map[string]config.WebhookRequest{
		outcomeSuccess: webhooks.Success,
		outcomeFailure: webhooks.Failure,
	}

	query := `INSERT INTO webhook (outcome, url, method, body_enc, skip_tls) VALUES (?, ?, ?, ?, ?)`
	headerQuery := `INSERT INTO webhook_header (outcome, key, value_enc) VALUES (?, ?, ?)`

	for _, outcome := range []string{outcomeSuccess, outcomeFailure} {
		request := requests[outcome]

		body, err := s.cipher.seal(request.Body)
		if err != nil {
			return err
		}

		if _, err := tx.ExecContext(ctx, query, outcome, request.URL, request.Method, body,
			webhooks.Client.SkipTLSVerification); err != nil {
			return fmt.Errorf("insert webhook %s: %w", outcome, err)
		}

		for key, value := range request.Headers {
			sealed, err := s.cipher.seal(value)
			if err != nil {
				return err
			}

			if _, err := tx.ExecContext(ctx, headerQuery, outcome, key, sealed); err != nil {
				return fmt.Errorf("insert webhook header %s: %w", key, err)
			}
		}
	}

	return nil
}

func (s *Store) LoadSnapshot(ctx context.Context) (*Snapshot, error) {
	profiles, defaultID, err := s.loadProfiles(ctx)
	if err != nil {
		return nil, err
	}

	primary, replicas, err := s.loadPiHoles(ctx, profiles, defaultID)
	if err != nil {
		return nil, err
	}

	sync, err := s.loadSync(ctx, profiles[defaultID])
	if err != nil {
		return nil, err
	}

	return &Snapshot{Primary: primary, Replicas: replicas, Sync: sync}, nil
}

func (s *Store) loadPiHoles(
	ctx context.Context,
	profiles map[int64]*config.Profile,
	defaultID int64,
) (model.PiHole, []Replica, error) {
	var primary model.PiHole

	replicas := []Replica{}

	query := `SELECT role, url, password_enc, profile_id FROM pihole WHERE enabled = 1 ORDER BY role DESC, position, id`

	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return primary, nil, fmt.Errorf("read piholes: %w", err)
	}

	defer rows.Close()

	for rows.Next() {
		var (
			role, rawURL, password string
			profileID              sql.NullInt64
		)

		if err := rows.Scan(&role, &rawURL, &password, &profileID); err != nil {
			return primary, nil, fmt.Errorf("read pihole: %w", err)
		}

		plain, err := s.cipher.open(password)
		if err != nil {
			return primary, nil, fmt.Errorf("%s %s: %w", role, rawURL, err)
		}

		if role == rolePrimary {
			primary = model.NewPiHole(rawURL, plain)
			continue
		}

		replicas = append(replicas, Replica{
			PiHole:  model.NewPiHole(rawURL, plain),
			Profile: resolveProfile(profiles, profileID, defaultID),
		})
	}

	if err := rows.Err(); err != nil {
		return primary, nil, fmt.Errorf("read piholes: %w", err)
	}

	return primary, replicas, nil
}

func resolveProfile(profiles map[int64]*config.Profile, profileID sql.NullInt64, defaultID int64) *config.Profile {
	if profileID.Valid {
		if profile, found := profiles[profileID.Int64]; found {
			return profile
		}
	}

	return profiles[defaultID]
}

func (s *Store) loadSync(ctx context.Context, profile *config.Profile) (*config.Sync, error) {
	sync := config.Sync{
		FullSync:        profile.FullSync,
		RunGravity:      profile.RunGravity,
		GravitySettings: profile.GravitySettings,
		ConfigSettings:  profile.ConfigSettings,
	}

	cron, err := loadSchedule(ctx, s.db)
	if err != nil {
		return nil, err
	}

	sync.Cron = cron

	webhooks, err := s.loadWebhooks(ctx)
	if err != nil {
		return nil, err
	}

	sync.WebhookSettings = webhooks

	return &sync, nil
}

func (s *Store) loadProfiles(ctx context.Context) (map[int64]*config.Profile, int64, error) {
	query := `SELECT p.id, p.name, p.full_sync, p.run_gravity, g.dhcp_leases, g."group", g.adlist, g.adlist_by_group,
		g.domainlist, g.domainlist_by_group, g.client, g.client_by_group
		FROM profile p JOIN profile_gravity g ON g.profile_id = p.id ORDER BY p.id`

	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, 0, fmt.Errorf("read profiles: %w", err)
	}

	defer rows.Close()

	profiles := map[int64]*config.Profile{}

	var defaultID int64

	for rows.Next() {
		var (
			id      int64
			profile config.Profile
			gravity config.GravitySettings
		)

		if err := rows.Scan(&id, &profile.Name, &profile.FullSync, &profile.RunGravity,
			&gravity.DHCPLeases, &gravity.Group, &gravity.Adlist, &gravity.AdlistByGroup,
			&gravity.Domainlist, &gravity.DomainlistByGroup, &gravity.Client, &gravity.ClientByGroup); err != nil {
			return nil, 0, fmt.Errorf("read profile: %w", err)
		}

		profile.GravitySettings = &gravity
		profiles[id] = &profile

		if defaultID == 0 || profile.Name == config.DefaultProfileName {
			defaultID = id
		}
	}

	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("read profiles: %w", err)
	}

	if len(profiles) == 0 {
		return nil, 0, errNoProfile
	}

	if err := s.attachSections(ctx, profiles); err != nil {
		return nil, 0, err
	}

	return profiles, defaultID, nil
}

func (s *Store) attachSections(ctx context.Context, profiles map[int64]*config.Profile) error {
	keys, err := s.loadProfileKeys(ctx)
	if err != nil {
		return err
	}

	stored, err := s.loadProfileSections(ctx, keys)
	if err != nil {
		return err
	}

	for id, profile := range profiles {
		settings := config.ConfigSettings{}

		for _, section := range settings.Fields() {
			if setting, found := stored[id][section.Name]; found {
				*section.Value = setting
				continue
			}

			*section.Value = config.NewConfigSetting(false, nil, nil)
		}

		profile.ConfigSettings = &settings
	}

	return nil
}

func (s *Store) loadProfileSections(
	ctx context.Context,
	keys map[int64]map[string][]string,
) (map[int64]map[string]*config.ConfigSetting, error) {
	query := `SELECT profile_id, section, enabled, filter_type FROM profile_section`

	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("read profile sections: %w", err)
	}

	defer rows.Close()

	stored := map[int64]map[string]*config.ConfigSetting{}

	for rows.Next() {
		var (
			profileID  int64
			section    string
			enabled    bool
			filterType string
		)

		if err := rows.Scan(&profileID, &section, &enabled, &filterType); err != nil {
			return nil, fmt.Errorf("read profile section: %w", err)
		}

		if stored[profileID] == nil {
			stored[profileID] = map[string]*config.ConfigSetting{}
		}

		stored[profileID][section] = newConfigSetting(enabled, filterType, keys[profileID][section])
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read profile sections: %w", err)
	}

	return stored, nil
}

func (s *Store) loadProfileKeys(ctx context.Context) (map[int64]map[string][]string, error) {
	query := `SELECT profile_id, section, key FROM profile_key ORDER BY profile_id, section, position`

	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("read profile keys: %w", err)
	}

	defer rows.Close()

	keys := map[int64]map[string][]string{}

	for rows.Next() {
		var (
			profileID int64
			section   string
			key       string
		)

		if err := rows.Scan(&profileID, &section, &key); err != nil {
			return nil, fmt.Errorf("read profile key: %w", err)
		}

		if keys[profileID] == nil {
			keys[profileID] = map[string][]string{}
		}

		keys[profileID][section] = append(keys[profileID][section], key)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read profile keys: %w", err)
	}

	return keys, nil
}

func loadSchedule(ctx context.Context, q querier) (*string, error) {
	var cron sql.NullString

	err := q.QueryRowContext(ctx, `SELECT cron FROM schedule WHERE id = 1`).Scan(&cron)
	if err != nil {
		return nil, fmt.Errorf("read schedule: %w", err)
	}

	if !cron.Valid || cron.String == "" {
		return nil, nil //nolint:nilnil // no schedule is a valid configuration.
	}

	return &cron.String, nil
}

func (s *Store) loadWebhooks(ctx context.Context) (*config.WebhookSettings, error) {
	headers, err := s.loadWebhookHeaders(ctx)
	if err != nil {
		return nil, err
	}

	rows, err := s.db.QueryContext(ctx, `SELECT outcome, url, method, body_enc, skip_tls FROM webhook`)
	if err != nil {
		return nil, fmt.Errorf("read webhooks: %w", err)
	}

	defer rows.Close()

	webhooks := config.WebhookSettings{}

	for rows.Next() {
		var (
			outcome string
			request config.WebhookRequest
			body    string
			skipTLS bool
		)

		if err := rows.Scan(&outcome, &request.URL, &request.Method, &body, &skipTLS); err != nil {
			return nil, fmt.Errorf("read webhook: %w", err)
		}

		if request.Body, err = s.cipher.open(body); err != nil {
			return nil, fmt.Errorf("webhook %s body: %w", outcome, err)
		}

		request.Headers = headers[outcome]
		webhooks.Client.SkipTLSVerification = webhooks.Client.SkipTLSVerification || skipTLS

		if outcome == outcomeSuccess {
			webhooks.Success = request
		} else {
			webhooks.Failure = request
		}
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read webhooks: %w", err)
	}

	return &webhooks, nil
}

func (s *Store) loadWebhookHeaders(ctx context.Context) (map[string]map[string]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT outcome, key, value_enc FROM webhook_header ORDER BY outcome, key`)
	if err != nil {
		return nil, fmt.Errorf("read webhook headers: %w", err)
	}

	defer rows.Close()

	headers := map[string]map[string]string{}

	for rows.Next() {
		var outcome, key, value string
		if err := rows.Scan(&outcome, &key, &value); err != nil {
			return nil, fmt.Errorf("read webhook header: %w", err)
		}

		plain, err := s.cipher.open(value)
		if err != nil {
			return nil, fmt.Errorf("webhook %s header %s: %w", outcome, key, err)
		}

		if headers[outcome] == nil {
			headers[outcome] = map[string]string{}
		}

		headers[outcome][key] = plain
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read webhook headers: %w", err)
	}

	return headers, nil
}

func piHoleName(piHole model.PiHole) string {
	if piHole.URL == nil {
		return ""
	}

	if piHole.URL.Host != "" {
		return piHole.URL.Host
	}

	return piHole.URL.String()
}

func filterTypeName(setting *config.ConfigSetting) string {
	if setting == nil || setting.Filter == nil || len(setting.Filter.Keys) == 0 {
		return filterNone
	}

	switch setting.Filter.Type {
	case filter.Include:
		return filterInclude
	case filter.Exclude:
		return filterExclude
	default:
		return filterNone
	}
}

func newConfigSetting(enabled bool, filterType string, keys []string) *config.ConfigSetting {
	if len(keys) == 0 {
		return config.NewConfigSetting(enabled, nil, nil)
	}

	switch filterType {
	case filterInclude:
		return config.NewConfigSetting(enabled, keys, nil)
	case filterExclude:
		return config.NewConfigSetting(enabled, nil, keys)
	default:
		return config.NewConfigSetting(enabled, nil, nil)
	}
}
