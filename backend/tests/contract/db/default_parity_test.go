package db_test

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// C9 keeps tts_chunks.status having the same DEFAULT between schema.sql and the migration
// chain.
//
// schema.sql is a hand-maintained composite snapshot; the real DB is built from
// migrations/*.up.sql in order. The two sources once diverged: migration 000006 set DEFAULT
// 'processing', schema.sql said 'pending' — no runtime error (every insert already specified
// status explicitly) but any tool that generates a schema and compares it to the production DB
// would keep reporting "different".
//
// Tests by simulating the final effective DEFAULT across the migration chain (last write wins)
// then comparing with the value declared in schema.sql — no real Postgres needed, in the same
// style as TestPythonSchemaParity.
func TestTTSChunksStatusDefaultParity(t *testing.T) {
	effDefault := ""
	for _, sql := range readUpMigrations(t, filepath.Join("..", "..", "..", "..", "backend", "db", "migrations")) {
		if v, ok := alterStatusDefault(sql); ok {
			effDefault = v
			continue
		}
		if v, ok := createTableStatusDefault(sql); ok {
			effDefault = v
		}
	}
	if effDefault == "" {
		t.Fatalf("no DEFAULT declaration for tts_chunks.status found in any migration — regex may have drifted")
	}

	schema, err := os.ReadFile(filepath.Clean(filepath.Join("..", "..", "..", "..", "backend", "db", "schema.sql")))
	if err != nil {
		t.Fatalf("read schema.sql: %v", err)
	}
	schemaDefault, ok := createTableStatusDefault(string(schema))
	if !ok {
		t.Fatalf("schema.sql does not declare DEFAULT for tts_chunks.status — does the table still exist?")
	}

	if schemaDefault != effDefault {
		t.Errorf("default mismatch: schema.sql '%s' vs full migration chain '%s'. If a migration changes "+
			"the default status, update schema.sql in the same commit (or add a compensating migration) — "+
			"do not have two 'correct' records at the same time.", schemaDefault, effDefault)
	}
}

// readUpMigrations reads all .up.sql files in numeric order.
func readUpMigrations(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read migration directory %s: %v", dir, err)
	}

	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".up.sql") {
			continue
		}
		names = append(names, e.Name())
	}
	sort.Strings(names)

	out := make([]string, 0, len(names))
	for _, n := range names {
		raw, err := os.ReadFile(filepath.Join(dir, n))
		if err != nil {
			t.Fatalf("read migration %s: %v", n, err)
		}
		out = append(out, string(raw))
	}
	return out
}

// ttsChunksCreateRe matches the entire CREATE TABLE tts_chunks block. Columns use
// VARCHAR(..) which contains `)` inside, but no `;` appears inside the block, so `);`
// only matches the block terminator. statusDefaultRe extracts the DEFAULT within that block.
var (
	ttsChunksCreateRe = regexp.MustCompile(`(?s)CREATE TABLE IF NOT EXISTS tts_chunks\s*\((.*?)\)\s*;`)
	statusDefaultRe   = regexp.MustCompile(`(?i)\bstatus\s+VARCHAR\(50\)\s+NOT NULL\s+DEFAULT\s+'([^']+)'`)
	alterStatusRe     = regexp.MustCompile(`(?i)ALTER\s+TABLE\s+tts_chunks\s+ALTER\s+COLUMN\s+status\s+SET\s+DEFAULT\s+'([^']+)'`)
)

// createTableStatusDefault returns the DEFAULT of status when the tts_chunks create block declares it.
func createTableStatusDefault(sql string) (string, bool) {
	m := ttsChunksCreateRe.FindStringSubmatch(sql)
	if m == nil {
		return "", false
	}
	d := statusDefaultRe.FindStringSubmatch(m[1])
	if d == nil {
		return "", false
	}
	return d[1], true
}

// alterStatusDefault returns the DEFAULT set by an ALTER tts_chunks... statement (last write wins).
func alterStatusDefault(sql string) (string, bool) {
	m := alterStatusRe.FindStringSubmatch(sql)
	if m == nil {
		return "", false
	}
	return m[1], true
}
