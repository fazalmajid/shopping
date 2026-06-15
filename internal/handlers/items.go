package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"shopping/internal/classifier"
	"shopping/internal/db"
	"shopping/internal/sse"
)

func (h *Handlers) GetItems(w http.ResponseWriter, r *http.Request) {
	items, err := h.queries.ListUncheckedItems(r.Context())
	if err != nil {
		jsonErr(w, "failed to list items", http.StatusInternalServerError)
		return
	}
	sections, err := h.queries.ListSections(r.Context())
	if err != nil {
		jsonErr(w, "failed to list sections", http.StatusInternalServerError)
		return
	}
	if items == nil {
		items = []db.Item{}
	}
	if sections == nil {
		sections = []db.Section{}
	}
	writeJSON(w, map[string]any{
		"items":    items,
		"sections": sections,
	})
}

func (h *Handlers) AddItem(w http.ResponseWriter, r *http.Request) {
	user := userFromCtx(r)
	var req struct {
		Text      string `json:"text"`
		SectionID *int   `json:"section_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonErr(w, "invalid request", http.StatusBadRequest)
		return
	}
	req.Text = strings.TrimSpace(req.Text)
	if req.Text == "" {
		jsonErr(w, "text is required", http.StatusBadRequest)
		return
	}

	// Manual section chosen by the user — use it immediately.
	if req.SectionID != nil {
		item, err := h.queries.AddItem(r.Context(), req.Text, req.SectionID, user.ID)
		if err != nil {
			jsonErr(w, "failed to add item", http.StatusInternalServerError)
			return
		}
		h.queries.UpsertItemSection(r.Context(), req.Text, *req.SectionID, "manual")
		h.broker.Publish(sse.Event{Type: "item_added", Data: item})
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(item)
		return
	}

	// Check the lookup cache before invoking the LLM.
	if cachedID, found, _ := h.queries.LookupItemSection(r.Context(), req.Text); found {
		item, err := h.queries.AddItem(r.Context(), req.Text, &cachedID, user.ID)
		if err != nil {
			jsonErr(w, "failed to add item", http.StatusInternalServerError)
			return
		}
		h.broker.Publish(sse.Event{Type: "item_added", Data: item})
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(item)
		return
	}

	// No cache hit: add with nil section and classify asynchronously.
	item, err := h.queries.AddItem(r.Context(), req.Text, nil, user.ID)
	if err != nil {
		jsonErr(w, "failed to add item", http.StatusInternalServerError)
		return
	}
	h.broker.Publish(sse.Event{Type: "item_added", Data: item})

	if h.clf != nil {
		h.clf.Enqueue(classifier.ClassifyRequest{ItemID: item.ID, Text: req.Text})
	} else if h.otherSectionID > 0 {
		// No classifier available: assign to "Other" synchronously.
		h.queries.UpdateItemSection(r.Context(), item.ID, h.otherSectionID)
		sid := h.otherSectionID
		item.SectionID = &sid
		h.broker.Publish(sse.Event{
			Type: "item_classified",
			Data: map[string]any{"id": item.ID, "section_id": sid},
		})
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(item)
}

func (h *Handlers) CheckItem(w http.ResponseWriter, r *http.Request) {
	user := userFromCtx(r)
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		jsonErr(w, "invalid id", http.StatusBadRequest)
		return
	}
	var req struct {
		Checked bool `json:"checked"`
	}
	json.NewDecoder(r.Body).Decode(&req)

	if err := h.queries.CheckItem(r.Context(), id, req.Checked, user.ID); err != nil {
		jsonErr(w, "failed to update item", http.StatusInternalServerError)
		return
	}
	h.broker.Publish(sse.Event{
		Type: "item_checked",
		Data: map[string]any{"id": id, "checked": req.Checked},
	})
	jsonOK(w)
}

func (h *Handlers) DeleteItem(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		jsonErr(w, "invalid id", http.StatusBadRequest)
		return
	}
	if err := h.queries.DeleteItem(r.Context(), id); err != nil {
		jsonErr(w, "failed to delete item", http.StatusInternalServerError)
		return
	}
	h.broker.Publish(sse.Event{Type: "item_deleted", Data: map[string]any{"id": id}})
	jsonOK(w)
}

func (h *Handlers) ClearChecked(w http.ResponseWriter, r *http.Request) {
	ids, err := h.queries.ClearCheckedItems(r.Context())
	if err != nil {
		jsonErr(w, "failed to clear items", http.StatusInternalServerError)
		return
	}
	if ids == nil {
		ids = []int64{}
	}
	h.broker.Publish(sse.Event{Type: "items_cleared", Data: map[string]any{"deleted_ids": ids}})
	jsonOK(w)
}
