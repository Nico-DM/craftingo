package catalog

import (
	"database/sql"
	"fmt"
	"strings"

	_ "github.com/tursodatabase/libsql-client-go/libsql"
	_ "modernc.org/sqlite"
)

// OpenDB opens a database URL. Local paths and file: URLs use the embedded
// SQLite driver; libsql:// and https:// URLs use Turso.
func OpenDB(url string) (*sql.DB, error) {
	driver := "libsql"
	if strings.HasPrefix(url, "file:") || !strings.Contains(url, "://") {
		driver = "sqlite"
		if !strings.HasPrefix(url, "file:") {
			url = "file:" + url
		}
		// PRAGMAs are per connection, so they must live in the DSN; database/sql
		// may open several connections. WAL keeps reads working during writes.
		sep := "?"
		if strings.Contains(url, "?") {
			sep = "&"
		}
		url += sep + "_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)"
	}
	db, err := sql.Open(driver, url)
	if err != nil {
		return nil, err
	}
	if driver == "sqlite" {
		// A small pool keeps reads and writes from serializing; busy_timeout handles
		// the occasional write lock.
		db.SetMaxOpenConns(4)
	}
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("open database: %w", err)
	}
	return db, nil
}
