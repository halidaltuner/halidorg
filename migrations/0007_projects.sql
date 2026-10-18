-- Phase 4.1: projects (software and homelab/DIY share one model and one
-- detail template) and their build logs (CONTENT_MODEL.md §2). The project
-- body is entries + revisions; the rows below are its fact strip, stack or
-- parts aside, hero links and gallery. Each revision also keeps a copy of
-- those rows in data_json, so restores bring them back.

CREATE TABLE projects (
  entry_id     TEXT PRIMARY KEY REFERENCES entries (id) ON DELETE CASCADE,
  project_type TEXT NOT NULL CHECK (project_type IN ('software', 'homelab')),
  -- statuses from the prototypes (Projects, Homelab, Project Detail)
  -- named apart from entries.status, the publish state
  project_status TEXT NOT NULL DEFAULT 'active'
               CHECK (project_status IN ('active', 'maintained', 'running', 'in_progress', 'paused', 'done', 'idea', 'archived')),
  category_id  TEXT REFERENCES categories (id) ON DELETE SET NULL, -- a homelab category; required for homelab (Go)
  tagline      TEXT NOT NULL DEFAULT '',
  kind_line    TEXT NOT NULL DEFAULT '', -- the small line over the title; empty: derived from stack or category
  github_repo  TEXT,                     -- owner/name, for the GitHub sync (Phase 4.4)
  started_at   INTEGER,
  ended_at     INTEGER,
  featured     INTEGER NOT NULL DEFAULT 0 CHECK (featured IN (0, 1)),
  cost_total   INTEGER,                  -- minor units (kuruş, cents); empty: the sum of the parts
  currency     TEXT NOT NULL DEFAULT 'TRY' CHECK (length(currency) = 3),
  time_spent   TEXT NOT NULL DEFAULT ''  -- free text: "6 weekends", "ongoing"
);

CREATE INDEX projects_type ON projects (project_type, project_status);

CREATE TABLE project_facts (
  project_id TEXT NOT NULL REFERENCES projects (entry_id) ON DELETE CASCADE,
  key        TEXT NOT NULL CHECK (length(key) BETWEEN 1 AND 40),
  value      TEXT NOT NULL DEFAULT '',
  sort       INTEGER NOT NULL DEFAULT 0
);

CREATE INDEX project_facts_project ON project_facts (project_id, sort);

-- "stack" aside, software only (Go)
CREATE TABLE project_stack (
  project_id TEXT NOT NULL REFERENCES projects (entry_id) ON DELETE CASCADE,
  name       TEXT NOT NULL CHECK (length(name) BETWEEN 1 AND 60),
  sort       INTEGER NOT NULL DEFAULT 0
);

CREATE INDEX project_stack_project ON project_stack (project_id, sort);

-- "parts" aside, homelab only (Go)
CREATE TABLE project_parts (
  project_id TEXT NOT NULL REFERENCES projects (entry_id) ON DELETE CASCADE,
  name       TEXT NOT NULL CHECK (length(name) BETWEEN 1 AND 120),
  qty        INTEGER NOT NULL DEFAULT 1 CHECK (qty >= 1),
  price      INTEGER,                    -- minor units, for the whole line
  currency   TEXT NOT NULL DEFAULT 'TRY' CHECK (length(currency) = 3),
  url        TEXT CHECK (url IS NULL OR url GLOB 'https://*'),
  sort       INTEGER NOT NULL DEFAULT 0
);

CREATE INDEX project_parts_project ON project_parts (project_id, sort);

-- Hero buttons: the first is the primary one.
CREATE TABLE project_links (
  project_id TEXT NOT NULL REFERENCES projects (entry_id) ON DELETE CASCADE,
  label      TEXT NOT NULL CHECK (length(label) BETWEEN 1 AND 40),
  url        TEXT NOT NULL CHECK (url GLOB 'https://*' OR url GLOB '/*' OR url GLOB '#*'),
  sort       INTEGER NOT NULL DEFAULT 0
);

CREATE INDEX project_links_project ON project_links (project_id, sort);

CREATE TABLE project_gallery (
  project_id TEXT NOT NULL REFERENCES projects (entry_id) ON DELETE CASCADE,
  media_id   TEXT NOT NULL REFERENCES media (id),
  sort       INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY (project_id, media_id)
);

CREATE INDEX project_gallery_media ON project_gallery (media_id);

-- buildlog: entries.title and the body are the log entry; it belongs to one
-- project and lives under it (/{lang}/projects/{project}/log/{slug}).
CREATE TABLE buildlogs (
  entry_id         TEXT PRIMARY KEY REFERENCES entries (id) ON DELETE CASCADE,
  project_entry_id TEXT NOT NULL REFERENCES projects (entry_id) ON DELETE CASCADE,
  log_date         INTEGER NOT NULL,
  hours_spent      REAL CHECK (hours_spent IS NULL OR hours_spent >= 0)
);

CREATE INDEX buildlogs_project ON buildlogs (project_entry_id, log_date DESC);

-- Homelab categories with the tones of design/Homelab.dc.html.
INSERT OR IGNORE INTO categories (id, kind, slug, name_tr, name_en, tone, sort) VALUES
  ('01K0000000000000000000HM01', 'homelab', 'homelab', 'homelab', 'homelab', 'orange', 10),
  ('01K0000000000000000000HM02', 'homelab', 'electronics', 'elektronik', 'electronics', 'violet', 20),
  ('01K0000000000000000000HM03', 'homelab', '3d-print', '3d baskı', '3d print', 'cyan', 30),
  ('01K0000000000000000000HM04', 'homelab', 'woodwork', 'ahşap', 'woodwork', 'amber', 40),
  ('01K0000000000000000000HM05', 'homelab', 'network', 'ağ', 'network', 'green', 50);
