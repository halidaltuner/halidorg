// Package settings reads and writes site_settings: small JSON values keyed
// by name that are not content, such as the homelab stats (Phase 4.3).
package settings

import (
	"context"
	"encoding/json"

	"halid.org/engine/internal/store"
)

// Get decodes the value of key into v; a missing key leaves v as it is.
func Get(ctx context.Context, db store.DB, key string, v any) error {
	res, err := db.Query(ctx, `SELECT value_json FROM site_settings WHERE key = ?`, key)
	if err != nil {
		return err
	}
	rows, err := store.Scan[struct {
		Value string `json:"value_json"`
	}](res)
	if err != nil || len(rows) == 0 {
		return err
	}
	return json.Unmarshal([]byte(rows[0].Value), v)
}

// Set stores v under key.
func Set(ctx context.Context, db store.DB, key string, v any) error {
	raw, err := json.Marshal(v)
	if err != nil {
		return err
	}
	_, err = db.Query(ctx, `INSERT INTO site_settings (key, value_json) VALUES (?, ?)
ON CONFLICT (key) DO UPDATE SET value_json = excluded.value_json`, key, string(raw))
	return err
}

// HomelabKey holds the homelab stat tiles that are typed, not computed.
const HomelabKey = "homelab_stats"

// Homelab is the typed part of the homelab stat tiles; builds is counted.
type Homelab struct {
	Nodes  string `json:"nodes"`
	Idle   string `json:"idle"`
	Uptime string `json:"uptime"`
}

// PhotosKey holds the photos page settings.
const PhotosKey = "photos"

// Photos is the 500px profile the photos page links to; empty hides the
// button.
type Photos struct {
	Profile string `json:"profile_500px"`
}
