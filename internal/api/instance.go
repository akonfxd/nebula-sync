package api

import (
	"net/http"
)

func (s *Server) instancesHandler(w http.ResponseWriter, r *http.Request) {
	instances, err := s.store.Instances(r.Context())
	if err != nil {
		writeStoreError(w, err)

		return
	}

	responses := make([]instanceResponse, 0, len(instances))
	for _, instance := range instances {
		responses = append(responses, newInstanceResponse(instance))
	}

	writeJSON(w, http.StatusOK, responses)
}

func (s *Server) createInstanceHandler(w http.ResponseWriter, r *http.Request) {
	var body instanceRequest
	if !decodeJSON(w, r, &body) {
		return
	}

	instance, err := s.store.CreateInstance(r.Context(), body.input())
	if err != nil {
		writeStoreError(w, err)

		return
	}

	if !s.applyChange(w, r) {
		return
	}

	writeJSON(w, http.StatusCreated, newInstanceResponse(*instance))
}

func (s *Server) updateInstanceHandler(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}

	var body instanceRequest
	if !decodeJSON(w, r, &body) {
		return
	}

	instance, err := s.store.UpdateInstance(r.Context(), id, body.input())
	if err != nil {
		writeStoreError(w, err)

		return
	}

	if !s.applyChange(w, r) {
		return
	}

	writeJSON(w, http.StatusOK, newInstanceResponse(*instance))
}

func (s *Server) deleteInstanceHandler(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}

	if err := s.store.DeleteInstance(r.Context(), id); err != nil {
		writeStoreError(w, err)

		return
	}

	if !s.applyChange(w, r) {
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
