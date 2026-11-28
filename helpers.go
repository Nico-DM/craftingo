package main

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"html/template"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"

	"github.com/Nico-DM/craftingo/internal/auth"
)

// markdownRenderer is safe for concurrent use. Goldmark does not emit raw HTML
// unless explicitly allowed, so catalog descriptions cannot inject markup.
var markdownRenderer = goldmark.New(
	goldmark.WithExtensions(extension.GFM),
)

func now() string {
	return time.Now().UTC().Format(time.RFC3339)
}

func expiresAt() string {
	return time.Now().UTC().Add(30 * 24 * time.Hour).Format(time.RFC3339)
}

func expiresTime() time.Time {
	return time.Now().Add(30 * 24 * time.Hour)
}

func newSessionToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func hashPassword(p string) (string, error) { return auth.HashPassword(p) }
func checkPassword(h, p string) bool        { return auth.CheckPassword(h, p) }

func (cfg *apiConfig) render(w http.ResponseWriter, r *http.Request, page string, data map[string]any) {
	if data == nil {
		data = map[string]any{}
	}
	if u, ok := currentUser(r); ok {
		data["User"] = u
	}
	if _, ok := data["Title"]; !ok {
		data["Title"] = "Craftingo"
	}
	var buf bytes.Buffer
	if err := cfg.templates.ExecuteTemplate(&buf, page, data); err != nil {
		log.Printf("render %s: %v", page, err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(buf.Bytes())
}

func rarityClass(rarity string) string {
	switch strings.ToLower(rarity) {
	case "common":
		return "r-common"
	case "uncommon":
		return "r-uncommon"
	case "rare":
		return "r-rare"
	case "very rare":
		return "r-veryrare"
	case "legendary":
		return "r-legendary"
	case "varies":
		return "r-varies"
	default:
		return "r-none"
	}
}

func joinNonEmpty(parts ...any) string {
	var out []string
	for _, raw := range parts {
		var p string
		switch v := raw.(type) {
		case string:
			p = v
		case *string:
			if v != nil {
				p = *v
			}
		}
		if strings.TrimSpace(p) != "" {
			out = append(out, p)
		}
	}
	return strings.Join(out, " · ")
}

func hasPrefix(s, prefix string) bool { return strings.HasPrefix(s, prefix) }

// markdownText renders a catalog description as HTML. Goldmark leaves raw HTML
// out unless explicitly allowed, which is exactly what we want for dataset text.
func markdownText(s string) template.HTML {
	var buf bytes.Buffer
	if err := markdownRenderer.Convert([]byte(s), &buf); err != nil {
		return template.HTML(template.HTMLEscapeString(s)) // #nosec G203 -- escaped above
	}
	return template.HTML(buf.String()) // #nosec G203 -- goldmark escapes raw HTML
}

func urlMsg(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, " ", "+"), "&", "%26")
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
