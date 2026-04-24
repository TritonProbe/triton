package dashboard

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
)

func (s *Server) handleClearAction(w http.ResponseWriter, r *http.Request) {
	var req ClearActionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid clear request", err)
		return
	}
	scope := req.Scope
	if scope == "" {
		scope = "all"
	}
	removed := map[string]int{}
	switch scope {
	case "all":
		probes, err := s.clearStoredResults("probes")
		if err != nil {
			writeAPIError(w, http.StatusInternalServerError, "failed to clear probes", err)
			return
		}
		benches, err := s.clearStoredResults("benches")
		if err != nil {
			writeAPIError(w, http.StatusInternalServerError, "failed to clear benches", err)
			return
		}
		traces, err := s.clearTraces()
		if err != nil {
			writeAPIError(w, http.StatusInternalServerError, "failed to clear traces", err)
			return
		}
		removed["probes"] = probes
		removed["benches"] = benches
		removed["traces"] = traces
	case "probes", "benches":
		count, err := s.clearStoredResults(scope)
		if err != nil {
			writeAPIError(w, http.StatusInternalServerError, "failed to clear "+scope, err)
			return
		}
		removed[scope] = count
	case "traces":
		count, err := s.clearTraces()
		if err != nil {
			writeAPIError(w, http.StatusInternalServerError, "failed to clear traces", err)
			return
		}
		removed["traces"] = count
	default:
		writeAPIError(w, http.StatusBadRequest, "unsupported clear scope", nil)
		return
	}
	s.clearDashboardCaches(scope)
	writeJSON(w, ClearActionResponse{
		Status:  "ok",
		Message: "cleared " + scope,
		Removed: removed,
	})
}

func (s *Server) clearStoredResults(category string) (int, error) {
	return s.store.Clear(category)
}

func (s *Server) clearTraces() (int, error) {
	if s.trace == "" {
		return 0, nil
	}
	entries, err := os.ReadDir(s.trace)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	removed := 0
	for _, entry := range entries {
		if entry.IsDir() || !validTraceName(entry.Name()) {
			continue
		}
		fullPath := filepath.Join(s.trace, entry.Name())
		info, err := os.Lstat(fullPath)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return removed, err
		}
		if info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			continue
		}
		if err := os.Remove(fullPath); err != nil && !os.IsNotExist(err) {
			return removed, err
		}
		removed++
	}
	return removed, nil
}

func (s *Server) clearDashboardCaches(scope string) {
	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()
	switch scope {
	case "probes":
		s.probeCache = map[string]cachedProbeSummary{}
		s.probeListCache = map[string]cachedProbeList{}
	case "benches":
		s.benchCache = map[string]cachedBenchSummary{}
		s.benchListCache = map[string]cachedBenchList{}
	case "traces":
		s.traceListCache = map[string]cachedTraceList{}
	default:
		s.probeCache = map[string]cachedProbeSummary{}
		s.benchCache = map[string]cachedBenchSummary{}
		s.probeListCache = map[string]cachedProbeList{}
		s.benchListCache = map[string]cachedBenchList{}
		s.traceListCache = map[string]cachedTraceList{}
	}
}
