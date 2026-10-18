-- Phase 4.2: a stack row's role, shown on the right of the "stack" aside
-- ("Go 1.23 · server", design/Project Detail.dc.html). Optional.
ALTER TABLE project_stack ADD COLUMN role TEXT NOT NULL DEFAULT '';
