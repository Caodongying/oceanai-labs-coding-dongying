# Multi-tenant certificate admin console

This file is the shared implementation contract for two independent Codex worktrees. Read it before changing code. One worktree owns the Go backend and persistence; the other owns the React frontend. Keep the API contract below identical in both. If an implementation detail must change, update this file first and coordinate the change before relying on it. Do not implement the AI-interaction-evidence requirements from Section 5 of the source assessment.

## Project overview and boundaries

Build an internal admin console in which a tenant admin reviews course completions, issues certificates to eligible learners, revokes certificates, and exports certificate data. Use Go with Gin, React (Vite is acceptable), SQLite, and Docker Compose for local deployment. A single command must build/start the complete app and initialize the default demo data. A separate command must run verification. The README must explain both commands and reproduce A1–A7 below.

There is no login UI or production authentication. The browser selects one seeded admin and sends `X-Admin-Email` on every protected request. The backend resolves the actor and tenant from its own database; client-supplied tenant IDs never grant access. Do not add cloud deployment, notifications, or unrelated features. Favor correct behavior over visual polish.

### Required default data

- Tenant IDs `east` and `west`, named East and West, with IANA time zones `Australia/Sydney` and `Australia/Perth`.
- Admins `alice@east.test` and `bob@west.test`, each bound to exactly one tenant.
- Both tenants have a `Safety 101` course and an `Alex Lee` learner. Give each tenant distinct course, learner, enrollment, and certificate IDs even when names match.
- Seed completed, incomplete, and already certified enrollments so bulk issuance produces both issued and skipped outcomes. Seed at least one certificate per tenant for list, detail, and export checks.
- Default startup stays small. Provide `./scripts/seed-large.sh --tenant east --count 100000` to generate **100,000 total certificates for that tenant** (idempotently, without changing the default quick-start dataset or adding rows on repeated invocation). Include duplicate values in sortable fields. Document any alternate command in the README if the script delegates to a backend command.

### Worktree ownership

| Worktree | Owns | Must not assume |
|---|---|---|
| Backend | `backend/`, schema/migrations, seed commands, API, backend tests, `compose.yaml`, `scripts/`, root README integration instructions | That a frontend response field or URL can be changed without updating this contract |
| Frontend | `frontend/`, UI tests, frontend-specific docs | Direct SQLite access, hardcoded tenant ownership, or undocumented API fields |

Both worktrees may read this file. The backend owns the final integrated Compose and README changes; the frontend should provide any frontend-specific instructions for integration. Keep all frontend requests relative to `/api/v1`; the Vite dev server should proxy that path to the backend, and the container setup should expose the same browser-facing path.

## Requirement specification

Scenario IDs refer to the acceptance scenarios in the assessment. `Core` means a required workflow behavior without a dedicated scenario row.

### Backend tasks

| Task description | Workflow | Scenario |
|---|---|---|
| Create schema and repeatable seed/migration commands for tenants, admins, courses, learners, enrollments, certificates, idempotency operations, and audit events. Enforce tenant and certificate uniqueness in the database. | All | A1–A7 |
| Resolve `X-Admin-Email` on each protected request; check the actor's tenant against every tenant path and query. Return a non-revealing 404 for other-tenant resources. Reject cross-tenant bulk issue before any write. | All | A1 |
| List certificates with case-insensitive partial search on learner name or serial number, page/limit, allowlisted sort, total count, and stable tie-breaking. Include local issue time/date and status. | Certificate list | A1, A5, A6 |
| List the actor's courses and paginated enrollments/completion states for review before issuance; return certificate detail with serial, course, learner, tenant-local issue time/date, and status. | Course completion review; bulk issue; certificate detail | A1, A4, A5 |
| Issue only for completed enrollments without an active or previously issued certificate for that enrollment; return an outcome for every enrollment in the course, with explicit skip reasons. Prevent duplicates using a database constraint/transaction. | Bulk issue | A2, A4 |
| Make bulk issue idempotent per tenant and key. Atomically bind the key to the canonical request payload; identical sequential/concurrent calls return the same completed result, and a changed payload returns 409 without writes. Commit each certificate with exactly one issuance audit event. | Bulk issue | A2, A3 |
| Revoke once, with one revocation audit event; repeated revocation returns an already-revoked result and creates no new event. | Certificate detail | A7 |
| Stream a CSV of all matching tenant certificates in the list's filter and order, using a deterministic secondary key. Keep SQLite configured for simultaneous readers (WAL and a busy timeout); a list request must succeed during a 100,000-row export. | CSV export | A1, A5, A6 |
| Provide the large-data generator and verification command; document A1–A7 reproduction in README. | All | A1–A7 |

### Frontend tasks

| Task description | Workflow | Scenario |
|---|---|---|
| Provide an actor selector populated from `/api/v1/demo/admins`; send the selected email in `X-Admin-Email`, fetch `/me`, and use only the returned tenant ID for tenant-scoped URLs. Reset tenant data when the actor changes. No login screen. | All | A1 |
| Show certificate list with learner/serial search, page controls, allowlisted sort controls, and loading, empty, and error states. Show the tenant-local issue date/time and status. | Certificate list | A1, A5, A6 |
| Provide a course picker with a paginated completion/enrollment view, then a bulk issue action. Generate one idempotency key per user-initiated operation, reuse it for transport retries, disable repeat submission while pending, and display issued/skipped/failed outcomes and operation status. A new intentional operation gets a new key. | Course completion review; bulk issue | A2, A3, A4 |
| Show certificate detail fields, require confirmation before revocation, and display the resulting status. Handle an already-revoked response without claiming another change occurred. | Certificate detail | A1, A5, A7 |
| Export through the protected API while preserving current search and sort; use a browser download mechanism that sends `X-Admin-Email` rather than an unprotected navigation link. Show progress/error state without blocking list use. | CSV export | A1, A5, A6 |
| Add focused UI checks for actor switching, list states, disabled bulk submission, outcome rendering, revoke confirmation, and export parameters. | All | A1–A7 |

## Shared API contract (v1)

All JSON requests and responses use `snake_case`. Base path: `/api/v1`. All protected endpoints require `X-Admin-Email: alice@east.test` or `bob@west.test`. Missing/unknown actor returns 401. A valid actor requesting another tenant's path or object gets 404, including cross-tenant course bulk issue. IDs are opaque strings. Unknown/invalid query parameters and invalid JSON return 400. Never trust a tenant ID in a request body. The backend must serve CORS or a same-origin proxy as needed for local UI use.

Errors have one consistent shape: `{ "error": { "code": "not_found", "message": "Resource not found" } }`. Stable codes: `bad_request`, `unauthorized`, `not_found`, `idempotency_conflict`, `internal_error`. Do not include another tenant's identifying details in errors.

### Discovery and course picker

| Method | Path (after `/api/v1`) | Query/body | `200` response | Other result |
|---|---|---|---|---|
| `GET` | `/demo/admins` | None; actor header not required | `items`: array of `{ email, tenant_id, tenant_name }` | — |
| `GET` | `/me` | Actor header | `{ email, tenant: { id, name, time_zone } }` | `401` for missing/unknown actor |
| `GET` | `/tenants/{tenant_id}/courses` | Actor header | `items`: array of `{ id, title }`, ordered by title then ID | `404` for other tenant |
| `GET` | `/tenants/{tenant_id}/courses/{course_id}/enrollments` | Actor header; `page`, `limit` | Paginated `items` of `Enrollment` | `404` for unknown/other-tenant course |

| Field | Enrollment response rule |
|---|---|
| `learner_id`, `learner_name` | Opaque learner ID and display name |
| `completion_status` | `completed` or `incomplete` |
| `certificate_status` | `none`, `issued`, or `revoked` |
| Pagination | `page`, `limit`, `total`, `total_pages`; defaults 1 and 20, maximum limit 100; zero pages when empty |
| Order | Learner name ascending, then learner ID ascending |

### Certificate list and detail

| Method | Path (after `/api/v1`) | Query | `200` response | Other result |
|---|---|---|---|---|
| `GET` | `/tenants/{tenant_id}/certificates` | `search`, `page`, `limit`, `sort_by`, `sort_dir` | `{ items: Certificate[], page, limit, total, total_pages, sort_by, sort_dir, search }` | `404` for other tenant |
| `GET` | `/tenants/{tenant_id}/certificates/{certificate_id}` | None | `Certificate` | `404` for missing/other-tenant certificate |

| Query field | Rule |
|---|---|
| `search` | Optional; trim spaces; case-insensitive substring of learner name or serial number; default empty string |
| `page`, `limit` | Positive integers; defaults 1 and 20; maximum limit 100 |
| `sort_by` | `issued_local_date` (default), `learner_name`, `course_title`, `serial_number`, or `status` |
| `sort_dir` | `asc` or `desc` (default) |
| Equal sort values | Always break ties by certificate ID ascending, regardless of `sort_dir` |
| Local-date sort | Compare tenant-local **calendar dates**, then apply the ID tie-breaker |
| Empty result | `items: []`, `total: 0`, `total_pages: 0` |

| `Certificate` field | Type / example |
|---|---|
| `id`, `serial_number` | Opaque string; e.g. `EAST-000001` for the serial |
| `learner` | `{ id: string, name: string }` |
| `course` | `{ id: string, title: string }` |
| `issued_at_utc` | RFC 3339 instant; e.g. `2026-01-10T13:30:00Z` |
| `issued_at_local` | RFC 3339 with tenant offset; East example `2026-01-11T00:30:00+11:00`; West example `2026-01-10T21:30:00+08:00` |
| `issued_local_date` | `YYYY-MM-DD` in tenant zone; East example `2026-01-11`; West example `2026-01-10` |
| `status` | `issued` or `revoked` |

The East and West examples use the same UTC instant. Persist the UTC instant and local date/time at issuance, or derive them consistently from the tenant IANA zone. Verify this case in a fixture or focused test.

### Bulk issue

| Method | Path (after `/api/v1`) | Headers and body | `200` response | Other result |
|---|---|---|---|---|
| `POST` | `/tenants/{tenant_id}/certificates/bulk-issue` | Actor header; `Idempotency-Key`; JSON `{ course_id, note? }` | `BulkIssueResult` | `404` for unknown/other-tenant course; `409` for key misuse |

| Request field | Rule |
|---|---|
| `Idempotency-Key` header | Required, nonempty, at most 128 characters; unique per tenant |
| `course_id` | Required opaque course ID |
| `note` | Optional string, at most 200 characters; may be recorded in audit metadata |
| JSON body | Only `course_id` and `note` are valid; `Content-Type: application/json` |

| `BulkIssueResult` field | Type / rule |
|---|---|
| `operation_id` | Opaque string; identical for every retry of one logical operation |
| `status` | Always `completed` in a successful response |
| `course` | `{ id: string, title: string }` |
| `summary` | `{ issued: number, skipped: number, failed: number }`; counts every outcome once |
| `outcomes` | Array of `LearnerOutcome`, ordered by learner ID ascending |

| `LearnerOutcome` field | Type / rule |
|---|---|
| `learner_id`, `learner_name` | Opaque ID and display name |
| `result` | `issued`, `skipped`, or `failed` |
| `reason` | `null` when issued; `incomplete` or `already_certified` when skipped; nonempty machine-readable string when failed |
| `certificate_id`, `serial_number` | Strings when issued; `null` otherwise |

The idempotency fingerprint includes the tenant and canonical validated body (`course_id` and normalized `note`). A key reused with a changed course or note returns `409 idempotency_conflict` without writes. Identical sequential or concurrent calls return the same **stored** completed result; concurrent callers wait rather than receive a partial result. Store the final response durably so retries survive restarts. A transaction and database uniqueness constraint must prevent duplicate issuance even when distinct keys race. Each actual issuance writes exactly one `issued` audit event in the same transaction.

### Revocation

| Method | Path (after `/api/v1`) | Headers and body | `200` response | Other result |
|---|---|---|---|---|
| `POST` | `/tenants/{tenant_id}/certificates/{certificate_id}/revoke` | Actor header; JSON `{ reason? }` | `{ certificate: Certificate, result: "revoked" }` for first change; `result: "already_revoked"` for repeats | `404` for missing/other-tenant certificate |

| Rule | Value |
|---|---|
| `reason` | Optional string, at most 200 characters; `{}` is valid |
| Audit | Only the first state change writes one `revoked` event; use a transaction/conditional update for concurrent calls |

### CSV export

| Method | Path (after `/api/v1`) | Query | `200` response | Other result |
|---|---|---|---|---|
| `GET` | `/tenants/{tenant_id}/certificates/export` | `search`, `sort_by`, `sort_dir`; same meaning/defaults as list; no pagination | CSV download of all matching rows | `404` for other tenant |

| CSV contract | Rule |
|---|---|
| Content type | `text/csv; charset=utf-8` |
| Download header | `Content-Disposition: attachment; filename="certificates-{tenant_id}.csv"` |
| Header row | Exactly `learner_name,course_title,serial_number,issued_local_date` |
| Data rows | CSV-escaped values; local date in `YYYY-MM-DD`; same filter/order/tie-breaker as list |
| Scale and concurrency | 100,000 rows within 60 seconds; repeated exports have identical order; list succeeds during export |
| Side effects | No audit events |

## Database design (SQLite)

The backend worktree owns migrations and may add implementation-only columns or indexes, but it must preserve these fields, relationships, and constraints. Use `TEXT` opaque IDs for application entities; tenant IDs are the fixed strings `east` and `west`. Store timestamps as RFC 3339 text in UTC unless a field explicitly says local. Enable SQLite foreign keys, WAL mode, and a busy timeout on every connection.

### Core models

| Table | Required columns | Relationships and constraints |
|---|---|---|
| `tenants` | `id` primary key, `name`, `time_zone` | `time_zone` is an IANA zone, e.g. `Australia/Sydney`. |
| `admins` | `id` primary key, `tenant_id`, `email` | `tenant_id` references `tenants`; `email` is globally unique. The actor header resolves to this row and its tenant. |
| `courses` | `id` primary key, `tenant_id`, `title` | `tenant_id` references `tenants`. Titles may match across tenants. |
| `learners` | `id` primary key, `tenant_id`, `name` | `tenant_id` references `tenants`. Names are not unique. |
| `enrollments` | `id` primary key, `tenant_id`, `course_id`, `learner_id`, `enrolled_at_utc`, nullable `completed_at_utc` | Course and learner must belong to the same tenant. `UNIQUE(tenant_id, course_id, learner_id)`. Non-null `completed_at_utc` means completed. |
| `certificates` | `id` primary key, `tenant_id`, `enrollment_id`, `serial_number`, `issued_at_utc`, `issued_at_local`, `issued_local_date`, `status`, nullable `revoked_at_utc`, nullable `revoked_by_admin_id` | Enrollment and revoking admin, if present, must belong to the same tenant. `UNIQUE(enrollment_id)` and `UNIQUE(serial_number)`. `status` is `issued` or `revoked`; revocation fields are set only for revoked rows. |

`issued_at_local` is an RFC 3339 timestamp with the tenant's offset at issuance; `issued_local_date` is its `YYYY-MM-DD` calendar date. Calculate both from `issued_at_utc` and the tenant's IANA zone when inserting. Store or derive them consistently for every read, sort, and export. A revoked certificate still occupies its enrollment, so it cannot be reissued.

### Idempotency and audit models

| Table | Required columns | Relationships and constraints |
|---|---|---|
| `bulk_issue_operations` | `id` primary key, `tenant_id`, `actor_admin_id`, `idempotency_key`, `request_hash`, `request_body_json`, `response_json`, `completed_at_utc` | Actor belongs to tenant. `UNIQUE(tenant_id, idempotency_key)`. Store the canonical validated body and exact completed API response; never overwrite either after commit. |
| `audit_events` | `id` primary key, `tenant_id`, `certificate_id`, `actor_admin_id`, `action`, `occurred_at_utc`, nullable `bulk_operation_id`, nullable `metadata_json` | Referenced certificate, actor, and operation belong to tenant. `action` is `issued` or `revoked`. `UNIQUE(certificate_id, action)` permits exactly one event of each type per certificate. `metadata_json` may contain the bulk note or revocation reason. |

Do not add a separate bulk-outcomes table unless implementation needs one. The immutable `response_json` stores every learner outcome and is returned verbatim on identical retries. Seeded and generated certificates must also have an `issued` audit event.

### Required transaction and query behavior

| Concern | Database rule |
|---|---|
| Tenant isolation | Include `tenant_id` in all resource queries and joins. Use composite tenant-aware foreign keys, with supporting `UNIQUE(tenant_id, id)` keys, or equivalent transactional checks to prevent cross-tenant references. A guessed ID must never bypass the tenant filter. |
| Bulk eligibility | Select enrollments in the requested tenant/course with non-null `completed_at_utc` and no certificate for that enrollment. Return incomplete and already certified learners as skipped outcomes. |
| Same-key retries | Canonicalize `course_id` and normalized `note`, then hash the validated body with tenant ID. In one transaction, bind the key, issue certificates, write issuance audits, and store the final response. Commit only a completed operation. A concurrent same-key caller waits for the first transaction, compares hashes, and reads its stored response; a different hash returns 409 without writes. |
| Distinct-key races | `UNIQUE(enrollment_id)` prevents duplicate certificates. Recheck eligibility within the write transaction; a loser is skipped as already certified. Each inserted certificate and its `issued` audit event commit together. |
| Revocation | Conditionally change `status` from `issued` to `revoked` and insert the `revoked` audit event in one transaction. A repeat, including a concurrent repeat, reads the revoked row and adds no event. |
| Export | Use a read-only query/cursor and do not hold a write transaction while streaming. Filter by tenant and search; order by requested sort field and certificate ID ascending for ties. A simultaneous list read must succeed. |

Create indexes for `enrollments(tenant_id, course_id, completed_at_utc)`, certificate lookup by tenant/enrollment and tenant/local date, and `audit_events(tenant_id, certificate_id)`. Add indexes for the other supported sorts where measurement shows they are needed for the 100,000-row export. Case-insensitive substring search may require a scan; it must still meet A6's time limit. Keep the default seed small and make the large-data generator idempotent.

## Start and verification commands

Implement these repository-root commands and document them in README:

```sh
./scripts/dev.sh                         # install/use declared dependencies, migrate/seed, and start full app via Docker Compose
./scripts/test.sh                        # start required services, run backend/frontend checks and API smoke checks, exit nonzero on failure
./scripts/seed-large.sh --tenant east --count 100000
```

`dev.sh` should be safe to rerun without duplicating seed data. The README should state the UI URL and API URL/port, how to stop the Compose stack, and the prerequisite Docker Compose command. The backend and frontend may have additional direct development commands, but the three commands above are the integration interface.

## Acceptance testing: reproduce A1–A7

Automate the checks where practical and put runnable steps with expected results in README. Use `alice@east.test` and `bob@west.test` as actor headers. The backend worktree owns API/invariant tests; the frontend worktree owns UI behavior tests. Run the integrated command after merging worktrees.

| ID | Setup and action | Required assertion |
|---|---|---|
| A1 tenant isolation | As Bob, request East list, detail, bulk issue with an East course, and export, using `/tenants/east/...`; also try an East object ID under `/tenants/west/...`. | 404/non-revealing responses, no East data, and no certificates or audit events from rejected bulk issue. Bob's West requests work. |
| A2 retry safety | As Alice, POST the same bulk body/key sequentially and with two concurrent clients. Inspect certificates/audit rows or dedicated test fixtures. | All responses have the same operation ID, summary, and outcomes; each eligible enrollment has at most one certificate and exactly one issuance audit event. |
| A3 key misuse | Reuse Alice's A2 key with another valid course ID or a changed `note`. | 409; original stored result remains unchanged; no second operation, certificate, or audit event. |
| A4 learner outcomes | Bulk issue a seeded course containing completed, incomplete, and already certified enrollments. | Only completed, uncertified enrollments are issued; skipped outcomes use the two specified reasons; summary matches outcomes. |
| A5 DST/local date | Create or use fixtures for the same `2026-01-10T13:30:00Z` instant in East and West; list, detail, sort by `issued_local_date`, and export. | East shows `2026-01-11 00:30` / CSV `2026-01-11`; West shows `2026-01-10 21:30` / CSV `2026-01-10`; sort follows those local dates. |
| A6 export growth | Run the 100,000-row generator for East. Search and sort on a field with duplicate values. Start export, request the list before it finishes, then repeat export. Time both. | Each export finishes within 60 seconds, has exactly the matching East rows and stable identical order, and the simultaneous list succeeds. Compare parsed CSV rows, not just file size. |
| A7 revocation audit | As Alice, revoke one issued certificate twice, then inspect status and audit rows. | First response `revoked` with one new audit event; second `already_revoked` with no additional event. |

For UI verification, confirm the actor switch clears prior tenant data, list loading/empty/error states render, the bulk button cannot submit twice while pending, the per-learner result is visible, revocation asks for confirmation, and export retains current search/sort while leaving the list usable. The README should include the exact API requests or test commands needed to reproduce each scenario, including the independent test data setup for cases that mutate state.
