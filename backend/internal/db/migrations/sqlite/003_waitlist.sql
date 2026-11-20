-- Public website: visitors leave an email to be notified when applying opens.
CREATE TABLE waitlist (
  id          TEXT PRIMARY KEY,
  email       TEXT NOT NULL UNIQUE,   -- stored lowercased
  kind        TEXT NOT NULL DEFAULT 'student' CHECK (kind IN ('student', 'agency')),
  program_id  TEXT REFERENCES programs(id) ON DELETE SET NULL,
  created_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
