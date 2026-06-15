package handlers

import (
	"encoding/json"
	"net/http"
	"strings"

	"shopping/internal/db"
	"shopping/internal/sse"
)

func (h *Handlers) GetSections(w http.ResponseWriter, r *http.Request) {
	sections, err := h.queries.ListSections(r.Context())
	if err != nil {
		jsonErr(w, "failed to list sections", http.StatusInternalServerError)
		return
	}
	if sections == nil {
		sections = []db.Section{}
	}
	writeJSON(w, sections)
}

func (h *Handlers) AddSection(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.Name) == "" {
		jsonErr(w, "name is required", http.StatusBadRequest)
		return
	}

	section, err := h.queries.AddSection(r.Context(), req.Name)
	if err != nil {
		if db.IsUniqueViolation(err) {
			jsonErr(w, "a section with that name already exists", http.StatusConflict)
			return
		}
		jsonErr(w, "failed to add section", http.StatusInternalServerError)
		return
	}

	// Keep the classifier's prompt current with the new section.
	if h.clf != nil {
		h.clf.ReloadSections(section)
	}

	h.broker.Publish(sse.Event{Type: "section_added", Data: section})

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(section)
}
