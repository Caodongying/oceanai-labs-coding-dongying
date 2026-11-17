# Multi-tenant certificate admin console

This repository contains the Go/Gin and SQLite backend and the React frontend. Demo access uses `X-Admin-Email` rather than a login screen.

## Commands

Docker with `docker compose` is required.

```sh
./scripts/dev.sh
./scripts/test.sh
./scripts/seed-large.sh --tenant east --count 100000
```

`dev.sh` builds and starts the stack, migrates the database, and seeds small default data. It is safe to rerun. The API is at <http://localhost:8080/api/v1>; when the frontend is present, the UI is at <http://localhost:3000>. If port 8080 is occupied, set `API_PORT=18080` for `dev.sh` and `test.sh`, and use that port in API requests. Stop with `docker compose --profile ui down`. Add `-v` to that command only to delete the database volume. `test.sh` starts the stack, runs backend tests and vet in Docker, runs frontend checks when present, and makes API smoke requests. The large seed script invokes the backend's `seed-large` command and fills the selected tenant to **100,000 total certificates**, without adding rows on repeated runs.

For direct backend development: `cd backend && go run ./cmd/server`. Set `DB_PATH` to choose the SQLite file.

## Remaining risks and known defects

No known functional defects are currently documented; the integrated verification command passes with `API_PORT=18080` on this machine. This is a local demo: `X-Admin-Email` is a selectable actor header, not production authentication. Anyone who can reach the API can impersonate either seeded admin, so do not expose this stack to untrusted users. The frontend install currently reports five dependency advisories (two moderate, one high, two critical); review them before using these dependencies outside the demo. The 60-second export target depends on the machine running Docker; use the A6 check below to measure it locally.

## Demo API

The public `GET /api/v1/demo/admins` returns the two actors. Protected requests need `X-Admin-Email: alice@east.test` or `bob@west.test`; there are no bearer tokens. Tenant IDs in paths are checked against the actor's database record.

```sh
API=http://localhost:${API_PORT:-8080}/api/v1
curl "$API/demo/admins"
curl -H 'X-Admin-Email: alice@east.test' "$API/me"
curl -H 'X-Admin-Email: alice@east.test' "$API/tenants/east/courses"
curl -H 'X-Admin-Email: alice@east.test' "$API/tenants/east/courses/course-east-safety/enrollments"
curl -H 'X-Admin-Email: alice@east.test' "$API/tenants/east/certificates?search=Alex&sort_by=issued_local_date&sort_dir=desc"
```

Each tenant has its own `Safety 101` course and `Alex Lee` learner. The seeded certificates share `2026-01-10T13:30:00Z`. East shows `2026-01-11T00:30:00+11:00`; West shows `2026-01-10T21:30:00+08:00`.

## Manual A1–A7 checks

Set `API=http://localhost:${API_PORT:-8080}/api/v1` as shown above. Each scenario below can use an independent fresh database: run `docker compose --profile ui down -v && ./scripts/dev.sh` before that scenario. **This deletes the demo database volume.** The focused Go tests named below create their own temporary databases and do not need a reset. Run them from the repository root with `docker build --target test -f backend/Dockerfile backend`, or run an individual test with `cd backend && go test ./internal/app -run '^TestName$' -count=1` when Go is installed locally.

**A1 — isolation.** On a fresh database, Bob's West list succeeds. Every East request below returns 404 with `not_found`, as does an East ID under a West path. The rejected bulk request makes no write. `TestIsolationAndLocalDates` also checks that certificate, audit, and operation counts stay unchanged.

```sh
curl -i -H 'X-Admin-Email: bob@west.test' "$API/tenants/west/certificates"
curl -i -H 'X-Admin-Email: bob@west.test' "$API/tenants/east/certificates"
curl -i -H 'X-Admin-Email: bob@west.test' "$API/tenants/east/certificates/cert-east-alex"
curl -i -H 'X-Admin-Email: bob@west.test' "$API/tenants/west/certificates/cert-east-alex"
curl -i -H 'X-Admin-Email: bob@west.test' "$API/tenants/east/certificates/export"
curl -i -H 'X-Admin-Email: bob@west.test' -H 'Idempotency-Key: a1' -H 'Content-Type: application/json' -d '{"course_id":"course-east-safety"}' "$API/tenants/east/certificates/bulk-issue"
```

**A2 — retry safety.** On fresh seed data, send the same body and key twice. Save the responses and compare them: the operation ID, summary, and outcomes must match exactly. The first call issues Blair and Drew. Run the concurrent example only after another fresh-database reset so both clients race to issue eligible learners. `TestBulkIdempotencyAndRevocation` checks equal concurrent responses, one certificate per enrollment, and one issuance audit per certificate; `TestConcurrentDistinctKeys` checks the distinct-key race.

```sh
curl -fsS -H 'X-Admin-Email: alice@east.test' -H 'Idempotency-Key: a2' -H 'Content-Type: application/json' -d '{"course_id":"course-east-safety"}' "$API/tenants/east/certificates/bulk-issue" -o /tmp/a2-first.json
curl -fsS -H 'X-Admin-Email: alice@east.test' -H 'Idempotency-Key: a2' -H 'Content-Type: application/json' -d '{"course_id":"course-east-safety"}' "$API/tenants/east/certificates/bulk-issue" -o /tmp/a2-retry.json
cmp /tmp/a2-first.json /tmp/a2-retry.json
# Reset the demo database before the concurrent issuance run:
docker compose --profile ui down -v && ./scripts/dev.sh
curl -fsS -H 'X-Admin-Email: alice@east.test' -H 'Idempotency-Key: a2-concurrent' -H 'Content-Type: application/json' -d '{"course_id":"course-east-safety"}' "$API/tenants/east/certificates/bulk-issue" -o /tmp/a2-concurrent-1.json &
curl -fsS -H 'X-Admin-Email: alice@east.test' -H 'Idempotency-Key: a2-concurrent' -H 'Content-Type: application/json' -d '{"course_id":"course-east-safety"}' "$API/tenants/east/certificates/bulk-issue" -o /tmp/a2-concurrent-2.json &
wait
cmp /tmp/a2-concurrent-1.json /tmp/a2-concurrent-2.json
```

**A3 — key misuse.** On a fresh database, bind a key to the first body, then reuse it with a changed note. Expect HTTP 409 and `idempotency_conflict`. Retrying the original body must still return the stored first response. `TestBulkIdempotencyAndRevocation` checks that the conflict adds no operation, certificate, or audit event.

```sh
curl -fsS -H 'X-Admin-Email: alice@east.test' -H 'Idempotency-Key: a3' -H 'Content-Type: application/json' -d '{"course_id":"course-east-safety"}' "$API/tenants/east/certificates/bulk-issue" -o /tmp/a3-first.json
curl -i -H 'X-Admin-Email: alice@east.test' -H 'Idempotency-Key: a3' -H 'Content-Type: application/json' -d '{"course_id":"course-east-safety","note":"changed"}' "$API/tenants/east/certificates/bulk-issue"
curl -fsS -H 'X-Admin-Email: alice@east.test' -H 'Idempotency-Key: a3' -H 'Content-Type: application/json' -d '{"course_id":"course-east-safety"}' "$API/tenants/east/certificates/bulk-issue" -o /tmp/a3-retry.json
cmp /tmp/a3-first.json /tmp/a3-retry.json
```

**A4 — learner outcomes.** On a fresh database, review the enrollment states, then issue the course. Expect `issued: 2`, `skipped: 2`, `failed: 0`; Blair and Drew are issued, Alex is skipped with `already_certified`, and Casey with `incomplete`. The four outcomes must be ordered by learner ID. `TestBulkIdempotencyAndRevocation` checks the summary and skip reasons.

```sh
curl -fsS -H 'X-Admin-Email: alice@east.test' "$API/tenants/east/courses/course-east-safety/enrollments"
curl -fsS -H 'X-Admin-Email: alice@east.test' -H 'Idempotency-Key: a4' -H 'Content-Type: application/json' -d '{"course_id":"course-east-safety"}' "$API/tenants/east/certificates/bulk-issue"
```

**A5 — local dates.** The two seeded certificates share `2026-01-10T13:30:00Z`. Detail and sorted list show `2026-01-11T00:30:00+11:00` for East and `2026-01-10T21:30:00+08:00` for West. CSV uses `2026-01-11` and `2026-01-10`, respectively. `TestIsolationAndLocalDates` checks detail, list, and parsed CSV values.

```sh
curl -H 'X-Admin-Email: alice@east.test' "$API/tenants/east/certificates/cert-east-alex"
curl -H 'X-Admin-Email: bob@west.test' "$API/tenants/west/certificates/cert-west-alex"
curl -H 'X-Admin-Email: alice@east.test' "$API/tenants/east/certificates?sort_by=issued_local_date&sort_dir=asc"
curl -H 'X-Admin-Email: bob@west.test' "$API/tenants/west/certificates?sort_by=issued_local_date&sort_dir=asc"
curl -H 'X-Admin-Email: alice@east.test' "$API/tenants/east/certificates/export?sort_by=issued_local_date"
curl -H 'X-Admin-Email: bob@west.test' "$API/tenants/west/certificates/export?sort_by=issued_local_date"
```

**A6 — large export.** From a fresh database, run `./scripts/seed-large.sh --tenant east --count 100000` twice; East must still have 100,000 **total** certificates. Generated learner names and issue dates contain duplicates. Run the filtered exports below; each should finish within 60 seconds, contain the same 400 matching rows in the same order, and the list should succeed during the first export. Then run a full export and check that it has 100,000 data rows. `TestLargeSeedExportOrderAndConcurrentList` checks idempotent seeding, parsed CSV equality, list concurrency, audits, and the full export time.

```sh
./scripts/seed-large.sh --tenant east --count 100000
./scripts/seed-large.sh --tenant east --count 100000
time curl -sS -H 'X-Admin-Email: alice@east.test' "$API/tenants/east/certificates/export?search=Demo%20Learner%20001&sort_by=learner_name&sort_dir=asc" -o /tmp/east-1.csv &
curl -H 'X-Admin-Email: alice@east.test' "$API/tenants/east/certificates?search=Demo%20Learner%20001&sort_by=learner_name&sort_dir=asc"
wait
time curl -sS -H 'X-Admin-Email: alice@east.test' "$API/tenants/east/certificates/export?search=Demo%20Learner%20001&sort_by=learner_name&sort_dir=asc" -o /tmp/east-2.csv
python3 -c "import csv; a=list(csv.reader(open('/tmp/east-1.csv'))); b=list(csv.reader(open('/tmp/east-2.csv'))); assert a==b and len(a)==401; print('400 matching rows in stable order')"
time curl -fsS -H 'X-Admin-Email: alice@east.test' "$API/tenants/east/certificates/export?sort_by=issued_local_date" -o /tmp/east-all.csv
python3 -c "import csv; rows=csv.reader(open('/tmp/east-all.csv')); assert sum(1 for _ in rows)-1==100000; print('100000 East certificates')"
```

**A7 — revoke once.** On a fresh database, the first response is `revoked`, the second `already_revoked`, and detail stays revoked. `TestBulkIdempotencyAndRevocation` checks that exactly one `revoked` audit event exists for this certificate.

```sh
curl -H 'X-Admin-Email: alice@east.test' -H 'Content-Type: application/json' -d '{}' "$API/tenants/east/certificates/cert-east-alex/revoke"
curl -H 'X-Admin-Email: alice@east.test' -H 'Content-Type: application/json' -d '{}' "$API/tenants/east/certificates/cert-east-alex/revoke"
curl -H 'X-Admin-Email: alice@east.test' "$API/tenants/east/certificates/cert-east-alex"
```

## UI checks

Run `./scripts/test.sh` for the frontend test suite. In the UI at <http://localhost:3000>, switch from Alice to Bob and confirm East data clears; check list loading, empty search, and API error states; open a course and confirm a pending bulk request disables repeat submission and shows per-learner outcomes; confirm revocation before submitting and check the repeat result; export with an active search and sort and verify the list remains usable while the download runs.
