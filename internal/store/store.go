// Package store keeps the HTTP cache, "seen" markers, settings and daily
// metric snapshots in SQLite (pure Go driver, no cgo).
package store

import (
	"database/sql"
	"errors"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

const schema = `
CREATE TABLE IF NOT EXISTS http_cache (url TEXT PRIMARY KEY, etag TEXT, last_modified TEXT, status INTEGER, body BLOB, fetched_at REAL);
CREATE TABLE IF NOT EXISTS seen (product TEXT PRIMARY KEY, release_id TEXT, seen_at REAL);
CREATE TABLE IF NOT EXISTS settings (key TEXT PRIMARY KEY, value TEXT);
CREATE TABLE IF NOT EXISTS snapshot (product TEXT, day TEXT, metric TEXT, value REAL, PRIMARY KEY (product, day, metric));
`

// Store is the SQLite database.
type Store struct{ db *sql.DB }

// Open opens (creating) voip-info.db in dir.
func Open(dir string) (*Store, error) {
	db, err := sql.Open("sqlite", filepath.Join(dir, "voip-info.db")+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, err
	}
	return &Store{db: db}, nil
}

// Close closes the database.
func (s *Store) Close() error { return s.db.Close() }

// CacheEntry is a stored HTTP response.
type CacheEntry struct {
	ETag, LastModified string
	Status             int
	Body               []byte
	Fetched            time.Time
}

// Age of the entry.
func (e *CacheEntry) Age() time.Duration { return time.Since(e.Fetched) }

func unix(t float64) time.Time { return time.Unix(0, int64(t*1e9)) }
func now() float64             { return float64(time.Now().UnixNano()) / 1e9 }

// Get returns the cached response of url, or nil.
func (s *Store) Get(url string) *CacheEntry {
	var e CacheEntry
	var etag, lm sql.NullString
	var at float64
	err := s.db.QueryRow(`SELECT etag, last_modified, status, body, fetched_at FROM http_cache WHERE url=?`, url).
		Scan(&etag, &lm, &e.Status, &e.Body, &at)
	if err != nil {
		return nil
	}
	e.ETag, e.LastModified, e.Fetched = etag.String, lm.String, unix(at)
	return &e
}

// Put stores a response.
func (s *Store) Put(url, etag, lastModified string, status int, body []byte) error {
	_, err := s.db.Exec(`INSERT OR REPLACE INTO http_cache VALUES (?,?,?,?,?,?)`, url, etag, lastModified, status, body, now())
	return err
}

// Touch marks a cached response as fresh (304 Not Modified).
func (s *Store) Touch(url string) error {
	_, err := s.db.Exec(`UPDATE http_cache SET fetched_at=? WHERE url=?`, now(), url)
	return err
}

// Seen returns the release tag last seen for a product.
func (s *Store) Seen(product string) (string, bool) {
	var tag string
	err := s.db.QueryRow(`SELECT release_id FROM seen WHERE product=?`, product).Scan(&tag)
	return tag, err == nil
}

// MarkSeen stores the release tag seen last.
func (s *Store) MarkSeen(product, tag string) error {
	_, err := s.db.Exec(`INSERT OR REPLACE INTO seen VALUES (?,?,?)`, product, tag, now())
	return err
}

// Setting returns a stored setting.
func (s *Store) Setting(key string) (string, bool) {
	var v string
	err := s.db.QueryRow(`SELECT value FROM settings WHERE key=?`, key).Scan(&v)
	return v, err == nil
}

// SetSetting stores a setting.
func (s *Store) SetSetting(key, value string) error {
	_, err := s.db.Exec(`INSERT OR REPLACE INTO settings VALUES (?,?)`, key, value)
	return err
}

// Snapshot stores the day's value of metrics.
func (s *Store) Snapshot(product, day string, metrics map[string]float64) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	for k, v := range metrics {
		if _, err := tx.Exec(`INSERT OR REPLACE INTO snapshot VALUES (?,?,?,?)`, product, day, k, v); err != nil {
			return errors.Join(err, tx.Rollback())
		}
	}
	return tx.Commit()
}

// Point is a day's value.
type Point struct {
	Day   string
	Value float64
}

// History returns the snapshots of a metric, oldest first.
func (s *Store) History(product, metric string) []Point {
	rows, err := s.db.Query(`SELECT day, value FROM snapshot WHERE product=? AND metric=? ORDER BY day`, product, metric)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []Point
	for rows.Next() {
		var p Point
		if rows.Scan(&p.Day, &p.Value) == nil {
			out = append(out, p)
		}
	}
	return out
}
