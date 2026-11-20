package main

import (
	"context"
	"net/http"
	"net/url"
	"strings"

	"github.com/Nico-DM/craftingo/internal/database"
)

type ctxKey string

const userKey ctxKey = "user"

type sessionUser struct {
	ID       string
	Username string
	Role     string
}

const sessionCookie = "craftingo_session"

func (cfg *apiConfig) loadUser(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie(sessionCookie)
		if err != nil || c.Value == "" {
			next.ServeHTTP(w, r)
			return
		}
		u, err := cfg.Q.GetSessionUser(r.Context(), database.GetSessionUserParams{
			Token: c.Value,
			Now:   now(),
		})
		if err != nil {
			next.ServeHTTP(w, r)
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), userKey, sessionUser{
			ID: u.ID, Username: u.Username, Role: u.Role,
		})))
	})
}

func currentUser(r *http.Request) (sessionUser, bool) {
	u, ok := r.Context().Value(userKey).(sessionUser)
	return u, ok
}

func (cfg *apiConfig) requireLogin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := currentUser(r); !ok {
			http.Redirect(w, r, "/login?next="+url.QueryEscape(r.URL.RequestURI()), http.StatusSeeOther) // #nosec G710 -- internal path
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (cfg *apiConfig) requireDM(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, ok := currentUser(r)
		if !ok {
			http.Redirect(w, r, "/login?next="+url.QueryEscape(r.URL.RequestURI()), http.StatusSeeOther) // #nosec G710 -- internal path
			return
		}
		if u.Role != "dm" {
			http.Error(w, "Dungeon master access only", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (cfg *apiConfig) handleLoginPage(w http.ResponseWriter, r *http.Request) {
	if _, ok := currentUser(r); ok {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	cfg.render(w, r, "login.html", map[string]any{
		"Title": "Sign in",
		"Next":  r.URL.Query().Get("next"),
		"Error": r.URL.Query().Get("error"),
	})
}

func (cfg *apiConfig) handleLogin(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	username := strings.TrimSpace(r.FormValue("username"))
	password := r.FormValue("password")
	next := r.FormValue("next")
	if next == "" || !strings.HasPrefix(next, "/") || strings.HasPrefix(next, "//") {
		next = "/"
	}
	u, err := cfg.Q.GetUserByUsername(r.Context(), username)
	if err != nil || !checkPassword(u.PasswordHash, password) {
		cfg.render(w, r, "login.html", map[string]any{
			"Title": "Sign in", "Next": next, "Error": "Wrong username or password",
		})
		return
	}
	token, err := newSessionToken()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	expires := expiresAt()
	if err := cfg.Q.CreateSession(r.Context(), database.CreateSessionParams{
		Token: token, UserID: u.ID, ExpiresAt: expires, CreatedAt: now(),
	}); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.SetCookie(w, &http.Cookie{ // #nosec G124 -- Secure is set when serving TLS
		Name: sessionCookie, Value: token, Path: "/", HttpOnly: true,
		SameSite: http.SameSiteLaxMode, Expires: expiresTime(), Secure: r.TLS != nil,
	})
	http.Redirect(w, r, next, http.StatusSeeOther) // #nosec G710 -- next is a validated local path
}

func (cfg *apiConfig) handleLogout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(sessionCookie); err == nil && c.Value != "" {
		_ = cfg.Q.DeleteSession(r.Context(), c.Value)
	}
	http.SetCookie(w, &http.Cookie{ // #nosec G124 -- Secure is set when serving TLS
		Name: sessionCookie, Value: "", Path: "/", MaxAge: -1, HttpOnly: true,
		SameSite: http.SameSiteLaxMode, Secure: r.TLS != nil,
	})
	http.Redirect(w, r, "/", http.StatusSeeOther)
}
