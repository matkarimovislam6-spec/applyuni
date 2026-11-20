# ApplyUni

AI-powered, subscription-based university application platform. Students (B2C) and agencies or schools (B2B) upload documents. AI checks them, and after one explicit approval per application, an agent submits to the university portal. The first market is Italy.

## Stack

| Part | Choice |
|---|---|
| Backend | Go 1.27: chi, pgx, Anthropic Go SDK |
| Frontend | React 18 (JavaScript) + Vite |
| Database | PostgreSQL 16 with row-level security per tenant |
| Files | S3-compatible (MinIO) storing **ciphertext only** |
| Keys | OpenBao/Vault transit (envelope encryption) |
| Jobs | Postgres queue (`FOR UPDATE SKIP LOCKED`), so no separate queue server is needed |

Student personal data must be hosted **in Uzbekistan** (Law on Personal Data, art. 27¹). Run Postgres, MinIO and OpenBao on Uzbek infrastructure. The public university catalogue can live anywhere.

## How it works

```
Student/Agency ─► React ─► Go API ─┬─► Postgres (RLS: org_id = app.org_id)
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
docker compose up -d                       # postgres, minio, openbao (+ transit key)
cd backend && cp .env.example .env         # set JWT_SECRET; ANTHROPIC_API_KEY optional
set -a && . ./.env && set +a && go run ./cmd/api

cd frontend && npm install && npm run dev  # http://localhost:5173 (proxies /api to :8080)
```

Tests: `cd backend && go test ./...`

Without `ANTHROPIC_API_KEY`, documents stay "Waiting for review". B2B staff can accept or reject them manually. `PORTAL_DRY_RUN=true` simulates submission. **Never enable it in production.**

## B2B

Registering as an agency or school creates an org with seats (5 during the trial). Staff (`owner`, `counselor`) add students, see all of them, and can accept or reject documents. Postgres RLS guarantees no tenant can read another's rows, even if application code has a bug.

## Not built yet

- **Portal adapters** (`internal/portal`). Each portal (Universitaly, Polimi, Unibo Studenti Online, Sapienza, DreamApply, …) needs a Playwright-based adapter built and tested against the live portal with a real test account. Until then, approved applications go to `needs_attention` with the approved package ready.
- **Inbound email** for the per-student alias, so the agent can read verification codes.
- **Catalogue pipeline**: crawl official calls and extract deadlines and fees with AI. Publish automatically only when two extractions agree; otherwise queue for the founder. Deadlines are `NULL` until verified, and the UI says so.
- Payments (Payme/Click/Stripe), Uzbek/Russian UI, parent view, essay feedback, password reset, refresh tokens in httpOnly cookies.
