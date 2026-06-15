package handlers

import (
	"context"
	"net/http"

	"shopping/internal/db"
)

type contextKey string

const ctxUser contextKey = "user"

func (h *Handlers) RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user := h.sessionUser(r)
		if user == nil {
			jsonErr(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ctxUser, user)))
	})
}

// RequireAuthPage redirects to /login instead of returning 401.
func (h *Handlers) RequireAuthPage(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := h.sessionUser(r)
		if user == nil {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		next(w, r.WithContext(context.WithValue(r.Context(), ctxUser, user)))
	}
}

func (h *Handlers) IsAuthenticated(r *http.Request) bool {
	return h.sessionUser(r) != nil
}

func (h *Handlers) sessionUser(r *http.Request) *db.User {
	cookie, err := r.Cookie("session")
	if err != nil {
		return nil
	}
	user, _ := h.queries.GetSessionUser(r.Context(), cookie.Value)
	return user
}

func userFromCtx(r *http.Request) *db.User {
	u, _ := r.Context().Value(ctxUser).(*db.User)
	return u
}
