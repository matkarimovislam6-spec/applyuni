-- Full cycle: document verdicts, portal account state, inbound email,
-- university decisions and a per-application timeline.

ALTER TABLE documents DROP CONSTRAINT IF EXISTS documents_status_check;
ALTER TABLE documents ADD CONSTRAINT documents_status_check
  CHECK (status IN ('uploaded', 'analyzing', 'accepted', 'rejected', 'needs_review', 'superseded'));
ALTER TABLE documents ADD COLUMN verdict jsonb;
ALTER TABLE documents ADD COLUMN pages integer;

ALTER TABLE portal_accounts ADD COLUMN status text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'active', 'failed'));
ALTER TABLE portal_accounts ADD COLUMN verified_at timestamptz;
ALTER TABLE portal_accounts ADD COLUMN last_error text;

ALTER TABLE applications ADD COLUMN decision text CHECK (decision IN ('admitted', 'not_admitted', 'waitlisted'));
ALTER TABLE applications ADD COLUMN decision_at timestamptz;

CREATE TABLE inbox_messages (
  id              uuid PRIMARY KEY,
  org_id          uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  student_id      uuid NOT NULL REFERENCES students(id) ON DELETE CASCADE,
  application_id  uuid REFERENCES applications(id) ON DELETE SET NULL,
  to_addr         text NOT NULL,
  from_addr       text NOT NULL,
  subject         text NOT NULL DEFAULT '',
  body_ct         bytea NOT NULL,
  nonce           bytea NOT NULL,
  wrapped_dek     bytea NOT NULL,
  key_ref         text NOT NULL,
  kind            text NOT NULL DEFAULT 'other',
  received_at     timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX inbox_to ON inbox_messages (to_addr, received_at);

CREATE TABLE application_events (
  id              bigserial PRIMARY KEY,
  org_id          uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  application_id  uuid NOT NULL REFERENCES applications(id) ON DELETE CASCADE,
  kind            text NOT NULL,
  note            text,
  at              timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX application_events_app ON application_events (application_id, id);

DO $$
DECLARE t text;
BEGIN
  FOREACH t IN ARRAY ARRAY['inbox_messages','application_events'] LOOP
    EXECUTE format('ALTER TABLE %I ENABLE ROW LEVEL SECURITY', t);
    EXECUTE format('ALTER TABLE %I FORCE ROW LEVEL SECURITY', t);
    EXECUTE format($p$CREATE POLICY tenant_isolation ON %I
                     USING (org_id = current_setting('app.org_id', true)::uuid)
                     WITH CHECK (org_id = current_setting('app.org_id', true)::uuid)$p$, t);
  END LOOP;
END$$;
GRANT SELECT, INSERT, UPDATE, DELETE ON inbox_messages, application_events TO applyuni_app;
GRANT USAGE ON ALL SEQUENCES IN SCHEMA public TO applyuni_app;
