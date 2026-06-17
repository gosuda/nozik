package db

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gg582/nozik/internal/model"
	_ "modernc.org/sqlite"
)

// DB wraps the SQLite taste database.
type DB struct {
	sql *sql.DB
}

// Open opens (and creates) the SQLite database at path.
func Open(path string) (*DB, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", path+"?_fk=on")
	if err != nil {
		return nil, err
	}
	d := &DB{sql: db}
	if err := d.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	return d, nil
}

func (d *DB) migrate() error {
	schema := `
CREATE TABLE IF NOT EXISTS plays (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    provider_id TEXT NOT NULL,
    provider_track_id TEXT NOT NULL,
    title TEXT NOT NULL,
    artist TEXT NOT NULL,
    album TEXT,
    duration_ms INTEGER DEFAULT 0,
    emotion TEXT DEFAULT 'unknown',
    weight REAL DEFAULT 1.0,
    play_count INTEGER DEFAULT 1,
    last_played DATETIME DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(provider_id, provider_track_id)
);
CREATE INDEX IF NOT EXISTS idx_plays_emotion ON plays(emotion);
CREATE INDEX IF NOT EXISTS idx_plays_artist ON plays(artist);
CREATE INDEX IF NOT EXISTS idx_plays_title ON plays(title);
CREATE VIRTUAL TABLE IF NOT EXISTS plays_fts USING fts5(title, artist, album, content='plays', content_rowid='id');
`
	_, err := d.sql.Exec(schema)
	return err
}

// Close closes the database.
func (d *DB) Close() error { return d.sql.Close() }

// RecordPlay inserts or updates a play record.
func (d *DB) RecordPlay(rec model.PlayRecord) error {
	_, err := d.sql.Exec(`
INSERT INTO plays(provider_id, provider_track_id, title, artist, album, duration_ms, emotion, weight, play_count, last_played)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(provider_id, provider_track_id) DO UPDATE SET
    play_count = play_count + 1,
    weight = MAX(weight, excluded.weight),
    emotion = excluded.emotion,
    last_played = excluded.last_played`,
		rec.ProviderID, rec.ProviderTrackID, rec.Title, rec.Artist, rec.Album,
		rec.DurationMs, string(rec.Emotion), rec.Weight, rec.PlayCount, rec.LastPlayed)
	if err != nil {
		return err
	}
	// Best-effort FTS sync.
	_, _ = d.sql.Exec("INSERT INTO plays_fts(plays_fts) VALUES('rebalance')")
	return nil
}

// RecentPlays returns the most recent play records.
func (d *DB) RecentPlays(limit int) ([]model.PlayRecord, error) {
	rows, err := d.sql.Query(`
SELECT id, provider_id, provider_track_id, title, artist, album, duration_ms, emotion, weight, play_count, last_played
FROM plays ORDER BY last_played DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanRecords(rows)
}

// Search searches the taste DB by text.
func (d *DB) Search(query string, limit int) ([]model.PlayRecord, error) {
	q := "%" + strings.TrimSpace(query) + "%"
	rows, err := d.sql.Query(`
SELECT id, provider_id, provider_track_id, title, artist, album, duration_ms, emotion, weight, play_count, last_played
FROM plays WHERE title LIKE ? OR artist LIKE ? OR album LIKE ? ORDER BY play_count DESC, last_played DESC LIMIT ?`, q, q, q, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanRecords(rows)
}

// Export writes all plays as JSON to w.
func (d *DB) Export(w io.Writer) error {
	recs, err := d.RecentPlays(100000)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(recs)
}

// Import reads JSON play records from r and merges them into the DB.
func (d *DB) Import(r io.Reader) error {
	var recs []model.PlayRecord
	if err := json.NewDecoder(r).Decode(&recs); err != nil {
		return err
	}
	for _, rec := range recs {
		if rec.LastPlayed.IsZero() {
			rec.LastPlayed = time.Now()
		}
		if rec.PlayCount <= 0 {
			rec.PlayCount = 1
		}
		if err := d.RecordPlay(rec); err != nil {
			return err
		}
	}
	return nil
}

// Join merges another SQLite taste database into this one.
func (d *DB) Join(otherPath string) error {
	other, err := Open(otherPath)
	if err != nil {
		return err
	}
	defer other.Close()
	recs, err := other.RecentPlays(100000)
	if err != nil {
		return err
	}
	for _, rec := range recs {
		if err := d.RecordPlay(rec); err != nil {
			return err
		}
	}
	return nil
}

// Discover returns recommendations from the taste DB based on emotion and mode.
func (d *DB) Discover(emotion model.Emotion, mode model.DiscoveryMode, limit int) ([]model.Recommendation, error) {
	var rows *sql.Rows
	var err error
	switch mode {
	case model.ModePioneer:
		rows, err = d.sql.Query(`
SELECT id, provider_id, provider_track_id, title, artist, album, duration_ms, emotion, weight, play_count, last_played
FROM plays
WHERE emotion = ? OR ? = 'unknown'
ORDER BY play_count ASC, RANDOM() LIMIT ?`, string(emotion), string(emotion), limit)
	default: // deep diver
		rows, err = d.sql.Query(`
SELECT id, provider_id, provider_track_id, title, artist, album, duration_ms, emotion, weight, play_count, last_played
FROM plays
WHERE emotion = ? OR ? = 'unknown'
ORDER BY play_count DESC, last_played DESC LIMIT ?`, string(emotion), string(emotion), limit)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	recs, err := scanRecords(rows)
	if err != nil {
		return nil, err
	}
	var out []model.Recommendation
	for _, r := range recs {
		out = append(out, model.Recommendation{
			Title:           r.Title,
			Artist:          r.Artist,
			Album:           r.Album,
			ProviderID:      r.ProviderID,
			ProviderTrackID: r.ProviderTrackID,
			Reason:          fmt.Sprintf("From your taste DB (%s, %s)", r.Emotion, mode),
		})
	}
	return out, nil
}

func scanRecords(rows *sql.Rows) ([]model.PlayRecord, error) {
	var recs []model.PlayRecord
	for rows.Next() {
		var r model.PlayRecord
		var em string
		if err := rows.Scan(&r.ID, &r.ProviderID, &r.ProviderTrackID, &r.Title, &r.Artist, &r.Album,
			&r.DurationMs, &em, &r.Weight, &r.PlayCount, &r.LastPlayed); err != nil {
			return nil, err
		}
		r.Emotion = model.Emotion(em)
		recs = append(recs, r)
	}
	return recs, rows.Err()
}
