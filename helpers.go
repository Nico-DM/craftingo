package main

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/Nico-DM/craftingo/internal/auth"
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

func urlMsg(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, " ", "+"), "&", "%26")
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
