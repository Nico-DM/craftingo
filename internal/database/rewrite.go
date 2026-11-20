package database

import (
	"context"
	"database/sql"
	"regexp"
	"strconv"
)

// sqlc emits SQLite numbered placeholders (?1, ?2, …). The Turso/libsql
// driver only accepts anonymous "?" placeholders, so every sqlc DBTX goes
// through Rewrite.

var numberedParam = regexp.MustCompile(`\?[0-9]+`)

// For returns Queries that rewrite numbered placeholders for libsql.
func For(db DBTX) *Queries {
	return New(Rewrite(db))
}

// Rewrite wraps db so Exec/Query rewrite ?N placeholders into anonymous ?.
func Rewrite(db DBTX) DBTX {
	if db == nil {
		return nil
	}
	if _, ok := db.(*rewriter); ok {
		return db
	}
	return &rewriter{inner: db}
}

type rewriter struct {
	inner DBTX
}

func rewriteArgs(query string, args []interface{}) (string, []interface{}) {
	if !numberedParam.MatchString(query) {
		return query, args
	}
	var out []interface{}
	rewritten := numberedParam.ReplaceAllStringFunc(query, func(tok string) string {
		n, err := strconv.Atoi(tok[1:])
		if err != nil || n < 1 || n > len(args) {
			// Leave a marker that will still fail clearly at the driver.
			out = append(out, nil)
			return "?"
		}
		out = append(out, args[n-1])
		return "?"
	})
	return rewritten, out
}

func (r *rewriter) ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error) {
	q, a := rewriteArgs(query, args)
	return r.inner.ExecContext(ctx, q, a...)
}

func (r *rewriter) PrepareContext(ctx context.Context, query string) (*sql.Stmt, error) {
	q, _ := rewriteArgs(query, nil)
	return r.inner.PrepareContext(ctx, q)
}

func (r *rewriter) QueryContext(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error) {
	q, a := rewriteArgs(query, args)
	return r.inner.QueryContext(ctx, q, a...)
}

func (r *rewriter) QueryRowContext(ctx context.Context, query string, args ...interface{}) *sql.Row {
	q, a := rewriteArgs(query, args)
	return r.inner.QueryRowContext(ctx, q, a...)
}
