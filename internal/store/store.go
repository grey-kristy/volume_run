package store

import (
	"database/sql"
	"fmt"
	"time"

	_ "github.com/mattn/go-sqlite3"

	"volume_run/internal/model"
)

type Store struct {
	db *sql.DB
}

type WeekRow struct {
	Week  string  `json:"week"`
	Km    float64 `json:"km"`
	Hours float64 `json:"hours"`
}

type ActivityRow struct {
	ID         string  `json:"id"`
	Name       string  `json:"name"`
	TypeKey    string  `json:"type_key"`   // raw Garmin sport, e.g. "trail_running"
	StartedAt  string  `json:"started_at"` // RFC3339 UTC
	DistanceKm float64 `json:"km"`
	DurationS  int     `json:"duration_s"`
	AverageHR  int     `json:"avg_hr"`
	MaxHR      int     `json:"max_hr"`
}

func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite3", path+"?_journal_mode=WAL")
	if err != nil {
		return nil, err
	}
	if err := migrate(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func migrate(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS activities (
			id          TEXT PRIMARY KEY,
			name        TEXT NOT NULL DEFAULT '',
			sport       TEXT NOT NULL,
			type_key    TEXT NOT NULL DEFAULT '',
			started_at  TEXT NOT NULL,
			week        TEXT NOT NULL,
			distance_km REAL NOT NULL,
			duration_s  INTEGER NOT NULL,
			avg_hr      INTEGER NOT NULL DEFAULT 0,
			max_hr      INTEGER NOT NULL DEFAULT 0
		);
		CREATE INDEX IF NOT EXISTS idx_week_sport ON activities(week, sport);
	`)
	return err
}

func (s *Store) Upsert(activities []model.Activity) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`
		INSERT OR REPLACE INTO activities (id, name, sport, type_key, started_at, week, distance_km, duration_s, avg_hr, max_hr)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, a := range activities {
		_, err := stmt.Exec(a.ID, a.Name, a.Sport, a.TypeKey, a.StartedAt.UTC().Format(time.RFC3339), a.Week, a.DistanceKm, a.DurationS, a.AverageHR, a.MaxHR)
		if err != nil {
			return fmt.Errorf("upsert %s: %w", a.ID, err)
		}
	}
	return tx.Commit()
}

// MaxStartedAt returns the most recent activity start time, or zero time if the table is empty.
func (s *Store) MaxStartedAt() (time.Time, error) {
	var raw sql.NullString
	err := s.db.QueryRow(`SELECT MAX(started_at) FROM activities`).Scan(&raw)
	if err != nil || !raw.Valid {
		return time.Time{}, err
	}
	t, err := time.Parse(time.RFC3339, raw.String)
	return t, err
}

// WeeklySummary returns km and hours grouped by ISO week, newest first.
// sport must be "all", "running", or "cycling".
func (s *Store) WeeklySummary(sport string) ([]WeekRow, error) {
	var (
		rows *sql.Rows
		err  error
	)
	if sport == "" || sport == "all" {
		rows, err = s.db.Query(`
			SELECT week,
			       ROUND(SUM(distance_km), 2),
			       ROUND(SUM(duration_s) / 3600.0, 2)
			FROM   activities
			GROUP  BY week
			ORDER  BY week DESC
		`)
	} else {
		rows, err = s.db.Query(`
			SELECT week,
			       ROUND(SUM(distance_km), 2),
			       ROUND(SUM(duration_s) / 3600.0, 2)
			FROM   activities
			WHERE  sport = ?
			GROUP  BY week
			ORDER  BY week DESC
		`, sport)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []WeekRow
	for rows.Next() {
		var r WeekRow
		if err := rows.Scan(&r.Week, &r.Km, &r.Hours); err != nil {
			return nil, err
		}
		result = append(result, r)
	}
	return result, rows.Err()
}

// WeekActivities returns individual activities for a given ISO week, ordered by start time.
func (s *Store) WeekActivities(week, sport string) ([]ActivityRow, error) {
	var (
		rows *sql.Rows
		err  error
	)
	if sport == "" || sport == "all" {
		rows, err = s.db.Query(`
			SELECT id, name, type_key, started_at, distance_km, duration_s, avg_hr, max_hr
			FROM   activities
			WHERE  week = ?
			ORDER  BY started_at
		`, week)
	} else {
		rows, err = s.db.Query(`
			SELECT id, name, type_key, started_at, distance_km, duration_s, avg_hr, max_hr
			FROM   activities
			WHERE  week = ? AND sport = ?
			ORDER  BY started_at
		`, week, sport)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []ActivityRow
	for rows.Next() {
		var r ActivityRow
		if err := rows.Scan(&r.ID, &r.Name, &r.TypeKey, &r.StartedAt, &r.DistanceKm, &r.DurationS, &r.AverageHR, &r.MaxHR); err != nil {
			return nil, err
		}
		result = append(result, r)
	}
	return result, rows.Err()
}
