package server

import (
	"encoding/json"
	"net/http"

	"github.com/ASHUTOSH-SWAIN-GIT/casper/internal/action"
)

// actionListItem is the catalog row shape — narrow on purpose so the
// /v1/actions list stays cheap. Full schema lives behind /v1/actions/{type}.
type actionListItem struct {
	Type          string `json:"type"`
	Service       string `json:"service"`
	Description   string `json:"description"`
	Reversibility string `json:"reversibility"`
	PolicyDefault string `json:"policy_default"`
	Resource      string `json:"resource"`
}

// actionDetail extends the list shape with the JSON Schema. The schema
// is decoded into a generic map so the dashboard can pretty-print it
// without re-parsing the bytes; serving the raw bytes is also fine but
// gives the client extra work.
type actionDetail struct {
	actionListItem
	PolicyQuery string         `json:"policy_query"`
	Schema      map[string]any `json:"schema"`
}

func (s *Server) handleListActions(w http.ResponseWriter, _ *http.Request) {
	types := action.Types()
	out := make([]actionListItem, 0, len(types))
	for _, t := range types {
		spec := action.MustLookup(t)
		out = append(out, specToListItem(spec))
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"actions": out,
		"total":   len(out),
	})
}

func (s *Server) handleGetAction(w http.ResponseWriter, r *http.Request) {
	t := r.PathValue("type")
	spec, ok := action.Lookup(t)
	if !ok {
		writeError(w, http.StatusNotFound, "action_not_found",
			"no action registered with type "+t)
		return
	}
	raw, err := action.SchemaJSON(t)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "schema_unavailable", err.Error())
		return
	}
	var schema map[string]any
	if err := json.Unmarshal(raw, &schema); err != nil {
		writeError(w, http.StatusInternalServerError, "schema_invalid",
			"failed to decode schema for "+t+": "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, actionDetail{
		actionListItem: specToListItem(spec),
		PolicyQuery:    spec.PolicyQuery,
		Schema:         schema,
	})
}

func specToListItem(spec action.Spec) actionListItem {
	return actionListItem{
		Type:          spec.Type,
		Service:       spec.Service,
		Description:   spec.Description,
		Reversibility: spec.Reversibility,
		PolicyDefault: spec.PolicyDefault,
		Resource:      spec.Resource,
	}
}
