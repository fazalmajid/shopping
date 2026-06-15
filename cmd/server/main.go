package main

import (
	"context"
	"database/sql"
	"embed"
	"io/fs"
	"log"
	"net/http"
	"time"

	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"shopping/internal/classifier"
	"shopping/internal/config"
	"shopping/internal/db"
	"shopping/internal/email"
	"shopping/internal/handlers"
	"shopping/internal/sse"
)

//go:embed migrations
var migrationsFS embed.FS

//go:embed static
var staticFS embed.FS

func main() {
	cfg := config.Load()
	ctx := context.Background()

	// ── Database ──────────────────────────────────────────────────────────
	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("pgxpool.New: %v", err)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		log.Fatalf("database ping: %v", err)
	}

	// ── Goose migrations (needs *sql.DB) ──────────────────────────────────
	sqlDB, err := sql.Open("pgx", cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("sql.Open for migrations: %v", err)
	}
	goose.SetBaseFS(migrationsFS)
	if err := goose.SetDialect("postgres"); err != nil {
		log.Fatalf("goose.SetDialect: %v", err)
	}
	if err := goose.Up(sqlDB, "migrations"); err != nil {
		log.Fatalf("goose.Up: %v", err)
	}
	sqlDB.Close()

	// ── Application layer ─────────────────────────────────────────────────
	queries := db.New(pool)

	sections, err := queries.ListSections(ctx)
	if err != nil {
		log.Fatalf("ListSections: %v", err)
	}

	// Find "Other" section ID for classifier fallback.
	otherSectionID := 0
	for _, s := range sections {
		if s.Name == "Other" {
			otherSectionID = s.ID
			break
		}
	}

	// On first run, create a bootstrap invitation so the first user can enrol
	// their passkey without manual database surgery.
	if hasUsers, err := queries.HasUsers(ctx); err != nil {
		log.Printf("checking for existing users: %v", err)
	} else if !hasUsers {
		if cfg.BootstrapEmail == "" {
			log.Printf("*** No users found. Set BOOTSTRAP_EMAIL and restart to receive a registration link. ***")
		} else {
			token, err := queries.CreateInvitation(ctx, cfg.BootstrapEmail, db.SystemUserID)
			if err != nil {
				log.Printf("creating bootstrap invitation: %v", err)
			} else {
				log.Printf("*** First-run bootstrap — register at: %s/invite/%s ***", cfg.WebAuthnOrigin, token)
			}
		}
	}

	broker := sse.NewBroker()
	go broker.Start()

	clf, err := classifier.New(cfg.LlamaServerPath, cfg.LlamaModelPath, queries, broker, sections)
	if err != nil {
		log.Printf("WARNING: LLM classifier unavailable (%v); falling back to stub", err)
		clf, _ = classifier.New("", "", queries, broker, sections)
	}
	defer clf.Stop()

	mailer := email.New(cfg)

	wconfig := &webauthn.Config{
		RPDisplayName: "Shopping List",
		RPID:          cfg.WebAuthnRPID,
		RPOrigins:     []string{cfg.WebAuthnOrigin},
	}
	wauth, err := webauthn.New(wconfig)
	if err != nil {
		log.Fatalf("webauthn.New: %v", err)
	}

	h := handlers.New(queries, broker, wauth, clf, mailer, cfg, otherSectionID)

	// ── Static files (embedded) ───────────────────────────────────────────
	staticSubFS, err := fs.Sub(staticFS, "static")
	if err != nil {
		log.Fatalf("fs.Sub static: %v", err)
	}
	staticHandler := http.FileServer(http.FS(staticSubFS))

	// ── Mux ───────────────────────────────────────────────────────────────
	mux := http.NewServeMux()

	// Static assets
	mux.Handle("GET /static/", http.StripPrefix("/static/", staticHandler))

	// SPA pages: all serve index.html; JS handles routing client-side.
	serveIndex := func(w http.ResponseWriter, r *http.Request) {
		f, err := staticSubFS.Open("index.html")
		if err != nil {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		f.Close()
		http.ServeFileFS(w, r, staticSubFS, "index.html")
	}
	mux.Handle("GET /", h.RequireAuthPage(serveIndex))
	mux.HandleFunc("GET /login", func(w http.ResponseWriter, r *http.Request) {
		if h.IsAuthenticated(r) {
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}
		http.ServeFileFS(w, r, staticSubFS, "index.html")
	})
	mux.HandleFunc("GET /invite/{token}", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFileFS(w, r, staticSubFS, "index.html")
	})

	// Auth API
	mux.HandleFunc("POST /api/auth/login/begin",     h.LoginBegin)
	mux.HandleFunc("POST /api/auth/login/finish",    h.LoginFinish)
	mux.HandleFunc("POST /api/auth/register/begin",  h.RegisterBegin)
	mux.HandleFunc("POST /api/auth/register/finish", h.RegisterFinish)
	mux.Handle("POST /api/auth/logout", h.RequireAuth(http.HandlerFunc(h.Logout)))

	// Items API (literal path before wildcard for correct routing)
	mux.Handle("GET /api/items",              h.RequireAuth(http.HandlerFunc(h.GetItems)))
	mux.Handle("POST /api/items",             h.RequireAuth(http.HandlerFunc(h.AddItem)))
	mux.Handle("PATCH /api/items/{id}/check", h.RequireAuth(http.HandlerFunc(h.CheckItem)))
	mux.Handle("DELETE /api/items/checked",   h.RequireAuth(http.HandlerFunc(h.ClearChecked)))
	mux.Handle("DELETE /api/items/{id}",      h.RequireAuth(http.HandlerFunc(h.DeleteItem)))

	// Sections API
	mux.Handle("GET /api/sections",  h.RequireAuth(http.HandlerFunc(h.GetSections)))
	mux.Handle("POST /api/sections", h.RequireAuth(http.HandlerFunc(h.AddSection)))

	// Invite API
	mux.HandleFunc("GET /api/invite/{token}", h.GetInviteInfo)
	mux.Handle("POST /api/invite", h.RequireAuth(http.HandlerFunc(h.SendInvite)))

	// SSE
	mux.Handle("GET /api/events", h.RequireAuth(http.HandlerFunc(h.ServeSSE)))

	// ── Background cleanup goroutine ──────────────────────────────────────
	go func() {
		for {
			time.Sleep(time.Minute)
			queries.CleanupSessions(context.Background())
		}
	}()

	log.Printf("Listening on %s", cfg.ServerAddr)
	log.Fatal(http.ListenAndServe(cfg.ServerAddr, mux))
}
