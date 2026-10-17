-- Phase 0 core tables (CONTENT_MODEL.md). Content tables arrive in Phase 1.

CREATE TABLE site_settings (
  key        TEXT PRIMARY KEY,
  value_json TEXT NOT NULL CHECK (json_valid(value_json))
);

-- id is the hex HMAC-SHA256(SESSION_SECRET, cookie token), never the token
-- itself, so a leaked table cannot be replayed as cookies (ADR-005).
CREATE TABLE admin_sessions (
  id             TEXT PRIMARY KEY,
  email          TEXT NOT NULL,
  created_at     INTEGER NOT NULL,
  expires_at     INTEGER NOT NULL,
  last_reauth_at INTEGER NOT NULL,
  ip             TEXT,
  ua             TEXT
);

CREATE INDEX admin_sessions_expires_at ON admin_sessions (expires_at);
