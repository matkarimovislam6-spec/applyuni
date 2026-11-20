-- Public website MVP: visitors leave an email to be notified when applying opens.
CREATE TABLE waitlist (
  id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  email       text NOT NULL,
  kind        text NOT NULL DEFAULT 'student' CHECK (kind IN ('student', 'agency')),
  program_id  uuid REFERENCES programs(id) ON DELETE SET NULL,
  created_at  timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX waitlist_email_lower ON waitlist (lower(email));
