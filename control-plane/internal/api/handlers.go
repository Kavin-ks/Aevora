package api

import (
	"encoding/json"
	"net/http"
)

// handleHealth is an unauthenticated liveness/readiness probe.
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// locationDTO is the wire shape of a location. Availability is flattened in so
// clients don't reason about gateway internals.
type locationDTO struct {
	Code      string `json:"code"`
	Country   string `json:"country"`
	Available bool   `json:"available"`
	Servers   int    `json:"servers"`
}

// handleLocations returns the country list a client renders on the map.
func (s *Server) handleLocations(w http.ResponseWriter, r *http.Request) {
	locs, err := s.locations.ListLocations(r.Context())
	if err != nil {
		s.log.Error("list locations", "err", err)
		writeError(w, http.StatusInternalServerError, "could not list locations")
		return
	}
	out := make([]locationDTO, 0, len(locs))
	for _, l := range locs {
		out = append(out, locationDTO{
			Code:      l.Code,
			Country:   l.CountryName,
			Available: l.Available(),
			Servers:   l.ServerCount,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"locations": out})
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}
