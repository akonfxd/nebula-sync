package api

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog/log"

	"github.com/lovelaze/nebula-sync/internal/api/ui"
	"github.com/lovelaze/nebula-sync/internal/config"
	"github.com/lovelaze/nebula-sync/internal/store"
	"github.com/lovelaze/nebula-sync/internal/sync"
)

const (
	readHeaderTimeout    = 10 * time.Second
	shutdownTimeout      = 10 * time.Second
	sessionPurgeInterval = time.Hour
	defaultSessionTTL    = 7 * 24 * time.Hour
)

// Controller is the running service, as the api is allowed to see it.
type Controller interface {
	// Sync runs a sync now, returning sync.ErrInProgress when one is already running.
	Sync(ctx context.Context) error
	// Reload rebuilds the sync plan and the schedule from the stored configuration.
	Reload(ctx context.Context) error
	// Schedule reports the active cron expression, empty when nothing is scheduled.
	Schedule() string
	// NextRun reports when the schedule fires next, zero when nothing is scheduled.
	NextRun() time.Time
	// Syncing reports whether a sync is running right now.
	Syncing() bool
}

type Server struct {
	state      *sync.State
	store      *store.Store
	controller Controller
	router     *chi.Mux
	addr       string
	sessionTTL time.Duration
	origins    []string
	logins     *limiter
}

func NewServer(state *sync.State, configStore *store.Store, controller Controller, conf *config.API) *Server {
	if conf == nil {
		conf = &config.API{}
	}

	ttl := conf.SessionTTL
	if ttl <= 0 {
		ttl = defaultSessionTTL
	}

	server := &Server{
		state:      state,
		store:      configStore,
		controller: controller,
		router:     chi.NewRouter(),
		addr:       conf.Addr,
		sessionTTL: ttl,
		origins:    conf.AllowedOrigins,
		logins:     newLimiter(loginAttempts, loginWindow),
	}

	server.routes()

	return server
}

func (s *Server) routes() {
	s.router.Use(recoverer, requestLog, securityHeaders)

	s.router.Get("/health", s.healthHandler)

	s.router.Route("/api/v1", func(r chi.Router) {
		r.Use(noStore, s.sameOrigin)

		// Without this an unknown /api path would fall through to the console
		// and answer a fetch with a page of html.
		r.NotFound(apiNotFound)
		r.MethodNotAllowed(apiMethodNotAllowed)

		r.Get("/info", s.infoHandler)
		r.Post("/auth/bootstrap", s.bootstrapHandler)
		r.Post("/auth/login", s.loginHandler)

		r.Group(func(r chi.Router) {
			r.Use(s.authenticated)

			r.Get("/auth/session", sessionHandler)
			r.Post("/auth/logout", s.logoutHandler)
			r.Post("/auth/password", s.passwordHandler)

			r.Get("/schema", schemaHandler)
			r.Get("/status", s.statusHandler)
			r.Get("/runs", s.runsHandler)
			r.Get("/schedule", s.scheduleHandler)
			r.Get("/instances", s.instancesHandler)
			r.Get("/profiles", s.profilesHandler)

			r.Group(func(r chi.Router) {
				r.Use(adminOnly)

				r.Post("/sync", s.syncHandler)
				r.Put("/schedule", s.updateScheduleHandler)

				r.Post("/instances", s.createInstanceHandler)
				r.Put("/instances/{id}", s.updateInstanceHandler)
				r.Delete("/instances/{id}", s.deleteInstanceHandler)

				r.Post("/profiles", s.createProfileHandler)
				r.Put("/profiles/{id}", s.updateProfileHandler)
				r.Delete("/profiles/{id}", s.deleteProfileHandler)

				r.Get("/users", s.usersHandler)
				r.Post("/users", s.createUserHandler)
				r.Put("/users/{id}", s.updateUserHandler)
				r.Delete("/users/{id}", s.deleteUserHandler)
			})
		})
	})

	s.router.NotFound(ui.Handler().ServeHTTP)
}

func apiNotFound(w http.ResponseWriter, _ *http.Request) {
	writeError(w, http.StatusNotFound, "no such endpoint")
}

func apiMethodNotAllowed(w http.ResponseWriter, _ *http.Request) {
	writeError(w, http.StatusMethodNotAllowed, "method not allowed")
}

func (s *Server) Start(ctx context.Context) {
	server := &http.Server{
		Handler:           s.router,
		Addr:              s.addr,
		ReadHeaderTimeout: readHeaderTimeout,
	}

	if ui.Built() {
		log.Info().Str("addr", s.addr).Msg("Serving the console")
	} else {
		log.Warn().Msg("No console was built into this binary, serving the api only")
	}

	go func() {
		log.Debug().Str("addr", s.addr).Msg("Starting http server")

		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error().Err(err).Msg("Http server stopped unexpectedly")
		}
	}()

	go s.purgeSessions(ctx)

	go func() {
		<-ctx.Done()

		shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), shutdownTimeout)
		defer cancel()

		if err := server.Shutdown(shutdownCtx); err != nil {
			log.Error().Err(err).Msg("Failed to shut down http server")
		}
	}()
}

// purgeSessions drops expired sessions so the table cannot grow without bound.
func (s *Server) purgeSessions(ctx context.Context) {
	if s.store == nil {
		return
	}

	ticker := time.NewTicker(sessionPurgeInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			purged, err := s.store.PurgeExpiredSessions(ctx)
			if err != nil {
				log.Warn().Err(err).Msg("Failed to purge expired sessions")

				continue
			}

			if purged > 0 {
				log.Debug().Int64("sessions", purged).Msg("Purged expired sessions")
			}
		}
	}
}
