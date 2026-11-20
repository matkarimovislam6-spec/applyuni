-- ApplyUni core schema.
-- Tenancy: every row that belongs to a customer carries org_id and is protected
-- by row-level security. B2C students get a personal org; B2B agencies/schools
-- get one org with many students. The API runs tenant queries as role
-- applyuni_app with app.org_id set, so the database itself enforces isolation.

CREATE EXTENSION IF NOT EXISTS pgcrypto;

DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'applyuni_app') THEN
    CREATE ROLE applyuni_app NOLOGIN;
  END IF;
END$$;

-- ---------- Accounts & tenancy ----------

CREATE TABLE organizations (
  id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  name        text NOT NULL,
  kind        text NOT NULL CHECK (kind IN ('personal', 'agency', 'school')),
  plan        text NOT NULL DEFAULT 'free',
  seat_limit  int  NOT NULL DEFAULT 1,
  created_at  timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE users (
  id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  email          text NOT NULL,
  password_hash  text NOT NULL,
  full_name      text NOT NULL,
  created_at     timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX users_email_lower ON users (lower(email));

CREATE TABLE memberships (
  user_id  uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  org_id   uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  role     text NOT NULL CHECK (role IN ('owner', 'counselor', 'student', 'parent')),
  PRIMARY KEY (user_id, org_id)
);

-- ---------- Catalogue (public, not personal data) ----------

CREATE TABLE countries (
  code        char(2) PRIMARY KEY,
  name        text NOT NULL,
  rules       jsonb NOT NULL,          -- document rules, see internal/requirements
  source_url  text,
  verified_at date
);

CREATE TABLE universities (
  id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  country_code  char(2) NOT NULL REFERENCES countries(code),
  name          text NOT NULL,
  city          text,
  portal        text NOT NULL,         -- portal adapter id, e.g. 'universitaly', 'polimi', 'dreamapply'
  website       text,
  UNIQUE (country_code, name)
);

CREATE TABLE programs (
  id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  university_id       uuid NOT NULL REFERENCES universities(id) ON DELETE CASCADE,
  name                text NOT NULL,
  degree              text NOT NULL CHECK (degree IN ('bachelor', 'master')),
  language            text NOT NULL DEFAULT 'en',
  tuition_eur_year    int,
  application_fee_eur int,
  deadline            date,            -- NULL = not verified yet; never guess
  intake              text,
  requirements        jsonb NOT NULL DEFAULT '{}',
  source_url          text,
  verified_at         date,
  UNIQUE (university_id, name, degree)
);

-- ---------- Tenant data (RLS) ----------

CREATE TABLE students (
  id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  org_id       uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  user_id      uuid REFERENCES users(id) ON DELETE SET NULL,
  full_name    text NOT NULL,
  email        text,
  email_alias  text,                   -- platform inbox the agent reads for portal verification codes
  profile      jsonb NOT NULL DEFAULT '{}',
  created_at   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX students_org ON students (org_id);

CREATE TABLE applications (
  id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  org_id        uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  student_id    uuid NOT NULL REFERENCES students(id) ON DELETE CASCADE,
  program_id    uuid NOT NULL REFERENCES programs(id),
  status        text NOT NULL DEFAULT 'collecting_documents' CHECK (status IN (
                  'collecting_documents', 'ready_for_review', 'approved',
                  'submitting', 'submitted', 'needs_attention', 'withdrawn')),
  status_note   text,
  portal_ref    text,
  submitted_at  timestamptz,
  created_at    timestamptz NOT NULL DEFAULT now(),
  updated_at    timestamptz NOT NULL DEFAULT now(),
  UNIQUE (student_id, program_id)
);
CREATE INDEX applications_org ON applications (org_id);

-- Files are envelope-encrypted: each file has its own AES-256-GCM data key,
-- stored only in wrapped (encrypted) form. The object store holds ciphertext.
CREATE TABLE documents (
  id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  org_id        uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  student_id    uuid NOT NULL REFERENCES students(id) ON DELETE CASCADE,
  doc_type      text NOT NULL,
  filename      text NOT NULL,
  content_type  text NOT NULL,
  size_bytes    bigint NOT NULL,
  sha256        bytea NOT NULL,        -- of plaintext, for dedupe/integrity
  object_key    text NOT NULL,
  nonce         bytea NOT NULL,
  wrapped_dek   bytea NOT NULL,
  key_ref       text NOT NULL,
  status        text NOT NULL DEFAULT 'uploaded' CHECK (status IN ('uploaded', 'analyzing', 'accepted', 'rejected')),
  ai_result     jsonb,
  created_at    timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX documents_student ON documents (student_id);

-- Explicit, per-application consent before the agent submits anything.
CREATE TABLE approvals (
  id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  org_id           uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  application_id   uuid NOT NULL REFERENCES applications(id) ON DELETE CASCADE,
  user_id          uuid NOT NULL REFERENCES users(id),
  snapshot_sha256  bytea NOT NULL,     -- hash of exactly what was approved
  snapshot         jsonb NOT NULL,
  ip               text,
  user_agent       text,
  approved_at      timestamptz NOT NULL DEFAULT now()
);

-- Portal logins the agent creates for the student; secret is envelope-encrypted.
CREATE TABLE portal_accounts (
  id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  org_id       uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  student_id   uuid NOT NULL REFERENCES students(id) ON DELETE CASCADE,
  portal       text NOT NULL,
  username     text NOT NULL,
  secret_ct    bytea NOT NULL,
  nonce        bytea NOT NULL,
  wrapped_dek  bytea NOT NULL,
  key_ref      text NOT NULL,
  created_at   timestamptz NOT NULL DEFAULT now(),
  UNIQUE (student_id, portal)
);

-- Background work (document analysis, portal submission). Processed with
-- SELECT ... FOR UPDATE SKIP LOCKED, so no separate queue server is needed.
CREATE TABLE jobs (
  id          bigserial PRIMARY KEY,
  org_id      uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  kind        text NOT NULL,
  payload     jsonb NOT NULL,
  status      text NOT NULL DEFAULT 'queued' CHECK (status IN ('queued', 'running', 'done', 'failed')),
  attempts    int NOT NULL DEFAULT 0,
  run_after   timestamptz NOT NULL DEFAULT now(),
  last_error  text,
  created_at  timestamptz NOT NULL DEFAULT now(),
  updated_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX jobs_ready ON jobs (run_after) WHERE status = 'queued';

CREATE TABLE audit_log (
  id         bigserial PRIMARY KEY,
  org_id     uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  user_id    uuid,
  action     text NOT NULL,
  target     text,
  at         timestamptz NOT NULL DEFAULT now()
);

-- ---------- Row-level security ----------

DO $$
DECLARE t text;
BEGIN
  FOREACH t IN ARRAY ARRAY['students','applications','documents','approvals','portal_accounts','jobs','audit_log'] LOOP
    EXECUTE format('ALTER TABLE %I ENABLE ROW LEVEL SECURITY', t);
    EXECUTE format('ALTER TABLE %I FORCE ROW LEVEL SECURITY', t);
    EXECUTE format($p$CREATE POLICY tenant_isolation ON %I
                     USING (org_id = current_setting('app.org_id', true)::uuid)
                     WITH CHECK (org_id = current_setting('app.org_id', true)::uuid)$p$, t);
  END LOOP;
END$$;

GRANT USAGE ON SCHEMA public TO applyuni_app;
GRANT SELECT ON countries, universities, programs, organizations TO applyuni_app;
GRANT SELECT (id, full_name) ON users TO applyuni_app;
GRANT SELECT, INSERT, UPDATE, DELETE ON students, applications, documents, approvals, portal_accounts, jobs, audit_log TO applyuni_app;
GRANT USAGE ON ALL SEQUENCES IN SCHEMA public TO applyuni_app;
