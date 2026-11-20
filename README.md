# ApplyUni

AI-powered, subscription-based university application platform. Students (B2C) and agencies or schools (B2B) upload documents. AI checks them, and after one explicit approval per application, an agent submits to the university portal. The first market is Italy.

## Current MVP: public website

There is no sign-in yet. The website has:

- a landing page with student and agency waitlist sign-ups (`POST /api/waitlist`)
- a list of Italian programmes (`GET /api/catalog/programs`)
- a personal document checklist per programme, built from 4 questions without an account (`POST /api/catalog/programs/{id}/checklist`)

The account, upload, approval and submission API described below is built and tested in the backend. The frontend will use it once accounts are switched on.

## Stack

| Part | Choice |
|---|---|
| Backend | Go 1.27: chi, database/sql, Anthropic Go SDK |
| Frontend | React 18 (JavaScript) + Vite |
| Database | **SQLite** for development (one file, no server); **PostgreSQL 16** for production, adding row-level security per tenant. Chosen by `DATABASE_URL`. |
| Files | S3-compatible (MinIO) storing **ciphertext only** |
| Keys | OpenBao/Vault transit (envelope encryption) |
| Jobs | Queue table in the same database (`SKIP LOCKED` on PostgreSQL), so no separate queue server is needed |

Student personal data must be hosted **in Uzbekistan** (Law on Personal Data, art. 27¹). Run Postgres, MinIO and OpenBao on Uzbek infrastructure. The public university catalogue can live anywhere.

## How it works

```
Student/Agency ─► React ─► Go API ─┬─► SQLite / Postgres (every query scoped by org_id)
                                   ├─► Envelope encryption ─► Vault transit (KEK)
                                   │         └─► MinIO (ciphertext)
                                   └─► jobs ─► worker ─┬─► Claude: document check
                                                       └─► portal adapter: submit
```

1. **Profile** (school years, English test, document language, scholarship) drives the **requirements engine** (`internal/requirements`). It merges country rules with programme requirements into a checklist. For example, 11 years of school adds "proof of a completed university year".
2. **Upload**: each file gets a random AES-256-GCM key. That key is wrapped by Vault, and the ciphertext goes to MinIO. The document ID is bound as associated data (AAD), so a ciphertext can't be swapped onto another record.
3. **AI check** (`internal/ai`): Claude verifies the file matches the requested document, is legible and valid, and extracts its fields. Problems are shown to the student in plain language.
4. When every item is accepted, the application becomes **ready_for_review**.
5. **Approve**: the student ticks the confirmation and clicks approve. The server stores an approval with a SHA-256 snapshot of exactly what was approved (programme, profile, document hashes), plus IP and user agent.
6. **Submit**: the worker re-checks the document hashes against the snapshot and runs the portal adapter.

## Run locally

```bash
cd backend && cp .env.example .env         # set JWT_SECRET and LOCAL_KEK_BASE64 (openssl rand -base64 32)
# for the simplest setup also set KMS_PROVIDER=local and STORAGE_PROVIDER=local
set -a && . ./.env && set +a && go run ./cmd/api   # creates data/applyuni.db on first run

cd frontend && npm install && npm run dev  # http://localhost:5173 (proxies /api to :8080)
```

Tests: `cd backend && go test ./...` runs the whole core flow on a temporary SQLite file (sign-up, profile, application, encrypted uploads, approval, submission, tenant isolation, seat limits, deletion).

### Moving to PostgreSQL

1. Create an empty database, e.g. `createdb applyuni`.
2. Set `DATABASE_URL=postgres://user:pass@host:5432/applyuni?sslmode=disable`. Migrations in `internal/db/migrations/postgres` run on start.
3. Run the same tests against it: `TEST_POSTGRES_URL=postgres://… go test ./internal/service/` (use an empty database).
4. For Vault and MinIO as well: `docker compose up -d`, then set `KMS_PROVIDER=vault` and `STORAGE_PROVIDER=s3`.

Schema changes go in both `internal/db/migrations/sqlite` and `internal/db/migrations/postgres`. Data in the SQLite file is not migrated automatically.

Without `ANTHROPIC_API_KEY`, documents stay "Waiting for review". B2B staff can accept or reject them manually. `PORTAL_DRY_RUN=true` simulates submission. **Never enable it in production.**

## B2B

Registering as an agency or school creates an org with seats (5 during the trial). Staff (`owner`, `counselor`) add students, see all of them, and can accept or reject documents. Every query on tenant data filters by `org_id`, and the tests check that one tenant can't read, change or delete another's data. On PostgreSQL, row-level security enforces the same rule a second time inside the database.

## Not built yet

- **Portal adapters** (`internal/portal`). Each portal (Universitaly, Polimi, Unibo Studenti Online, Sapienza, DreamApply, …) needs a Playwright-based adapter built and tested against the live portal with a real test account. Until then, approved applications go to `needs_attention` with the approved package ready.
- **Inbound email** for the per-student alias, so the agent can read verification codes.
- **Catalogue pipeline**: crawl official calls and extract deadlines and fees with AI. Publish automatically only when two extractions agree; otherwise queue for the founder. Deadlines are `NULL` until verified, and the UI says so.
- Payments (Payme/Click/Stripe), Uzbek/Russian UI, parent view, essay feedback, password reset, refresh tokens in httpOnly cookies.
