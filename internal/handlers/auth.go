package handlers

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/go-webauthn/webauthn/webauthn"
)

// LoginBegin starts a WebAuthn login ceremony.
func (h *Handlers) LoginBegin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email string `json:"email"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.Email) == "" {
		jsonErr(w, "email required", http.StatusBadRequest)
		return
	}

	user, err := h.queries.GetUserByEmail(r.Context(), strings.TrimSpace(req.Email))
	if err != nil || user == nil {
		jsonErr(w, "no passkey registered for this email", http.StatusBadRequest)
		return
	}

	options, sessionData, err := h.wauth.BeginLogin(user)
	if err != nil {
		log.Printf("BeginLogin: %v", err)
		jsonErr(w, "failed to begin login", http.StatusInternalServerError)
		return
	}

	if err := h.storeWebAuthnSession(r.Context(), w, sessionData); err != nil {
		jsonErr(w, "session error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, options)
}

// LoginFinish completes the WebAuthn login ceremony.
func (h *Handlers) LoginFinish(w http.ResponseWriter, r *http.Request) {
	sessionData, err := h.loadWebAuthnSession(r.Context(), r, w)
	if err != nil {
		return
	}

	user, err := h.queries.GetUserByID(r.Context(), sessionData.UserID)
	if err != nil || user == nil {
		jsonErr(w, "user not found", http.StatusUnauthorized)
		return
	}

	credential, err := h.wauth.FinishLogin(user, *sessionData, r)
	if err != nil {
		log.Printf("FinishLogin: %v", err)
		jsonErr(w, "authentication failed", http.StatusUnauthorized)
		return
	}

	h.queries.StoreCredential(r.Context(), user.ID, *credential)

	if err := h.createAppSession(r.Context(), w, user.ID); err != nil {
		jsonErr(w, "session error", http.StatusInternalServerError)
		return
	}
	jsonOK(w)
}

// RegisterBegin starts a WebAuthn registration ceremony (invite token required).
func (h *Handlers) RegisterBegin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Token       string `json:"token"`
		DisplayName string `json:"display_name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonErr(w, "invalid request", http.StatusBadRequest)
		return
	}

	inv, err := h.queries.GetValidInvitation(r.Context(), req.Token)
	if err != nil || inv == nil {
		jsonErr(w, "invalid or expired invitation", http.StatusBadRequest)
		return
	}

	// Get or create user (a previous failed attempt may have already created it).
	user, err := h.queries.GetUserByEmail(r.Context(), inv.Email)
	if err != nil {
		jsonErr(w, "internal error", http.StatusInternalServerError)
		return
	}
	if user == nil {
		dn := strings.TrimSpace(req.DisplayName)
		if dn == "" {
			dn = inv.Email
		}
		user, err = h.queries.CreateUser(r.Context(), inv.Email, dn)
		if err != nil {
			log.Printf("CreateUser: %v", err)
			jsonErr(w, "failed to create user", http.StatusInternalServerError)
			return
		}
	}

	options, sessionData, err := h.wauth.BeginRegistration(user)
	if err != nil {
		log.Printf("BeginRegistration: %v", err)
		jsonErr(w, "failed to begin registration", http.StatusInternalServerError)
		return
	}

	if err := h.storeWebAuthnSession(r.Context(), w, sessionData); err != nil {
		jsonErr(w, "session error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, options)
}

// RegisterFinish completes registration. The invite token is in ?token=<TOKEN>.
func (h *Handlers) RegisterFinish(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	inv, err := h.queries.GetValidInvitation(r.Context(), token)
	if err != nil || inv == nil {
		jsonErr(w, "invalid or expired invitation", http.StatusBadRequest)
		return
	}

	sessionData, err := h.loadWebAuthnSession(r.Context(), r, w)
	if err != nil {
		return
	}

	user, err := h.queries.GetUserByID(r.Context(), sessionData.UserID)
	if err != nil || user == nil {
		jsonErr(w, "user not found", http.StatusInternalServerError)
		return
	}

	credential, err := h.wauth.FinishRegistration(user, *sessionData, r)
	if err != nil {
		log.Printf("FinishRegistration: %v", err)
		jsonErr(w, "registration failed: "+err.Error(), http.StatusBadRequest)
		return
	}

	if err := h.queries.StoreCredential(r.Context(), user.ID, *credential); err != nil {
		log.Printf("StoreCredential: %v", err)
		jsonErr(w, "failed to store credential", http.StatusInternalServerError)
		return
	}

	// Mark invitation used only after the credential is safely stored.
	if err := h.queries.UseInvitation(r.Context(), token); err != nil {
		log.Printf("UseInvitation: %v", err)
	}

	if err := h.createAppSession(r.Context(), w, user.ID); err != nil {
		jsonErr(w, "session error", http.StatusInternalServerError)
		return
	}
	jsonOK(w)
}

func (h *Handlers) Logout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie("session"); err == nil {
		h.queries.DeleteAppSession(r.Context(), cookie.Value)
	}
	clearCookie(w, "session")
	clearCookie(w, "wa_session")
	jsonOK(w)
}

// ---------- helpers ----------

func (h *Handlers) storeWebAuthnSession(ctx context.Context, w http.ResponseWriter, sd *webauthn.SessionData) error {
	data, err := json.Marshal(sd)
	if err != nil {
		return err
	}
	id, err := h.queries.CreateWebAuthnSession(ctx, string(data), time.Now().Add(5*time.Minute))
	if err != nil {
		return err
	}
	http.SetCookie(w, &http.Cookie{
		Name:     "wa_session",
		Value:    id,
		Path:     "/",
		HttpOnly: true,
		Secure:   h.isSecure(),
		SameSite: http.SameSiteStrictMode,
		MaxAge:   300,
	})
	return nil
}

func (h *Handlers) loadWebAuthnSession(ctx context.Context, r *http.Request, w http.ResponseWriter) (*webauthn.SessionData, error) {
	cookie, err := r.Cookie("wa_session")
	if err != nil {
		jsonErr(w, "no challenge session", http.StatusBadRequest)
		return nil, err
	}
	raw, err := h.queries.GetAndDeleteWebAuthnSession(ctx, cookie.Value)
	if err != nil {
		jsonErr(w, "invalid or expired challenge session", http.StatusBadRequest)
		return nil, err
	}
	var sd webauthn.SessionData
	if err := json.Unmarshal([]byte(raw), &sd); err != nil {
		jsonErr(w, "corrupt session data", http.StatusInternalServerError)
		return nil, err
	}
	return &sd, nil
}

func (h *Handlers) createAppSession(ctx context.Context, w http.ResponseWriter, userID []byte) error {
	token, err := h.queries.CreateAppSession(ctx, userID, time.Now().Add(30*24*time.Hour))
	if err != nil {
		return err
	}
	http.SetCookie(w, &http.Cookie{
		Name:     "session",
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   h.isSecure(),
		SameSite: http.SameSiteStrictMode,
		MaxAge:   30 * 24 * 3600,
	})
	return nil
}

func (h *Handlers) isSecure() bool {
	return strings.HasPrefix(h.cfg.WebAuthnOrigin, "https://")
}

func clearCookie(w http.ResponseWriter, name string) {
	http.SetCookie(w, &http.Cookie{Name: name, MaxAge: -1, Path: "/"})
}
