package main

import (
	"strings"
	"testing"
)

func TestMarkdownTextEmphasis(t *testing.T) {
	got := string(markdownText("**bold** and *italic*"))
	if !strings.Contains(got, "<strong>bold</strong>") {
		t.Fatalf("missing strong: %q", got)
	}
	if !strings.Contains(got, "<em>italic</em>") {
		t.Fatalf("missing emphasis: %q", got)
	}
}

func TestMarkdownTextTable(t *testing.T) {
	got := string(markdownText("| A | B |\n| --- | --- |\n| 1 | 2 |\n"))
	if !strings.Contains(got, "<table>") {
		t.Fatalf("missing table: %q", got)
	}
	if !strings.Contains(got, "<td>1</td>") {
		t.Fatalf("missing cell: %q", got)
	}
}

func TestMarkdownTextDropsRawHTML(t *testing.T) {
	got := string(markdownText("<script>alert(1)</script>"))
	if strings.Contains(got, "<script>") {
		t.Fatalf("raw HTML was not escaped: %q", got)
	}
}

func TestMarkdownTextEmpty(t *testing.T) {
	if got := string(markdownText("")); got != "" {
		t.Fatalf("expected empty output, got %q", got)
	}
}
