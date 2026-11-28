package main

import (
	"context"
	"database/sql"
	"embed"
	"html/template"
	"log"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/go-chi/chi"
	"github.com/joho/godotenv"

	"github.com/Nico-DM/craftingo/internal/catalog"
	"github.com/Nico-DM/craftingo/internal/database"
)

type apiConfig struct {
	DB        *sql.DB
	Q         *database.Queries
	Store     *catalog.Store
	templates *template.Template
}

//go:embed templates/*.html
var templateFiles embed.FS

func main() {
	_ = godotenv.Load(".env")

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		log.Fatal("DATABASE_URL environment variable is not set")
	}
	db, err := catalog.OpenDB(dbURL)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	store := catalog.NewStore(db)
	if err := store.Load(context.Background()); err != nil {
		log.Fatalf("load catalog (did you run the migrations and seed?): %v", err)
	}

	funcs := template.FuncMap{
		"rarityClass": rarityClass,
		"join":        joinNonEmpty,
		"hasPrefix":   hasPrefix,
		"markdown":    markdownText,
		"add":         func(a, b int) int { return a + b },
	}
	tmpl := template.Must(template.New("").Funcs(funcs).ParseFS(templateFiles, "templates/*.html"))

	cfg := &apiConfig{DB: db, Q: database.For(db), Store: store, templates: tmpl}
	catalogResolveName = func(name string) (*catalog.Item, bool) { return store.ItemByName(name) }

	r := chi.NewRouter()
	r.Use(cfg.loadUser)

	r.Get("/", cfg.handleHome)
	r.Get("/items/{id}", cfg.handleItem)
	r.Get("/login", cfg.handleLoginPage)
	r.Post("/login", cfg.handleLogin)
	r.Post("/logout", cfg.handleLogout)
	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})

	r.Group(func(r chi.Router) {
		r.Use(cfg.requireLogin)
		r.Get("/inventory", cfg.handleInventory)
		r.Get("/inventory/units/{id}", cfg.handleInventoryUnit)
		r.Post("/inventory/units", cfg.handleInventoryAdd)
		r.Post("/inventory/units/{id}/quantity", cfg.handleInventoryQuantity)
		r.Post("/inventory/units/{id}/delete", cfg.handleInventoryDelete)
		r.Get("/craft/{id}", cfg.handleCraft)
		r.Post("/craft/{id}", cfg.handleCraft)
		r.Post("/cart", cfg.handleCartAdd)
		r.Post("/cart/{id}/trait", cfg.handleCartTrait)
		r.Post("/cart/{id}/delete", cfg.handleCartDelete)
		r.Post("/cart/clear", cfg.handleCartClear)
		r.Post("/cart/finalize", cfg.handleCartFinalize)
	})

	r.Group(func(r chi.Router) {
		r.Use(cfg.requireDM)
		r.Get("/admin", cfg.handleAdmin)
		r.Get("/admin/items", cfg.handleAdminItems)
		r.Get("/admin/items/{id}", cfg.handleAdminItem)
		r.Post("/admin/items/{id}", cfg.handleAdminItemSave)
		r.Post("/admin/items/{id}/visible", cfg.handleAdminItemVisible)
		r.Post("/admin/items/{id}/recipe", cfg.handleAdminRecipeSave)
		r.Get("/admin/users", cfg.handleAdminUsers)
		r.Post("/admin/users", cfg.handleAdminUserCreate)
		r.Post("/admin/users/{id}/delete", cfg.handleAdminUserDelete)
		r.Post("/admin/reload", cfg.handleAdminReload)
	})

	r.Get("/generate", cfg.handleGenerateIndex)
	r.Get("/generate/{id}", cfg.handleGenerateArchetype)
	r.Get("/generate/{id}/{base}", cfg.handleGenerateCombo)

	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           r,
		ReadHeaderTimeout: 5 * time.Second,
	}
	slog.Info("Serving Craftingo", "port", port)
	log.Fatal(srv.ListenAndServe())
}
