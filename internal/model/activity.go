package model

import "time"

type Activity struct {
	ID         string
	Sport      string    // "running" | "cycling" | "other"
	StartedAt  time.Time // local time as returned by Garmin
	Week       string    // ISO week label, e.g. "2026-W45"
	DistanceKm float64
	DurationS  int
}
