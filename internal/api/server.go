package api

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog/log"

	"github.com/lovelaze/nebula-sync/internal/sync"
)

const (
	readHeaderTimeout = 10 * time.Second
	shutdownTimeout   = 10 * time.Second
)

type Server struct {
	state  *sync.State
	router *chi.Mux
	addr   string
}

func NewServer(state *sync.State, addr string) *Server {
	router := chi.NewRouter()
	server := &Server{
		state:  state,
		router: router,
		addr:   addr,
	}

	router.Get("/health", server.healthHandler)

	return server
}

func (s *Server) Start(ctx context.Context) {
	server := &http.Server{
		Handler:           s.router,
		Addr:              s.addr,
		ReadHeaderTimeout: readHeaderTimeout,
	}

	go func() {
		log.Debug().Str("addr", s.addr).Msg("Starting http server")

		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error().Err(err).Msg("Http server stopped unexpectedly")
		}
	}()

	go func() {
		<-ctx.Done()

		shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), shutdownTimeout)
		defer cancel()

		if err := server.Shutdown(shutdownCtx); err != nil {
			log.Error().Err(err).Msg("Failed to shut down http server")
		}
	}()
}
