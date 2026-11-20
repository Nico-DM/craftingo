package database

import (
	"reflect"
	"testing"
)

func TestRewriteArgsAnonymousPassthrough(t *testing.T) {
	q, a := rewriteArgs("SELECT 1 WHERE id = ?", []interface{}{"x"})
	if q != "SELECT 1 WHERE id = ?" {
		t.Fatalf("query: %q", q)
	}
	if !reflect.DeepEqual(a, []interface{}{"x"}) {
		t.Fatalf("args: %#v", a)
	}
}

func TestRewriteArgsNumbered(t *testing.T) {
	q, a := rewriteArgs(
		"WHERE (?1 IS NULL OR name LIKE '%' || ?1 || '%') AND category = ?2",
		[]interface{}{"sword", "Weapon"},
	)
	wantQ := "WHERE (? IS NULL OR name LIKE '%' || ? || '%') AND category = ?"
	wantA := []interface{}{"sword", "sword", "Weapon"}
	if q != wantQ {
		t.Fatalf("query:\n got %q\nwant %q", q, wantQ)
	}
	if !reflect.DeepEqual(a, wantA) {
		t.Fatalf("args:\n got %#v\nwant %#v", a, wantA)
	}
}

func TestRewriteArgsTwoDigit(t *testing.T) {
	args := make([]interface{}, 12)
	for i := range args {
		args[i] = i + 1
	}
	q, a := rewriteArgs("SELECT ?10, ?1, ?12", args)
	wantQ := "SELECT ?, ?, ?"
	wantA := []interface{}{10, 1, 12}
	if q != wantQ {
		t.Fatalf("query: %q", q)
	}
	if !reflect.DeepEqual(a, wantA) {
		t.Fatalf("args: %#v", a)
	}
}
