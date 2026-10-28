package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/lovelaze/nebula-sync/internal/config"
	"github.com/lovelaze/nebula-sync/internal/pihole/model"
	"github.com/lovelaze/nebula-sync/internal/sync/filter"
)

const (
	defaultProfileName = "default"

	rolePrimary = "primary"
	roleReplica = "replica"

	outcomeSuccess = "success"
	outcomeFailure = "failure"

	filterNone    = "none"
	filterInclude = "include"
	filterExclude = "exclude"
)

type Snapshot struct {
	Primary  model.PiHole
	Replicas []model.PiHole
	Sync     *config.Sync
}

type sectionField struct {
	name  string
	value **config.ConfigSetting
}

func sectionFields(settings *config.ConfigSettings) []sectionField {
	return []sectionField{
		{"dns", &settings.DNS},
		{"dhcp", &settings.DHCP},
		{"ntp", &settings.NTP},
		{"resolver", &settings.Resolver},
		{"database", &settings.Database},
		{"webserver", &settings.Webserver},
		{"files", &settings.Files},
		{"misc", &settings.Misc},
		{"debug", &settings.Debug},
	}
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

	profileID, err := insertProfile(ctx, tx, conf.Sync, now)
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

func insertProfile(ctx context.Context, tx *sql.Tx, sync *config.Sync, now string) (int64, error) {
	query := `INSERT INTO profile (name, full_sync, run_gravity, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`

	result, err := tx.ExecContext(ctx, query, defaultProfileName, sync.FullSync, sync.RunGravity, now, now)
	if err != nil {
		return 0, fmt.Errorf("insert profile: %w", err)
	}

	profileID, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("insert profile: %w", err)
	}

	if err := insertProfileGravity(ctx, tx, profileID, sync.GravitySettings); err != nil {
		return 0, err
	}

	if err := insertProfileSections(ctx, tx, profileID, sync.ConfigSettings); err != nil {
		return 0, err
	}

	return profileID, nil
}

func insertProfileGravity(ctx context.Context, tx *sql.Tx, profileID int64, gravity *config.GravitySettings) error {
	if gravity == nil {
		gravity = &config.GravitySettings{}
	}

	query := `INSERT INTO profile_gravity (profile_id, dhcp_leases, "group", adlist, adlist_by_group,
		domainlist, domainlist_by_group, client, client_by_group) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`

	_, err := tx.ExecContext(ctx, query, profileID,
		gravity.DHCPLeases, gravity.Group, gravity.Adlist, gravity.AdlistByGroup,
		gravity.Domainlist, gravity.DomainlistByGroup, gravity.Client, gravity.ClientByGroup)
	if err != nil {
		return fmt.Errorf("insert profile gravity: %w", err)
	}

	return nil
}

func insertProfileSections(ctx context.Context, tx *sql.Tx, profileID int64, settings *config.ConfigSettings) error {
	if settings == nil {
		settings = &config.ConfigSettings{}
	}

	sectionQuery := `INSERT INTO profile_section (profile_id, section, enabled, filter_type) VALUES (?, ?, ?, ?)`
	keyQuery := `INSERT INTO profile_key (profile_id, section, key, position) VALUES (?, ?, ?, ?)`

	for _, section := range sectionFields(settings) {
		setting := *section.value
		enabled := setting != nil && setting.Enabled

		if _, err := tx.ExecContext(ctx, sectionQuery, profileID, section.name, enabled, filterTypeName(setting)); err != nil {
			return fmt.Errorf("insert profile section %s: %w", section.name, err)
		}

		if setting == nil || setting.Filter == nil {
			continue
		}

		for position, key := range setting.Filter.Keys {
			if _, err := tx.ExecContext(ctx, keyQuery, profileID, section.name, key, position); err != nil {
				return fmt.Errorf("insert profile key %s.%s: %w", section.name, key, err)
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
	primary, replicas, err := s.loadPiHoles(ctx)
	if err != nil {
		return nil, err
	}

	sync, err := s.loadSync(ctx)
	if err != nil {
		return nil, err
	}

	return &Snapshot{Primary: primary, Replicas: replicas, Sync: sync}, nil
}

func (s *Store) loadPiHoles(ctx context.Context) (model.PiHole, []model.PiHole, error) {
	var primary model.PiHole

	replicas := []model.PiHole{}

	query := `SELECT role, url, password_enc FROM pihole WHERE enabled = 1 ORDER BY role DESC, position, id`

	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return primary, nil, fmt.Errorf("read piholes: %w", err)
	}

	defer rows.Close()

	for rows.Next() {
		var role, rawURL, password string
		if err := rows.Scan(&role, &rawURL, &password); err != nil {
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

		replicas = append(replicas, model.NewPiHole(rawURL, plain))
	}

	if err := rows.Err(); err != nil {
		return primary, nil, fmt.Errorf("read piholes: %w", err)
	}

	return primary, replicas, nil
}

func (s *Store) loadSync(ctx context.Context) (*config.Sync, error) {
	profileID, sync, err := s.loadProfile(ctx)
	if err != nil {
		return nil, err
	}

	settings, err := s.loadProfileSections(ctx, profileID)
	if err != nil {
		return nil, err
	}

	sync.ConfigSettings = settings

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

	return sync, nil
}

func (s *Store) loadProfile(ctx context.Context) (int64, *config.Sync, error) {
	var (
		profileID int64
		sync      config.Sync
		gravity   config.GravitySettings
	)

	query := `SELECT p.id, p.full_sync, p.run_gravity, g.dhcp_leases, g."group", g.adlist, g.adlist_by_group,
		g.domainlist, g.domainlist_by_group, g.client, g.client_by_group
		FROM profile p JOIN profile_gravity g ON g.profile_id = p.id ORDER BY p.id LIMIT 1`

	err := s.db.QueryRowContext(ctx, query).Scan(&profileID, &sync.FullSync, &sync.RunGravity,
		&gravity.DHCPLeases, &gravity.Group, &gravity.Adlist, &gravity.AdlistByGroup,
		&gravity.Domainlist, &gravity.DomainlistByGroup, &gravity.Client, &gravity.ClientByGroup)
	if err != nil {
		return 0, nil, fmt.Errorf("read profile: %w", err)
	}

	sync.GravitySettings = &gravity

	return profileID, &sync, nil
}

func (s *Store) loadProfileSections(ctx context.Context, profileID int64) (*config.ConfigSettings, error) {
	keys, err := s.loadProfileKeys(ctx, profileID)
	if err != nil {
		return nil, err
	}

	query := `SELECT section, enabled, filter_type FROM profile_section WHERE profile_id = ?`

	rows, err := s.db.QueryContext(ctx, query, profileID)
	if err != nil {
		return nil, fmt.Errorf("read profile sections: %w", err)
	}

	defer rows.Close()

	stored := map[string]*config.ConfigSetting{}

	for rows.Next() {
		var (
			section    string
			enabled    bool
			filterType string
		)

		if err := rows.Scan(&section, &enabled, &filterType); err != nil {
			return nil, fmt.Errorf("read profile section: %w", err)
		}

		stored[section] = newConfigSetting(enabled, filterType, keys[section])
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read profile sections: %w", err)
	}

	settings := config.ConfigSettings{}
	for _, section := range sectionFields(&settings) {
		if setting, found := stored[section.name]; found {
			*section.value = setting
			continue
		}

		*section.value = config.NewConfigSetting(false, nil, nil)
	}

	return &settings, nil
}

func (s *Store) loadProfileKeys(ctx context.Context, profileID int64) (map[string][]string, error) {
	query := `SELECT section, key FROM profile_key WHERE profile_id = ? ORDER BY section, position`

	rows, err := s.db.QueryContext(ctx, query, profileID)
	if err != nil {
		return nil, fmt.Errorf("read profile keys: %w", err)
	}

	defer rows.Close()

	keys := map[string][]string{}

	for rows.Next() {
		var section, key string
		if err := rows.Scan(&section, &key); err != nil {
			return nil, fmt.Errorf("read profile key: %w", err)
		}

		keys[section] = append(keys[section], key)
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
