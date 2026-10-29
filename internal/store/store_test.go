package store

import (
	"database/sql"
	"path/filepath"
	"testing"
)

func TestNextBesideAnOlderCache(t *testing.T) {
	dir := t.TempDir()
	old, err := sql.Open("sqlite", filepath.Join(dir, "voip-info.db"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = old.Exec(`CREATE TABLE http_cache (url TEXT PRIMARY KEY, etag TEXT, last_modified TEXT, status INTEGER, body BLOB, fetched_at REAL);
		INSERT INTO http_cache VALUES ('u0', '', '', 200, 'old', 1)`)
	old.Close()
	if err != nil {
		t.Fatal(err)
	}
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if e := s.Get("u0"); e == nil || string(e.Body) != "old" || e.Next != "" {
		t.Errorf("an older entry: %+v", e)
	}
	if err := s.Put("u1", "e", "", "u1?after=c", 200, []byte("new")); err != nil {
		t.Fatal(err)
	}
	if e := s.Get("u1"); e == nil || e.Next != "u1?after=c" || e.ETag != "e" {
		t.Errorf("%+v", e)
	}
	// the last page has no next
	if err := s.Put("u1", "e2", "", "", 200, []byte("new")); err != nil {
		t.Fatal(err)
	}
	if e := s.Get("u1"); e == nil || e.Next != "" {
		t.Errorf("%+v", e)
	}
	// an older version writes six columns, as before
	if _, err := s.db.Exec(`INSERT OR REPLACE INTO http_cache VALUES (?,?,?,?,?,?)`, "u2", "", "", 200, []byte("x"), 1.0); err != nil {
		t.Errorf("http_cache must stay as older versions know it: %v", err)
	}
}
