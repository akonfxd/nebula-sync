package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/lovelaze/nebula-sync/internal/api/ui"
	"github.com/lovelaze/nebula-sync/internal/config"
	"github.com/lovelaze/nebula-sync/internal/pihole/model"
	"github.com/lovelaze/nebula-sync/internal/store"
	"github.com/lovelaze/nebula-sync/internal/sync"
)

const (
	testOrigin      = "http://example.com"
	adminPassword   = "correct horse battery"
	viewerPassword  = "another long password"
	primaryPassword = "primary-secret"
	testEncKey      = "abababababababababababababababababababababababababababababababab"
)

type fakeController struct {
	syncs     atomic.Int32
	reloads   atomic.Int32
	syncErr   error
	reloadErr error
	cron      string
	next      time.Time
	syncing   bool
}

func (c *fakeController) Sync(_ context.Context) error {
	c.syncs.Add(1)

	return c.syncErr
}

func (c *fakeController) Reload(_ context.Context) error {
	c.reloads.Add(1)

	return c.reloadErr
}

func (c *fakeController) Schedule() string {
	return c.cron
}

func (c *fakeController) NextRun() time.Time {
	return c.next
}

func (c *fakeController) Syncing() bool {
	return c.syncing
}

type testAPI struct {
	server     *Server
	store      *store.Store
	controller *fakeController
	cookie     *http.Cookie
}

func newTestAPI(t *testing.T) *testAPI {
	t.Helper()

	path := filepath.Join(t.TempDir(), "nebula-sync.db")

	configStore, err := store.Open(t.Context(), &config.Store{
		Path:          path,
		EncryptionKey: testEncKey,
		PasswordCost:  store.MinPasswordCost,
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, configStore.Close())
	})

	controller := &fakeController{}

	return &testAPI{
		server:     NewServer(sync.NewState(), configStore, controller, &config.API{Addr: ":0", SessionTTL: time.Hour}),
		store:      configStore,
		controller: controller,
	}
}

func (a *testAPI) request(t *testing.T, method, path string, body any) *http.Request {
	t.Helper()

	var reader io.Reader

	if body != nil {
		encoded, err := json.Marshal(body)
		require.NoError(t, err)
		reader = bytes.NewReader(encoded)
	}

	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Origin", testOrigin)

	if a.cookie != nil {
		req.AddCookie(a.cookie)
	}

	return req
}

func (a *testAPI) send(t *testing.T, req *http.Request) *httptest.ResponseRecorder {
	t.Helper()

	resp := httptest.NewRecorder()
	a.server.router.ServeHTTP(resp, req)

	for _, cookie := range resp.Result().Cookies() {
		if cookie.Name == sessionCookieName {
			a.cookie = cookie
		}
	}

	return resp
}

func (a *testAPI) do(t *testing.T, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()

	return a.send(t, a.request(t, method, path, body))
}

func (a *testAPI) bootstrap(t *testing.T) {
	t.Helper()

	resp := a.do(t, http.MethodPost, "/api/v1/auth/bootstrap",
		credentials{Username: "admin", Password: adminPassword})
	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())
	require.NotNil(t, a.cookie)
}

func decodeBody[T any](t *testing.T, resp *httptest.ResponseRecorder) T {
	t.Helper()

	var payload T
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &payload), resp.Body.String())

	return payload
}

func TestAPI_bootstrapThenSignIn(t *testing.T) {
	api := newTestAPI(t)

	info := decodeBody[infoResponse](t, api.do(t, http.MethodGet, "/api/v1/info", nil))
	assert.False(t, info.Initialized)
	assert.False(t, info.Authenticated)

	api.bootstrap(t)

	info = decodeBody[infoResponse](t, api.do(t, http.MethodGet, "/api/v1/info", nil))
	assert.True(t, info.Initialized)
	assert.True(t, info.Authenticated)

	second := api.do(t, http.MethodPost, "/api/v1/auth/bootstrap",
		credentials{Username: "intruder", Password: adminPassword})
	assert.Equal(t, http.StatusConflict, second.Code)

	assert.Equal(t, http.StatusNoContent, api.do(t, http.MethodPost, "/api/v1/auth/logout", nil).Code)

	api.cookie = nil
	assert.Equal(t, http.StatusUnauthorized, api.do(t, http.MethodGet, "/api/v1/instances", nil).Code)

	wrong := api.do(t, http.MethodPost, "/api/v1/auth/login",
		credentials{Username: "admin", Password: "not the password"})
	assert.Equal(t, http.StatusUnauthorized, wrong.Code)

	session := api.do(t, http.MethodPost, "/api/v1/auth/login",
		credentials{Username: "admin", Password: adminPassword})
	require.Equal(t, http.StatusOK, session.Code, session.Body.String())
	assert.Equal(t, "admin", decodeBody[sessionResponse](t, session).User.Username)
}

func TestAPI_requiresAuthentication(t *testing.T) {
	api := newTestAPI(t)
	api.bootstrap(t)
	api.cookie = nil

	for _, path := range []string{"/api/v1/instances", "/api/v1/profiles", "/api/v1/status", "/api/v1/schema"} {
		assert.Equal(t, http.StatusUnauthorized, api.do(t, http.MethodGet, path, nil).Code, path)
	}
}

func TestAPI_rejectsCrossOriginWrites(t *testing.T) {
	api := newTestAPI(t)
	api.bootstrap(t)

	foreign := api.request(t, http.MethodPost, "/api/v1/sync", nil)
	foreign.Header.Set("Origin", "http://evil.test")
	assert.Equal(t, http.StatusForbidden, api.send(t, foreign).Code)

	bare := api.request(t, http.MethodPost, "/api/v1/sync", nil)
	bare.Header.Del("Origin")
	assert.Equal(t, http.StatusForbidden, api.send(t, bare).Code)

	referred := api.request(t, http.MethodPost, "/api/v1/sync", nil)
	referred.Header.Del("Origin")
	referred.Header.Set("Referer", testOrigin+"/console")
	assert.Equal(t, http.StatusAccepted, api.send(t, referred).Code)

	// A read is not a write, so it does not need an origin.
	read := api.request(t, http.MethodGet, "/api/v1/status", nil)
	read.Header.Del("Origin")
	assert.Equal(t, http.StatusOK, api.send(t, read).Code)
}

func TestAPI_allowedOriginIsAccepted(t *testing.T) {
	api := newTestAPI(t)
	api.server.origins = []string{"http://localhost:5173"}
	api.bootstrap(t)

	req := api.request(t, http.MethodPost, "/api/v1/sync", nil)
	req.Header.Set("Origin", "http://localhost:5173")

	assert.Equal(t, http.StatusAccepted, api.send(t, req).Code)
}

func (a *testAPI) seed(t *testing.T) {
	t.Helper()

	conf := &config.Config{
		Primary:  model.NewPiHole("http://primary.test", primaryPassword),
		Replicas: []model.PiHole{model.NewPiHole("http://replica.test", "replica-secret")},
		Sync: &config.Sync{
			FullSync:        true,
			GravitySettings: &config.GravitySettings{},
			ConfigSettings:  &config.ConfigSettings{},
		},
	}

	require.NoError(t, a.store.SeedFromEnv(t.Context(), conf))
}

func (a *testAPI) addViewer(t *testing.T) {
	t.Helper()

	resp := a.do(t, http.MethodPost, "/api/v1/users",
		userRequest{Username: "viewer", Password: viewerPassword, Role: store.RoleViewer})
	require.Equal(t, http.StatusCreated, resp.Code, resp.Body.String())
}

// The stored Pi-hole password must never travel back to a browser, in any shape.
func TestAPI_instancePasswordNeverComesBack(t *testing.T) {
	api := newTestAPI(t)
	api.seed(t)
	api.bootstrap(t)

	listed := api.do(t, http.MethodGet, "/api/v1/instances", nil)
	require.Equal(t, http.StatusOK, listed.Code)

	body := listed.Body.String()
	assert.NotContains(t, body, primaryPassword)
	assert.NotContains(t, body, "replica-secret")
	assert.NotContains(t, strings.ToLower(body), `"password"`)

	instances := decodeBody[[]instanceResponse](t, listed)
	require.Len(t, instances, 2)
	assert.Equal(t, "primary", instances[0].Role)
	assert.True(t, instances[0].HasPassword)

	created := api.do(t, http.MethodPost, "/api/v1/instances", map[string]any{
		"role": "replica", "url": "http://replica-two.test", "password": "a-fresh-secret",
	})
	require.Equal(t, http.StatusCreated, created.Code, created.Body.String())
	assert.NotContains(t, created.Body.String(), "a-fresh-secret")
	assert.True(t, decodeBody[instanceResponse](t, created).HasPassword)

	// The write reached the running service.
	assert.Equal(t, int32(1), api.controller.reloads.Load())

	// And it reached the database, encrypted.
	snapshot, err := api.store.LoadSnapshot(t.Context())
	require.NoError(t, err)
	require.Len(t, snapshot.Replicas, 2)
	assert.Equal(t, "a-fresh-secret", snapshot.Replicas[1].PiHole.Password)
}

func TestAPI_viewerCannotWrite(t *testing.T) {
	api := newTestAPI(t)
	api.seed(t)
	api.bootstrap(t)
	api.addViewer(t)

	api.cookie = nil
	signedIn := api.do(t, http.MethodPost, "/api/v1/auth/login",
		credentials{Username: "viewer", Password: viewerPassword})
	require.Equal(t, http.StatusOK, signedIn.Code, signedIn.Body.String())

	assert.Equal(t, http.StatusOK, api.do(t, http.MethodGet, "/api/v1/status", nil).Code)
	assert.Equal(t, http.StatusOK, api.do(t, http.MethodGet, "/api/v1/instances", nil).Code)

	assert.Equal(t, http.StatusForbidden, api.do(t, http.MethodPost, "/api/v1/sync", nil).Code)
	assert.Equal(t, http.StatusForbidden, api.do(t, http.MethodGet, "/api/v1/users", nil).Code)
	assert.Equal(t, http.StatusForbidden, api.do(t, http.MethodDelete, "/api/v1/instances/2", nil).Code)
	assert.Zero(t, api.controller.syncs.Load())
}

func TestAPI_profileWrites(t *testing.T) {
	api := newTestAPI(t)
	api.seed(t)
	api.bootstrap(t)

	listed := decodeBody[[]profileResponse](t, api.do(t, http.MethodGet, "/api/v1/profiles", nil))
	require.Len(t, listed, 1)
	assert.True(t, listed[0].Default)
	assert.Equal(t, 1, listed[0].InUse)

	created := api.do(t, http.MethodPost, "/api/v1/profiles", profileRequest{
		Name:       "dns-only",
		RunGravity: true,
		Gravity:    map[string]bool{config.GravityAdlist: true},
		Sections: map[string]sectionPayload{
			config.SectionDNS: {Enabled: true, Filter: filterInclude, Keys: []string{"upstreams"}},
		},
	})
	require.Equal(t, http.StatusCreated, created.Code, created.Body.String())

	profile := decodeBody[profileResponse](t, created)
	assert.Equal(t, "dns-only", profile.Name)
	assert.True(t, profile.Gravity[config.GravityAdlist])
	assert.False(t, profile.Gravity[config.GravityGroup])
	assert.True(t, profile.Sections[config.SectionDNS].Enabled)
	assert.Equal(t, []string{"upstreams"}, profile.Sections[config.SectionDNS].Keys)
	assert.False(t, profile.Sections[config.SectionMisc].Enabled)
	assert.Equal(t, int32(1), api.controller.reloads.Load())

	deleted := api.do(t, http.MethodDelete, "/api/v1/profiles/"+strconv.FormatInt(profile.ID, 10), nil)
	assert.Equal(t, http.StatusNoContent, deleted.Code)
}

// The webserver section carries the api password, so the api must refuse to enable it.
func TestAPI_profileCannotEnableTheWebserverSection(t *testing.T) {
	api := newTestAPI(t)
	api.seed(t)
	api.bootstrap(t)

	rejected := api.do(t, http.MethodPost, "/api/v1/profiles", profileRequest{
		Name:     "dangerous",
		Sections: map[string]sectionPayload{config.SectionWebserver: {Enabled: true}},
	})
	require.Equal(t, http.StatusBadRequest, rejected.Code)
	assert.Contains(t, rejected.Body.String(), "api password")

	unknown := api.do(t, http.MethodPost, "/api/v1/profiles", profileRequest{
		Name:     "typo",
		Sections: map[string]sectionPayload{"dnss": {Enabled: true}},
	})
	assert.Equal(t, http.StatusBadRequest, unknown.Code)

	badTable := api.do(t, http.MethodPost, "/api/v1/profiles", profileRequest{
		Name:    "typo",
		Gravity: map[string]bool{"adlists": true},
	})
	assert.Equal(t, http.StatusBadRequest, badTable.Code)
}

func TestAPI_syncAndSchedule(t *testing.T) {
	api := newTestAPI(t)
	api.seed(t)
	api.bootstrap(t)

	assert.Equal(t, http.StatusAccepted, api.do(t, http.MethodPost, "/api/v1/sync", nil).Code)
	assert.Equal(t, int32(1), api.controller.syncs.Load())

	api.controller.syncErr = sync.ErrInProgress
	assert.Equal(t, http.StatusConflict, api.do(t, http.MethodPost, "/api/v1/sync", nil).Code)

	bad := api.do(t, http.MethodPut, "/api/v1/schedule", map[string]any{"cron": "every tuesday"})
	assert.Equal(t, http.StatusBadRequest, bad.Code)

	api.controller.cron = "0 * * * *"
	good := api.do(t, http.MethodPut, "/api/v1/schedule", map[string]any{"cron": "0 * * * *"})
	require.Equal(t, http.StatusOK, good.Code, good.Body.String())
	assert.Equal(t, "0 * * * *", decodeBody[scheduleResponse](t, good).Cron)

	stored, err := api.store.Schedule(t.Context())
	require.NoError(t, err)
	require.NotNil(t, stored)
	assert.Equal(t, "0 * * * *", *stored)

	cleared := api.do(t, http.MethodPut, "/api/v1/schedule", map[string]any{"cron": nil})
	require.Equal(t, http.StatusOK, cleared.Code)

	stored, err = api.store.Schedule(t.Context())
	require.NoError(t, err)
	assert.Nil(t, stored)
}

func TestAPI_loginIsRateLimited(t *testing.T) {
	api := newTestAPI(t)
	api.bootstrap(t)
	api.cookie = nil

	key := "192.0.2.1|admin"
	for range loginAttempts {
		allowed, _ := api.server.logins.allow(key)
		require.True(t, allowed)
	}

	blocked := api.do(t, http.MethodPost, "/api/v1/auth/login",
		credentials{Username: "admin", Password: adminPassword})
	require.Equal(t, http.StatusTooManyRequests, blocked.Code)
	assert.NotEmpty(t, blocked.Header().Get("Retry-After"))

	api.server.logins.forget(key)

	allowed := api.do(t, http.MethodPost, "/api/v1/auth/login",
		credentials{Username: "admin", Password: adminPassword})
	assert.Equal(t, http.StatusOK, allowed.Code)
}

func TestAPI_schemaDescribesTheVocabulary(t *testing.T) {
	api := newTestAPI(t)
	api.bootstrap(t)

	schema := decodeBody[schemaResponse](t, api.do(t, http.MethodGet, "/api/v1/schema", nil))

	require.Len(t, schema.Sections, len(config.Sections()))
	assert.Equal(t, config.GravityTables(), schema.Gravity)
	assert.Equal(t, []string{store.RoleAdmin, store.RoleViewer}, schema.UserRoles)
	assert.Equal(t, store.MinPasswordLength, schema.PasswordMinLength)

	byName := map[string]sectionSchema{}
	for _, section := range schema.Sections {
		byName[section.Name] = section
	}

	assert.True(t, byName[config.SectionDNS].Syncable)
	assert.False(t, byName[config.SectionWebserver].Syncable)
	assert.NotEmpty(t, byName[config.SectionWebserver].Reason)
}

// Changing a password must end every session the account holds, including this one.
func TestAPI_passwordChangeReplacesSessions(t *testing.T) {
	api := newTestAPI(t)
	api.bootstrap(t)

	old := api.cookie

	changed := api.do(t, http.MethodPost, "/api/v1/auth/password", map[string]string{
		"currentPassword": adminPassword,
		"newPassword":     viewerPassword,
	})
	require.Equal(t, http.StatusOK, changed.Code, changed.Body.String())
	require.NotEqual(t, old.Value, api.cookie.Value)

	fresh := api.cookie
	api.cookie = old
	assert.Equal(t, http.StatusUnauthorized, api.do(t, http.MethodGet, "/api/v1/auth/session", nil).Code)

	api.cookie = fresh
	assert.Equal(t, http.StatusOK, api.do(t, http.MethodGet, "/api/v1/auth/session", nil).Code)

	wrong := api.do(t, http.MethodPost, "/api/v1/auth/password", map[string]string{
		"currentPassword": "not it",
		"newPassword":     "another password entirely",
	})
	assert.Equal(t, http.StatusUnauthorized, wrong.Code)
}

func TestAPI_runsHistory(t *testing.T) {
	api := newTestAPI(t)
	api.bootstrap(t)

	_, err := api.store.RecordRun(t.Context(), store.Run{
		StartedAt: time.Now().UTC(), FinishedAt: time.Now().UTC(), Mode: "full",
		Source: store.SourceStartup, Success: true,
		Replicas: []store.RunReplica{{Target: "http://replica.test", Profile: "default", Mode: "full"}},
	})
	require.NoError(t, err)

	runs := decodeBody[[]runResponse](t, api.do(t, http.MethodGet, "/api/v1/runs?limit=5", nil))
	require.Len(t, runs, 1)
	assert.Equal(t, store.SourceStartup, runs[0].Source)
	require.Len(t, runs[0].Replicas, 1)
	assert.Equal(t, "http://replica.test", runs[0].Replicas[0].Target)

	assert.Equal(t, http.StatusBadRequest, api.do(t, http.MethodGet, "/api/v1/runs?limit=soon", nil).Code)
}

// A change that cannot be applied must not be reported as a success.
func TestAPI_reportsAFailedReload(t *testing.T) {
	api := newTestAPI(t)
	api.seed(t)
	api.bootstrap(t)

	api.controller.reloadErr = errors.New("primary is unreachable")

	resp := api.do(t, http.MethodPut, "/api/v1/schedule", map[string]any{"cron": "0 * * * *"})
	require.Equal(t, http.StatusInternalServerError, resp.Code)
	assert.Contains(t, resp.Body.String(), "could not be applied")
}

// The console is a catch-all route, so an api path that does not exist must be
// caught before it: a fetch expecting json should never be handed a web page.
func TestAPI_unknownApiPathStaysJSON(t *testing.T) {
	api := newTestAPI(t)
	api.bootstrap(t)

	missing := api.do(t, http.MethodGet, "/api/v1/nope", nil)
	assert.Equal(t, http.StatusNotFound, missing.Code)
	assert.Contains(t, missing.Header().Get("Content-Type"), "application/json")
	assert.NotEmpty(t, decodeBody[errorResponse](t, missing).Error)

	wrongMethod := api.do(t, http.MethodDelete, "/api/v1/status", nil)
	assert.Equal(t, http.StatusMethodNotAllowed, wrongMethod.Code)
	assert.Contains(t, wrongMethod.Header().Get("Content-Type"), "application/json")
}

// Every path the api does not claim belongs to the console, including the
// client side routes a browser asks for after a reload.
func TestAPI_consoleOwnsEverythingElse(t *testing.T) {
	api := newTestAPI(t)

	for _, path := range []string{"/", "/instances", "/profiles"} {
		resp := api.do(t, http.MethodGet, path, nil)

		assert.NotContains(t, resp.Header().Get("Content-Type"), "application/json", path)
		assert.NotEmpty(t, resp.Header().Get("Content-Security-Policy"), path)

		if ui.Built() {
			assert.Equal(t, http.StatusOK, resp.Code, path)
			assert.Contains(t, resp.Body.String(), "<!doctype html>", path)
		}
	}
}

// /health predates the console and stays unauthenticated, so the catch-all
// must not have swallowed it.
func TestAPI_healthIsStillReachable(t *testing.T) {
	api := newTestAPI(t)

	assert.Equal(t, http.StatusInternalServerError, api.do(t, http.MethodGet, "/health", nil).Code)

	api.server.state.Record(nil, nil)
	assert.Equal(t, http.StatusOK, api.do(t, http.MethodGet, "/health", nil).Code)
}
