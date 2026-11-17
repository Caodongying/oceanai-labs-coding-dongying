# Multi-tenant certificate admin console

This repository contains the Go/Gin and SQLite backend. The React frontend is maintained in a separate worktree; the startup script includes it when `frontend/package.json` is present. Demo access uses `X-Admin-Email` rather than a login screen.

## Commands

Docker with `docker compose` is required.

```sh
./scripts/dev.sh
./scripts/test.sh
./scripts/seed-large.sh --tenant east --count 100000
```

`dev.sh` builds and starts the stack, migrates the database, and seeds small default data. It is safe to rerun. The API is at <http://localhost:8080/api/v1>; when the frontend is present, the UI is at <http://localhost:3000>. If port 8080 is occupied, set `API_PORT=18080` for `dev.sh` and `test.sh`, and use that port in API requests. Stop with `docker compose --profile ui down`. Add `-v` to that command only to delete the database volume. `test.sh` starts the stack, runs backend tests and vet in Docker, runs frontend checks when present, and makes API smoke requests. The large seed script invokes the backend's `seed-large` command and fills the selected tenant to **100,000 total certificates**, without adding rows on repeated runs.

For direct backend development: `cd backend && go run ./cmd/server`. Set `DB_PATH` to choose the SQLite file.

## Demo API

The public `GET /api/v1/demo/admins` returns the two actors. Protected requests need `X-Admin-Email: alice@east.test` or `bob@west.test`. Tenant IDs in paths are checked against the actor's database record.

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

Run mutating checks against a fresh database if you need the initial issued/skipped counts. Reset with `docker compose --profile ui down -v && ./scripts/dev.sh`.

**A1 — isolation.** Bob's West list succeeds. Every East request below returns 404, as does an East ID under a West path. The rejected bulk request makes no write.

```sh
curl -i -H 'X-Admin-Email: bob@west.test' "$API/tenants/west/certificates"
curl -i -H 'X-Admin-Email: bob@west.test' "$API/tenants/east/certificates"
curl -i -H 'X-Admin-Email: bob@west.test' "$API/tenants/east/certificates/cert-east-alex"
curl -i -H 'X-Admin-Email: bob@west.test' "$API/tenants/west/certificates/cert-east-alex"
curl -i -H 'X-Admin-Email: bob@west.test' "$API/tenants/east/certificates/export"
curl -i -H 'X-Admin-Email: bob@west.test' -H 'Idempotency-Key: a1' -H 'Content-Type: application/json' -d '{"course_id":"course-east-safety"}' "$API/tenants/east/certificates/bulk-issue"
```

**A2–A4 — retries, key misuse, learner outcomes.** On fresh seed data, the first request issues Blair and Drew, skips Alex as `already_certified`, and skips Casey as `incomplete`. The second returns the same stored operation ID and outcomes. Concurrent calls with one new key also return identical results; a changed note with an old key returns 409.

```sh
curl -H 'X-Admin-Email: alice@east.test' -H 'Idempotency-Key: a2' -H 'Content-Type: application/json' -d '{"course_id":"course-east-safety"}' "$API/tenants/east/certificates/bulk-issue"
curl -H 'X-Admin-Email: alice@east.test' -H 'Idempotency-Key: a2' -H 'Content-Type: application/json' -d '{"course_id":"course-east-safety"}' "$API/tenants/east/certificates/bulk-issue"
for i in 1 2; do curl -sS -H 'X-Admin-Email: alice@east.test' -H 'Idempotency-Key: a2-concurrent' -H 'Content-Type: application/json' -d '{"course_id":"course-east-safety"}' "$API/tenants/east/certificates/bulk-issue" & done; wait
curl -i -H 'X-Admin-Email: alice@east.test' -H 'Idempotency-Key: a2' -H 'Content-Type: application/json' -d '{"course_id":"course-east-safety","note":"changed"}' "$API/tenants/east/certificates/bulk-issue"
```

**A5 — local dates.** Detail and export show January 11 for East and January 10 for West. List ordering with `sort_by=issued_local_date` uses those dates.

```sh
curl -H 'X-Admin-Email: alice@east.test' "$API/tenants/east/certificates/cert-east-alex"
curl -H 'X-Admin-Email: bob@west.test' "$API/tenants/west/certificates/cert-west-alex"
curl -H 'X-Admin-Email: alice@east.test' "$API/tenants/east/certificates/export?sort_by=issued_local_date"
curl -H 'X-Admin-Email: bob@west.test' "$API/tenants/west/certificates/export?sort_by=issued_local_date"
```

**A6 — large export.** Run the large seed command above. The two exports should contain identical parsed rows in the same order, finish within 60 seconds each, and the list request should succeed during export. Generated learner names and issue dates contain duplicates.

```sh
time curl -sS -H 'X-Admin-Email: alice@east.test' "$API/tenants/east/certificates/export?search=Demo%20Learner%20001&sort_by=learner_name&sort_dir=asc" -o /tmp/east-1.csv &
curl -H 'X-Admin-Email: alice@east.test' "$API/tenants/east/certificates?search=Demo%20Learner%20001&sort_by=learner_name&sort_dir=asc"
wait
time curl -sS -H 'X-Admin-Email: alice@east.test' "$API/tenants/east/certificates/export?search=Demo%20Learner%20001&sort_by=learner_name&sort_dir=asc" -o /tmp/east-2.csv
python3 -c "import csv; a=list(csv.reader(open('/tmp/east-1.csv'))); b=list(csv.reader(open('/tmp/east-2.csv'))); assert a==b and len(a)==401; print('400 matching rows in stable order')"
```

**A7 — revoke once.** On a fresh database, the first response is `revoked`, the second `already_revoked`, and detail stays revoked. The database permits only one `revoked` audit event per certificate.

```sh
curl -H 'X-Admin-Email: alice@east.test' -H 'Content-Type: application/json' -d '{}' "$API/tenants/east/certificates/cert-east-alex/revoke"
curl -H 'X-Admin-Email: alice@east.test' -H 'Content-Type: application/json' -d '{}' "$API/tenants/east/certificates/cert-east-alex/revoke"
curl -H 'X-Admin-Email: alice@east.test' "$API/tenants/east/certificates/cert-east-alex"
```
