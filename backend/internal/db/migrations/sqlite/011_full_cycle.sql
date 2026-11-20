-- Full cycle: document verdicts, portal account state, inbound email,
-- university decisions and a per-application timeline.

-- Documents gain 'needs_review' (a person looks before we decide) and
-- 'superseded' (replaced by a newer upload of the same type). SQLite can't
-- alter a CHECK, so the table is rebuilt; nothing references documents.
CREATE TABLE documents_new (
  id            TEXT PRIMARY KEY,
  org_id        TEXT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  student_id    TEXT NOT NULL REFERENCES students(id) ON DELETE CASCADE,
  doc_type      TEXT NOT NULL,
  filename      TEXT NOT NULL,
  content_type  TEXT NOT NULL,
  size_bytes    INTEGER NOT NULL,
  sha256        BLOB NOT NULL,
  object_key    TEXT NOT NULL,
  nonce         BLOB NOT NULL,
  wrapped_dek   BLOB NOT NULL,
  key_ref       TEXT NOT NULL,
  status        TEXT NOT NULL DEFAULT 'uploaded' CHECK (status IN ('uploaded', 'analyzing', 'accepted', 'rejected', 'needs_review', 'superseded')),
  ai_result     TEXT,
  verdict       TEXT,
  pages         INTEGER,
  created_at    TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
INSERT INTO documents_new (id, org_id, student_id, doc_type, filename, content_type, size_bytes, sha256, object_key, nonce, wrapped_dek, key_ref, status, ai_result, created_at)
  SELECT id, org_id, student_id, doc_type, filename, content_type, size_bytes, sha256, object_key, nonce, wrapped_dek, key_ref, status, ai_result, created_at FROM documents;
DROP TABLE documents;
ALTER TABLE documents_new RENAME TO documents;
CREATE INDEX documents_student ON documents (student_id);

ALTER TABLE portal_accounts ADD COLUMN status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'active', 'failed'));
ALTER TABLE portal_accounts ADD COLUMN verified_at TIMESTAMP;
ALTER TABLE portal_accounts ADD COLUMN last_error TEXT;

-- The university's answer, separate from our process status.
ALTER TABLE applications ADD COLUMN decision TEXT CHECK (decision IN ('admitted', 'not_admitted', 'waitlisted'));
ALTER TABLE applications ADD COLUMN decision_at TIMESTAMP;

-- Emails universities send to a student's platform address. Bodies are
-- envelope-encrypted like documents.
CREATE TABLE inbox_messages (
  id              TEXT PRIMARY KEY,
  org_id          TEXT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  student_id      TEXT NOT NULL REFERENCES students(id) ON DELETE CASCADE,
  application_id  TEXT REFERENCES applications(id) ON DELETE SET NULL,
  to_addr         TEXT NOT NULL,
  from_addr       TEXT NOT NULL,
  subject         TEXT NOT NULL DEFAULT '',
  body_ct         BLOB NOT NULL,
  nonce           BLOB NOT NULL,
  wrapped_dek     BLOB NOT NULL,
  key_ref         TEXT NOT NULL,
  kind            TEXT NOT NULL DEFAULT 'other',
  received_at     TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX inbox_to ON inbox_messages (to_addr, received_at);

CREATE TABLE application_events (
  id              INTEGER PRIMARY KEY AUTOINCREMENT,
  org_id          TEXT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  application_id  TEXT NOT NULL REFERENCES applications(id) ON DELETE CASCADE,
  kind            TEXT NOT NULL,
  note            TEXT,
  at              TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX application_events_app ON application_events (application_id, id);
