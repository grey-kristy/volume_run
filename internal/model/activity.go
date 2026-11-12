package model

import "time"

type Activity struct {
	ID         string
	Name       string
	Sport      string    // normalised family: "running" | "cycling" | "other"
	TypeKey    string    // raw Garmin typeKey, e.g. "trail_running", "road_biking"
	StartedAt  time.Time // local time as returned by Garmin
	Week       string    // ISO week label, e.g. "2026-W45"
	DistanceKm float64
	DurationS  int
}
