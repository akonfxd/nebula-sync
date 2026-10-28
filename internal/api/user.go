package api

import (
	"context"
	"net/http"

	"github.com/rs/zerolog/log"

	"github.com/lovelaze/nebula-sync/internal/store"
)

type userRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Role     string `json:"role"`
}

type userUpdate struct {
	Role     *string `json:"role"`
	Password *string `json:"password"`
}

func (s *Server) usersHandler(w http.ResponseWriter, r *http.Request) {
	users, err := s.store.Users(r.Context())
	if err != nil {
		writeStoreError(w, err)

		return
	}

	responses := make([]userResponse, 0, len(users))
	for _, user := range users {
		responses = append(responses, newUserResponse(user))
	}

	writeJSON(w, http.StatusOK, responses)
}

func (s *Server) createUserHandler(w http.ResponseWriter, r *http.Request) {
	var body userRequest
	if !decodeJSON(w, r, &body) {
		return
	}

	if body.Role == "" {
		body.Role = store.RoleViewer
	}

	user, err := s.store.CreateUser(r.Context(), body.Username, body.Password, body.Role)
	if err != nil {
		writeStoreError(w, err)

		return
	}

	log.Info().Str("user", user.Username).Str("role", user.Role).Msg("Created account")
	writeJSON(w, http.StatusCreated, newUserResponse(*user))
}

// updateUserHandler changes another account's role or password. Either change
// ends that account's sessions, so a demotion takes effect at once.
func (s *Server) updateUserHandler(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}

	var body userUpdate
	if !decodeJSON(w, r, &body) {
		return
	}

	if err := s.applyUserChanges(r.Context(), id, body); err != nil {
		writeStoreError(w, err)

		return
	}

	user, err := s.store.User(r.Context(), id)
	if err != nil {
		writeStoreError(w, err)

		return
	}

	writeJSON(w, http.StatusOK, newUserResponse(*user))
}

func (s *Server) applyUserChanges(ctx context.Context, id int64, body userUpdate) error {
	if body.Role != nil {
		if err := s.store.SetRole(ctx, id, *body.Role); err != nil {
			return err
		}
	}

	if body.Password != nil {
		if err := s.store.SetPassword(ctx, id, *body.Password); err != nil {
			return err
		}
	}

	if body.Role == nil && body.Password == nil {
		return nil
	}

	return s.store.DeleteUserSessions(ctx, id)
}

func (s *Server) deleteUserHandler(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}

	if err := s.store.DeleteUser(r.Context(), id); err != nil {
		writeStoreError(w, err)

		return
	}

	w.WriteHeader(http.StatusNoContent)
}
