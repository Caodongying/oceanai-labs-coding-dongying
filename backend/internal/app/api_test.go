package app

import (
	"bytes"
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

type fixture struct {
	db     *sql.DB
	server *httptest.Server
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err = Migrate(db); err != nil {
		t.Fatal(err)
	}
	if err = Seed(db); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(Router(db))
	t.Cleanup(func() { server.Close(); db.Close() })
	return fixture{db, server}
}

func (f fixture) request(t *testing.T, method, path, actor, key, body string) (int, []byte) {
	t.Helper()
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	req, err := http.NewRequest(method, f.server.URL+"/api/v1"+path, reader)
	if err != nil {
		t.Fatal(err)
	}
	if actor != "" {
		req.Header.Set("X-Admin-Email", actor)
	}
	if key != "" {
		req.Header.Set("Idempotency-Key", key)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := f.server.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return resp.StatusCode, b
}

func wantStatus(t *testing.T, got, want int, body []byte) {
	t.Helper()
	if got != want {
		t.Fatalf("status %d, want %d: %s", got, want, body)
	}
}
func count(t *testing.T, db *sql.DB, q string, args ...any) int {
	t.Helper()
	var n int
	if err := db.QueryRow(q, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestIsolationAndLocalDates(t *testing.T) {
	f := newFixture(t)
	baseCert := count(t, f.db, `SELECT COUNT(*) FROM certificates`)
	baseAudit := count(t, f.db, `SELECT COUNT(*) FROM audit_events`)
	for _, path := range []string{
		"/tenants/east/certificates",
		"/tenants/east/certificates/cert-east-alex",
		"/tenants/west/certificates/cert-east-alex",
		"/tenants/east/certificates/export",
	} {
		status, b := f.request(t, "GET", path, "bob@west.test", "", "")
		wantStatus(t, status, 404, b)
		if !bytes.Contains(b, []byte(`"code":"not_found"`)) {
			t.Fatalf("unexpected error: %s", b)
		}
	}
	status, b := f.request(t, "POST", "/tenants/east/certificates/bulk-issue", "bob@west.test", "cross", `{"course_id":"course-east-safety"}`)
	wantStatus(t, status, 404, b)
	if n := count(t, f.db, `SELECT COUNT(*) FROM certificates`); n != baseCert {
		t.Fatalf("cross tenant write: %d certificates", n)
	}
	if n := count(t, f.db, `SELECT COUNT(*) FROM audit_events`); n != baseAudit {
		t.Fatalf("cross tenant write: %d audits", n)
	}
	if n := count(t, f.db, `SELECT COUNT(*) FROM bulk_issue_operations`); n != 0 {
		t.Fatalf("cross tenant operation: %d", n)
	}

	for _, tc := range []struct{ actor, tenant, id, local, date string }{
		{"alice@east.test", "east", "cert-east-alex", "2026-01-11T00:30:00+11:00", "2026-01-11"},
		{"bob@west.test", "west", "cert-west-alex", "2026-01-10T21:30:00+08:00", "2026-01-10"},
	} {
		path := "/tenants/" + tc.tenant + "/certificates"
		status, b = f.request(t, "GET", path+"/"+tc.id, tc.actor, "", "")
		wantStatus(t, status, 200, b)
		var cert certificate
		if err := json.Unmarshal(b, &cert); err != nil {
			t.Fatal(err)
		}
		if cert.IssuedAtLocal != tc.local || cert.IssuedLocalDate != tc.date {
			t.Fatalf("local date mismatch: %+v", cert)
		}
		status, b = f.request(t, "GET", path+"?sort_by=issued_local_date", tc.actor, "", "")
		wantStatus(t, status, 200, b)
		if !bytes.Contains(b, []byte(tc.date)) {
			t.Fatalf("list omitted local date: %s", b)
		}
		status, b = f.request(t, "GET", path+"/export", tc.actor, "", "")
		wantStatus(t, status, 200, b)
		rows, err := csv.NewReader(bytes.NewReader(b)).ReadAll()
		if err != nil {
			t.Fatal(err)
		}
		if len(rows) != 2 || rows[1][3] != tc.date {
			t.Fatalf("CSV date mismatch: %v", rows)
		}
	}
	status, b = f.request(t, "GET", "/tenants/west/certificates", "bob@west.test", "", "")
	wantStatus(t, status, 200, b)
}

func TestBulkIdempotencyAndRevocation(t *testing.T) {
	f := newFixture(t)
	path := "/tenants/east/certificates/bulk-issue"
	body := `{"course_id":"course-east-safety"}`
	const actor = "alice@east.test"
	status, first := f.request(t, "POST", path, actor, "sequential", body)
	wantStatus(t, status, 200, first)
	status, again := f.request(t, "POST", path, actor, "sequential", body)
	wantStatus(t, status, 200, again)
	if !bytes.Equal(first, again) {
		t.Fatalf("sequential retry changed result")
	}
	var result bulkResult
	if err := json.Unmarshal(first, &result); err != nil {
		t.Fatal(err)
	}
	if result.Summary.Issued != 2 || result.Summary.Skipped != 2 || result.Summary.Failed != 0 || len(result.Outcomes) != 4 {
		t.Fatalf("unexpected summary: %+v", result)
	}
	if result.Outcomes[0].Reason == nil || *result.Outcomes[0].Reason != "already_certified" || result.Outcomes[2].Reason == nil || *result.Outcomes[2].Reason != "incomplete" {
		t.Fatalf("wrong skip reasons: %+v", result.Outcomes)
	}
	status, conflict := f.request(t, "POST", path, actor, "sequential", `{"course_id":"course-east-safety","note":"changed"}`)
	wantStatus(t, status, 409, conflict)
	if !bytes.Contains(conflict, []byte(`"code":"idempotency_conflict"`)) {
		t.Fatalf("wrong conflict: %s", conflict)
	}
	if n := count(t, f.db, `SELECT COUNT(*) FROM bulk_issue_operations`); n != 1 {
		t.Fatalf("operation count %d", n)
	}
	if n := count(t, f.db, `SELECT COUNT(*) FROM certificates WHERE tenant_id='east'`); n != 3 {
		t.Fatalf("certificate count %d", n)
	}
	if n := count(t, f.db, `SELECT COUNT(*) FROM audit_events WHERE tenant_id='east' AND action='issued'`); n != 3 {
		t.Fatalf("issuance audit count %d", n)
	}
	var stored string
	if err := f.db.QueryRow(`SELECT response_json FROM bulk_issue_operations WHERE idempotency_key='sequential'`).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if stored != string(first) {
		t.Fatal("stored result changed")
	}

	var wg sync.WaitGroup
	responses := make([][]byte, 2)
	for i := range responses {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			status, b := f.request(t, "POST", path, actor, "concurrent", body)
			wantStatus(t, status, 200, b)
			responses[i] = b
		}(i)
	}
	wg.Wait()
	if !bytes.Equal(responses[0], responses[1]) {
		t.Fatal("concurrent same-key results differed")
	}
	if n := count(t, f.db, `SELECT COUNT(*) FROM (SELECT enrollment_id FROM certificates GROUP BY enrollment_id HAVING COUNT(*)>1)`); n != 0 {
		t.Fatal("duplicate certificates")
	}
	if n := count(t, f.db, `SELECT COUNT(*) FROM (SELECT certificate_id,action FROM audit_events GROUP BY certificate_id,action HAVING COUNT(*)>1)`); n != 0 {
		t.Fatal("duplicate audits")
	}

	revokePath := "/tenants/east/certificates/cert-east-alex/revoke"
	status, b := f.request(t, "POST", revokePath, actor, "", `{}`)
	wantStatus(t, status, 200, b)
	if !bytes.Contains(b, []byte(`"result":"revoked"`)) {
		t.Fatalf("first revoke: %s", b)
	}
	status, b = f.request(t, "POST", revokePath, actor, "", `{}`)
	wantStatus(t, status, 200, b)
	if !bytes.Contains(b, []byte(`"result":"already_revoked"`)) {
		t.Fatalf("second revoke: %s", b)
	}
	if n := count(t, f.db, `SELECT COUNT(*) FROM audit_events WHERE certificate_id='cert-east-alex' AND action='revoked'`); n != 1 {
		t.Fatalf("revocation audits %d", n)
	}
}

func TestConcurrentDistinctKeys(t *testing.T) {
	f := newFixture(t)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			status, b := f.request(t, "POST", "/tenants/east/certificates/bulk-issue", "alice@east.test", fmt.Sprintf("key-%d", i), `{"course_id":"course-east-safety"}`)
			wantStatus(t, status, 200, b)
		}(i)
	}
	wg.Wait()
	if n := count(t, f.db, `SELECT COUNT(*) FROM certificates WHERE tenant_id='east'`); n != 3 {
		t.Fatalf("distinct keys issued duplicates: %d", n)
	}
	if n := count(t, f.db, `SELECT COUNT(*) FROM audit_events WHERE tenant_id='east' AND action='issued'`); n != 3 {
		t.Fatalf("audits %d", n)
	}
}

func TestRejectInvalidRequest(t *testing.T) {
	f := newFixture(t)
	for _, body := range []string{"null", `{"unexpected":1}`, `{"reason":123}`} {
		status, response := f.request(t, "POST", "/tenants/east/certificates/cert-east-alex/revoke", "alice@east.test", "", body)
		wantStatus(t, status, 400, response)
	}
	status, response := f.request(t, "GET", "/tenants/east/certificates?sort_by=invalid", "alice@east.test", "", "")
	wantStatus(t, status, 400, response)
	status, response = f.request(t, "GET", "/tenants/east/certificates?limit=101", "alice@east.test", "", "")
	wantStatus(t, status, 400, response)
	if n := count(t, f.db, `SELECT COUNT(*) FROM audit_events WHERE action='revoked'`); n != 0 {
		t.Fatalf("invalid request wrote %d audits", n)
	}
}

func TestLargeSeedExportOrderAndConcurrentList(t *testing.T) {
	f := newFixture(t)
	if err := SeedLarge(f.db, "east", 100000); err != nil {
		t.Fatal(err)
	}
	if err := SeedLarge(f.db, "east", 100000); err != nil {
		t.Fatal(err)
	}
	if n := count(t, f.db, `SELECT COUNT(*) FROM certificates WHERE tenant_id='east'`); n != 100000 {
		t.Fatalf("large seed count %d", n)
	}
	if n := count(t, f.db, `SELECT COUNT(*) FROM certificates c LEFT JOIN audit_events a ON a.tenant_id=c.tenant_id AND a.certificate_id=c.id AND a.action='issued' WHERE c.tenant_id='east' AND a.id IS NULL`); n != 0 {
		t.Fatalf("missing issuance audits %d", n)
	}
	path := "/tenants/east/certificates/export?search=Demo%20Learner%20001&sort_by=learner_name&sort_dir=desc"
	var wg sync.WaitGroup
	var exported []byte
	wg.Add(1)
	go func() {
		defer wg.Done()
		status, b := f.request(t, "GET", path, "alice@east.test", "", "")
		wantStatus(t, status, 200, b)
		exported = b
	}()
	status, b := f.request(t, "GET", "/tenants/east/certificates?search=Demo%20Learner%20001&sort_by=learner_name&sort_dir=desc", "alice@east.test", "", "")
	wantStatus(t, status, 200, b)
	var listing struct {
		Total int `json:"total"`
	}
	if err := json.Unmarshal(b, &listing); err != nil {
		t.Fatal(err)
	}
	wg.Wait()
	first, err := csv.NewReader(bytes.NewReader(exported)).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(first)-1 != listing.Total || listing.Total != 400 {
		t.Fatalf("export/list count %d/%d", len(first)-1, listing.Total)
	}
	status, b = f.request(t, "GET", path, "alice@east.test", "", "")
	wantStatus(t, status, 200, b)
	second, err := csv.NewReader(bytes.NewReader(b)).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatal("CSV row order changed")
	}
	start := time.Now()
	status, b = f.request(t, "GET", "/tenants/east/certificates/export?sort_by=issued_local_date", "alice@east.test", "", "")
	wantStatus(t, status, 200, b)
	allRows, err := csv.NewReader(bytes.NewReader(b)).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(allRows)-1 != 100000 {
		t.Fatalf("full export has %d rows", len(allRows)-1)
	}
	if elapsed := time.Since(start); elapsed > 60*time.Second {
		t.Fatalf("full export took %s", elapsed)
	}
}
