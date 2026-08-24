package compat

import "testing"

func TestCanonicalSQLEquivalentRepresentationsMatch(t *testing.T) {
	tests := []struct {
		name        string
		left, right string
	}{
		{
			name:  "punctuation whitespace",
			left:  "CREATE TABLE s ( a TEXT, b INTEGER )",
			right: "create table s(a text,b integer)",
		},
		{
			name:  "comments are not semantics",
			left:  "CREATE TABLE s (a TEXT /* historical layout */, b INTEGER)",
			right: "CREATE TABLE s(a TEXT,b INTEGER)",
		},
		{
			name:  "alter appended layout",
			left:  "CREATE TABLE s (a TEXT, b INTEGER\n)",
			right: "CREATE TABLE s (a TEXT, b INTEGER)",
		},
		{
			name:  "comment token boundary",
			left:  "CREATE TABLE s (a/**/TEXT)",
			right: "CREATE TABLE s (a TEXT)",
		},
		{
			name:  "apostrophe in line comment",
			left:  "CREATE TABLE t ( -- don't change lexer state\n  a   INTEGER, b TEXT)",
			right: "CREATE TABLE t (a INTEGER,b TEXT)",
		},
		{
			name:  "apostrophe in block comment",
			left:  "CREATE TABLE t ( /* don't change lexer state */ a INTEGER )",
			right: "CREATE TABLE t (a INTEGER)",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got, want := canonicalSQL(tt.left), canonicalSQL(tt.right); got != want {
				t.Fatalf("canonical SQL differs\nleft:  %q\nright: %q", got, want)
			}
		})
	}
}

func TestCanonicalSQLBehavioralDifferencesRemainDistinct(t *testing.T) {
	tests := []struct {
		name        string
		left, right string
	}{
		{"literal", "CREATE TABLE s(a TEXT DEFAULT 'x  y')", "CREATE TABLE s(a TEXT DEFAULT 'x y')"},
		{"literal comment marker", "CREATE TABLE s(a TEXT DEFAULT 'foo -- bar')", "CREATE TABLE s(a TEXT DEFAULT 'foo bar')"},
		{"quoted identifier", "CREATE TABLE \"a  b\"(id INTEGER)", "CREATE TABLE \"a b\"(id INTEGER)"},
		{"check", "CREATE TABLE s(a INTEGER CHECK(a > 0))", "CREATE TABLE s(a INTEGER CHECK(a >= 0))"},
		{"conflict", "CREATE TABLE s(a TEXT UNIQUE)", "CREATE TABLE s(a TEXT UNIQUE ON CONFLICT REPLACE)"},
		{"deferrable", "CREATE TABLE s(a INTEGER REFERENCES p(id))", "CREATE TABLE s(a INTEGER REFERENCES p(id) DEFERRABLE)"},
		{"partial index", "CREATE INDEX i ON s(a) WHERE a > 0", "CREATE INDEX i ON s(a) WHERE a >= 0"},
		{"trigger", "CREATE TRIGGER t AFTER INSERT ON s BEGIN SELECT 1; END", "CREATE TRIGGER t AFTER INSERT ON s BEGIN SELECT 2; END"},
		{"fts tokenizer", "CREATE VIRTUAL TABLE f USING fts5(body, tokenize='porter')", "CREATE VIRTUAL TABLE f USING fts5(body, tokenize='trigram')"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if canonicalSQL(tt.left) == canonicalSQL(tt.right) {
				t.Fatalf("%s unexpectedly collided", tt.name)
			}
		})
	}
}
