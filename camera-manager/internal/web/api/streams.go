package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"camera-appliance/camera-manager/internal/state"
)

func (s *Server) getSlots(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, s.app.Slots, http.StatusOK)
}

func (s *Server) getBindings(w http.ResponseWriter, r *http.Request) {
	bindings, err := s.app.Store.Bindings(r.Context())
	writeResult(w, bindings, err)
}

func (s *Server) postBinding(w http.ResponseWriter, r *http.Request) {
	var binding state.Binding
	if err := json.NewDecoder(r.Body).Decode(&binding); err != nil {
		writeError(w, err, http.StatusBadRequest)
		return
	}
	err := s.app.Assign(r.Context(), binding)
	writeResult(w, map[string]string{"status": "ok"}, err)
}

func (s *Server) deleteBinding(w http.ResponseWriter, r *http.Request) {
	slotID := strings.TrimPrefix(r.URL.Path, "/api/bindings/")
	err := s.app.RemoveBinding(r.Context(), slotID)
	writeResult(w, map[string]string{"status": "ok"}, err)
}

func (s *Server) replaceBinding(w http.ResponseWriter, r *http.Request) {
	if !strings.HasSuffix(r.URL.Path, "/replace") {
		writeError(w, errors.New("not found"), http.StatusNotFound)
		return
	}
	slotID := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/api/bindings/"), "/replace")
	var binding state.Binding
	if err := json.NewDecoder(r.Body).Decode(&binding); err != nil {
		writeError(w, err, http.StatusBadRequest)
		return
	}
	binding.SlotID = slotID
	err := s.app.Assign(r.Context(), binding)
	writeResult(w, map[string]string{"status": "ok"}, err)
}

func (s *Server) renderGo2RTC(w http.ResponseWriter, r *http.Request) {
	result, err := s.app.RenderConfiguredGo2RTC(r.Context())
	writeResult(w, result, err)
}

func (s *Server) restartGo2RTC(w http.ResponseWriter, r *http.Request) {
	err := s.app.RestartStreams(r.Context())
	if err == nil {
		_ = s.app.Store.AddEvent(r.Context(), "info", "go2rtc.restarted", "go2rtc wurde neu gestartet", nil)
	}
	writeResult(w, map[string]string{"status": "ok"}, err)
}

func (s *Server) getRelayStatus(w http.ResponseWriter, r *http.Request) {
	result, err := s.app.Relays().Statuses(r.Context())
	writeResult(w, result, err)
}

func (s *Server) startRelay(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		writeError(w, errors.New("relay id is required"), http.StatusBadRequest)
		return
	}
	result, err := s.app.Relays().Start(r.Context(), id)
	writeResult(w, result, err)
}

func (s *Server) stopRelay(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		writeError(w, errors.New("relay id is required"), http.StatusBadRequest)
		return
	}
	result, err := s.app.Relays().Stop(r.Context(), id)
	writeResult(w, result, err)
}

func (s *Server) restartRelay(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		writeError(w, errors.New("relay id is required"), http.StatusBadRequest)
		return
	}
	result, err := s.app.Relays().Restart(r.Context(), id)
	writeResult(w, result, err)
}
