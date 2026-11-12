// Package garmin implements a client for the Garmin Connect Personal API.
//
// Authentication uses a static API key to obtain a short-lived bearer token.
// Set GARMIN_PERSONAL_API_KEY in the environment (or pass it via --api-key).
//
// API reference: https://github.com/garmin/connect-personal-api-python
// Base URL: https://personal.connectapi.garmin.com
package garmin

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"run_vol/internal/model"
)

const (
	baseURL        = "https://personal.connectapi.garmin.com"
	tokenPath      = "/personalapitoken-service/personal-api/personal-api-key/token"
	activitiesPath = "/activitylist-service/activities/search/activities"
	pageSize       = 100
	tokenMinTTL    = time.Hour // reuse cached token if more than 1h remains
)

// Client holds the API key and a cached bearer token.
type Client struct {
	apiKey    string
	tokenFile string
	token     *bearerToken
}

type bearerToken struct {
	Fingerprint string  `json:"fingerprint"`
	AccessToken string  `json:"access_token"`
	ExpiresAt   float64 `json:"expires_at"` // Unix epoch seconds
}

func (t *bearerToken) valid() bool {
	if t == nil || t.AccessToken == "" {
		return false
	}
	return time.Until(time.Unix(int64(t.ExpiresAt), 0)) > tokenMinTTL
}

// apiActivity matches one element of the activities search response array.
type apiActivity struct {
	ActivityID     int64   `json:"activityId"`
	StartTimeLocal string  `json:"startTimeLocal"` // "2006-01-02 15:04:05"
	ActivityType   struct {
		TypeKey string `json:"typeKey"` // e.g. "running", "road_biking"
	} `json:"activityType"`
	Distance float64 `json:"distance"` // metres
	Duration float64 `json:"duration"` // seconds
}

// NewClient creates a client using GARMIN_PERSONAL_API_KEY from the environment.
// tokenFile is a local path used to cache the bearer token between restarts.
func NewClient(tokenFile string) (*Client, error) {
	apiKey := os.Getenv("GARMIN_PERSONAL_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("GARMIN_PERSONAL_API_KEY must be set")
	}
	c := &Client{apiKey: apiKey, tokenFile: tokenFile}
	if err := c.loadToken(); err != nil {
		log.Printf("garmin: no cached token (%v), will fetch on first sync", err)
	}
	return c, nil
}

// ListActivities returns all activities that started strictly after `after`.
// Pass a zero time to fetch the full history.
func (c *Client) ListActivities(after time.Time) ([]model.Activity, error) {
	if err := c.ensureToken(); err != nil {
		return nil, err
	}

	var all []model.Activity
	start := 0

	for {
		page, done, err := c.fetchPage(start, after)
		if err != nil {
			return nil, err
		}
		all = append(all, page...)
		if done {
			break
		}
		start += pageSize
		time.Sleep(300 * time.Millisecond)
	}

	return all, nil
}

// fetchPage fetches one page of activities. done is true when the page was
// short (last page) or when we encountered activities older than `after`.
func (c *Client) fetchPage(start int, after time.Time) ([]model.Activity, bool, error) {
	params := url.Values{
		"start": {strconv.Itoa(start)},
		"limit": {strconv.Itoa(pageSize)},
	}

	req, err := http.NewRequest(http.MethodGet, baseURL+activitiesPath+"?"+params.Encode(), nil)
	if err != nil {
		return nil, false, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token.AccessToken)
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, false, fmt.Errorf("activities request: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return nil, false, fmt.Errorf("activities API returned %d: %s", resp.StatusCode, body)
	}

	var result []apiActivity
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, false, fmt.Errorf("decode activities: %w\nbody: %s", err, body)
	}

	done := len(result) < pageSize

	var activities []model.Activity
	for _, a := range result {
		t, err := time.ParseInLocation("2006-01-02 15:04:05", a.StartTimeLocal, time.UTC)
		if err != nil {
			log.Printf("garmin: skip activity %d: bad time %q: %v", a.ActivityID, a.StartTimeLocal, err)
			continue
		}
		if !after.IsZero() && !t.After(after) {
			done = true
			continue
		}
		activities = append(activities, convertActivity(a, t))
	}

	return activities, done, nil
}

// ensureToken fetches a fresh bearer token if the cached one is missing or expiring soon.
func (c *Client) ensureToken() error {
	fp := fingerprint(c.apiKey)
	if c.token != nil && c.token.Fingerprint == fp && c.token.valid() {
		return nil
	}

	req, err := http.NewRequest(http.MethodPost, baseURL+tokenPath, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "ApiKey "+c.apiKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("token request: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("token endpoint returned %d: %s", resp.StatusCode, body)
	}

	accessToken := strings.TrimSpace(string(body))
	expiry, err := jwtExpiry(accessToken)
	if err != nil {
		return fmt.Errorf("parse JWT expiry: %w", err)
	}

	c.token = &bearerToken{
		Fingerprint: fp,
		AccessToken: accessToken,
		ExpiresAt:   float64(expiry.Unix()),
	}
	return c.saveToken()
}

func convertActivity(a apiActivity, t time.Time) model.Activity {
	year, week := t.ISOWeek()
	return model.Activity{
		ID:         strconv.FormatInt(a.ActivityID, 10),
		Sport:      normaliseSport(a.ActivityType.TypeKey),
		StartedAt:  t,
		Week:       fmt.Sprintf("%04d-W%02d", year, week),
		DistanceKm: a.Distance / 1000,
		DurationS:  int(a.Duration),
	}
}

func normaliseSport(typeKey string) string {
	switch typeKey {
	case "running", "trail_running", "treadmill_running", "track_running":
		return "running"
	case "cycling", "road_biking", "mountain_biking", "indoor_cycling", "virtual_ride":
		return "cycling"
	default:
		return "other"
	}
}

// fingerprint is a SHA-256 hash of the API key, used to detect key rotation in the cache.
func fingerprint(apiKey string) string {
	h := sha256.Sum256([]byte(baseURL + apiKey))
	return fmt.Sprintf("%x", h)
}

// jwtExpiry decodes the JWT payload (without verifying the signature) and returns exp.
func jwtExpiry(token string) (time.Time, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return time.Time{}, fmt.Errorf("not a valid JWT")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return time.Time{}, fmt.Errorf("decode payload: %w", err)
	}
	var claims struct {
		Exp int64 `json:"exp"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return time.Time{}, fmt.Errorf("unmarshal claims: %w", err)
	}
	if claims.Exp == 0 {
		return time.Time{}, fmt.Errorf("no exp claim in token")
	}
	return time.Unix(claims.Exp, 0), nil
}

func (c *Client) loadToken() error {
	data, err := os.ReadFile(c.tokenFile)
	if err != nil {
		return err
	}
	var t bearerToken
	if err := json.Unmarshal(data, &t); err != nil {
		return err
	}
	c.token = &t
	return nil
}

func (c *Client) saveToken() error {
	data, err := json.MarshalIndent(c.token, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(c.tokenFile, data, 0600)
}
