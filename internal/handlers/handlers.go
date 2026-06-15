package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/go-webauthn/webauthn/webauthn"

	"shopping/internal/classifier"
	"shopping/internal/config"
	"shopping/internal/db"
	"shopping/internal/email"
	"shopping/internal/sse"
)

type Handlers struct {
	queries        *db.Queries
	broker         *sse.Broker
	wauth          *webauthn.WebAuthn
	clf            *classifier.Classifier // may be nil
	mailer         *email.Mailer
	cfg            *config.Config
	otherSectionID int // fallback when classifier is unavailable
}

func New(
	queries *db.Queries,
	broker *sse.Broker,
	wauth *webauthn.WebAuthn,
	clf *classifier.Classifier,
	mailer *email.Mailer,
	cfg *config.Config,
	otherSectionID int,
) *Handlers {
	return &Handlers{
		queries:        queries,
		broker:         broker,
		wauth:          wauth,
		clf:            clf,
		mailer:         mailer,
		cfg:            cfg,
		otherSectionID: otherSectionID,
	}
}

func jsonOK(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"ok": true})
}

func jsonErr(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}
