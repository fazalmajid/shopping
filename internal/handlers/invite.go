package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"
)

// GetInviteInfo returns the email associated with a valid invitation token.
// Used by the registration page to pre-fill the email field.
func (h *Handlers) GetInviteInfo(w http.ResponseWriter, r *http.Request) {
	token := r.PathValue("token")
	inv, err := h.queries.GetValidInvitation(r.Context(), token)
	if err != nil || inv == nil {
		jsonErr(w, "invalid or expired invitation", http.StatusNotFound)
		return
	}
	writeJSON(w, map[string]string{"email": inv.Email})
}

func (h *Handlers) SendInvite(w http.ResponseWriter, r *http.Request) {
	user := userFromCtx(r)
	var req struct {
		Email string `json:"email"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.Email) == "" {
		jsonErr(w, "email is required", http.StatusBadRequest)
		return
	}

	token, err := h.queries.CreateInvitation(r.Context(), strings.TrimSpace(req.Email), user.ID)
	if err != nil {
		log.Printf("CreateInvitation: %v", err)
		jsonErr(w, "failed to create invitation", http.StatusInternalServerError)
		return
	}

	inviteURL := h.cfg.WebAuthnOrigin + "/invite/" + token
	if err := h.mailer.SendInvite(req.Email, inviteURL, user.DisplayName); err != nil {
		// Email is best-effort; the invitation is created in the DB.
		log.Printf("SendInvite email error: %v", err)
	}

	jsonOK(w)
}
