package app

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

func Open(path string) (*sql.DB, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return nil, err
	}
	u := &url.URL{Scheme: "file", Path: path}
	q := u.Query()
	q.Add("_pragma", "foreign_keys(1)")
	q.Add("_pragma", "busy_timeout(30000)")
	q.Add("_pragma", "journal_mode(WAL)")
	u.RawQuery = q.Encode()
	db, err := sql.Open("sqlite", u.String())
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(8)
	db.SetConnMaxLifetime(0)
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

const schema = `
CREATE TABLE IF NOT EXISTS tenants(id TEXT PRIMARY KEY,name TEXT NOT NULL,time_zone TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS admins(id TEXT PRIMARY KEY,tenant_id TEXT NOT NULL,email TEXT NOT NULL UNIQUE,UNIQUE(tenant_id,id),FOREIGN KEY(tenant_id) REFERENCES tenants(id));
CREATE TABLE IF NOT EXISTS courses(id TEXT PRIMARY KEY,tenant_id TEXT NOT NULL,title TEXT NOT NULL,UNIQUE(tenant_id,id),FOREIGN KEY(tenant_id) REFERENCES tenants(id));
CREATE TABLE IF NOT EXISTS learners(id TEXT PRIMARY KEY,tenant_id TEXT NOT NULL,name TEXT NOT NULL,UNIQUE(tenant_id,id),FOREIGN KEY(tenant_id) REFERENCES tenants(id));
CREATE TABLE IF NOT EXISTS enrollments(id TEXT PRIMARY KEY,tenant_id TEXT NOT NULL,course_id TEXT NOT NULL,learner_id TEXT NOT NULL,enrolled_at_utc TEXT NOT NULL,completed_at_utc TEXT,UNIQUE(tenant_id,id),UNIQUE(tenant_id,course_id,learner_id),FOREIGN KEY(tenant_id,course_id) REFERENCES courses(tenant_id,id),FOREIGN KEY(tenant_id,learner_id) REFERENCES learners(tenant_id,id));
CREATE TABLE IF NOT EXISTS certificates(id TEXT PRIMARY KEY,tenant_id TEXT NOT NULL,enrollment_id TEXT NOT NULL UNIQUE,serial_number TEXT NOT NULL UNIQUE,issued_at_utc TEXT NOT NULL,issued_at_local TEXT NOT NULL,issued_local_date TEXT NOT NULL,status TEXT NOT NULL CHECK(status IN ('issued','revoked')),revoked_at_utc TEXT,revoked_by_admin_id TEXT,UNIQUE(tenant_id,id),FOREIGN KEY(tenant_id,enrollment_id) REFERENCES enrollments(tenant_id,id),FOREIGN KEY(tenant_id,revoked_by_admin_id) REFERENCES admins(tenant_id,id),CHECK((status='issued' AND revoked_at_utc IS NULL AND revoked_by_admin_id IS NULL) OR (status='revoked' AND revoked_at_utc IS NOT NULL AND revoked_by_admin_id IS NOT NULL)));
CREATE TABLE IF NOT EXISTS bulk_issue_operations(id TEXT PRIMARY KEY,tenant_id TEXT NOT NULL,actor_admin_id TEXT NOT NULL,idempotency_key TEXT NOT NULL,request_hash TEXT NOT NULL,request_body_json TEXT NOT NULL,response_json TEXT NOT NULL,completed_at_utc TEXT NOT NULL,UNIQUE(tenant_id,id),UNIQUE(tenant_id,idempotency_key),FOREIGN KEY(tenant_id,actor_admin_id) REFERENCES admins(tenant_id,id));
CREATE TABLE IF NOT EXISTS audit_events(id TEXT PRIMARY KEY,tenant_id TEXT NOT NULL,certificate_id TEXT NOT NULL,actor_admin_id TEXT NOT NULL,action TEXT NOT NULL CHECK(action IN ('issued','revoked')),occurred_at_utc TEXT NOT NULL,bulk_operation_id TEXT,metadata_json TEXT,UNIQUE(certificate_id,action),FOREIGN KEY(tenant_id,certificate_id) REFERENCES certificates(tenant_id,id),FOREIGN KEY(tenant_id,actor_admin_id) REFERENCES admins(tenant_id,id),FOREIGN KEY(tenant_id,bulk_operation_id) REFERENCES bulk_issue_operations(tenant_id,id));
CREATE INDEX IF NOT EXISTS idx_enrollments_course ON enrollments(tenant_id,course_id,completed_at_utc);
CREATE INDEX IF NOT EXISTS idx_cert_enrollment ON certificates(tenant_id,enrollment_id);
CREATE INDEX IF NOT EXISTS idx_cert_local_date ON certificates(tenant_id,issued_local_date,id);
CREATE INDEX IF NOT EXISTS idx_cert_serial ON certificates(tenant_id,serial_number,id);
CREATE INDEX IF NOT EXISTS idx_cert_status ON certificates(tenant_id,status,id);
CREATE INDEX IF NOT EXISTS idx_audit_cert ON audit_events(tenant_id,certificate_id);
`

func Migrate(db *sql.DB) error { _, err := db.Exec(schema); return err }

func Seed(db *sql.DB) error {
	ctx := context.Background()
	conn, err := beginImmediate(ctx, db)
	if err != nil {
		return err
	}
	defer conn.Close()
	defer conn.ExecContext(ctx, "ROLLBACK")
	queries := []string{
		`INSERT OR IGNORE INTO tenants VALUES('east','East','Australia/Sydney'),('west','West','Australia/Perth')`,
		`INSERT OR IGNORE INTO admins VALUES('admin-east','east','alice@east.test'),('admin-west','west','bob@west.test')`,
		`INSERT OR IGNORE INTO courses VALUES('course-east-safety','east','Safety 101'),('course-east-extra','east','Workplace Basics'),('course-west-safety','west','Safety 101')`,
		`INSERT OR IGNORE INTO learners VALUES('learner-east-alex','east','Alex Lee'),('learner-east-blair','east','Blair Chen'),('learner-east-casey','east','Casey Diaz'),('learner-east-drew','east','Drew Evans'),('learner-west-alex','west','Alex Lee'),('learner-west-blair','west','Blair Chen'),('learner-west-casey','west','Casey Diaz')`,
		`INSERT OR IGNORE INTO enrollments VALUES('enroll-east-alex','east','course-east-safety','learner-east-alex','2026-01-01T00:00:00Z','2026-01-09T00:00:00Z'),('enroll-east-blair','east','course-east-safety','learner-east-blair','2026-01-01T00:00:00Z','2026-01-09T00:00:00Z'),('enroll-east-casey','east','course-east-safety','learner-east-casey','2026-01-01T00:00:00Z',NULL),('enroll-east-drew','east','course-east-safety','learner-east-drew','2026-01-01T00:00:00Z','2026-01-09T00:00:00Z'),('enroll-west-alex','west','course-west-safety','learner-west-alex','2026-01-01T00:00:00Z','2026-01-09T00:00:00Z'),('enroll-west-blair','west','course-west-safety','learner-west-blair','2026-01-01T00:00:00Z','2026-01-09T00:00:00Z'),('enroll-west-casey','west','course-west-safety','learner-west-casey','2026-01-01T00:00:00Z',NULL)`,
		`INSERT OR IGNORE INTO certificates(id,tenant_id,enrollment_id,serial_number,issued_at_utc,issued_at_local,issued_local_date,status) VALUES('cert-east-alex','east','enroll-east-alex','EAST-000001','2026-01-10T13:30:00Z','2026-01-11T00:30:00+11:00','2026-01-11','issued'),('cert-west-alex','west','enroll-west-alex','WEST-000001','2026-01-10T13:30:00Z','2026-01-10T21:30:00+08:00','2026-01-10','issued')`,
		`INSERT OR IGNORE INTO audit_events(id,tenant_id,certificate_id,actor_admin_id,action,occurred_at_utc) VALUES('audit-east-alex','east','cert-east-alex','admin-east','issued','2026-01-10T13:30:00Z'),('audit-west-alex','west','cert-west-alex','admin-west','issued','2026-01-10T13:30:00Z')`,
	}
	for _, q := range queries {
		if _, err := conn.ExecContext(ctx, q); err != nil {
			return err
		}
	}
	_, err = conn.ExecContext(ctx, "COMMIT")
	return err
}

func beginImmediate(ctx context.Context, db *sql.DB) (*sql.Conn, error) {
	conn, err := db.Conn(ctx)
	if err != nil {
		return nil, err
	}
	if _, err = conn.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
		conn.Close()
		return nil, err
	}
	return conn, nil
}

func SeedLargeCLI(db *sql.DB, args []string) error {
	if len(args) != 4 || args[0] != "--tenant" || args[2] != "--count" {
		return fmt.Errorf("usage: seed-large --tenant east|west --count 100000")
	}
	count, err := strconv.Atoi(args[3])
	if err != nil || count < 1 {
		return fmt.Errorf("count must be positive")
	}
	return SeedLarge(db, args[1], count)
}

func SeedLarge(db *sql.DB, tenant string, count int) error {
	if tenant != "east" && tenant != "west" {
		return fmt.Errorf("unknown tenant")
	}
	var existing int
	if err := db.QueryRow(`SELECT COUNT(*) FROM certificates WHERE tenant_id=?`, tenant).Scan(&existing); err != nil {
		return err
	}
	if existing > count {
		return fmt.Errorf("tenant already has %d certificates, greater than requested %d", existing, count)
	}
	zone := "Australia/Sydney"
	admin := "admin-east"
	course := "course-east-safety"
	if tenant == "west" {
		zone = "Australia/Perth"
		admin = "admin-west"
		course = "course-west-safety"
	}
	loc, err := time.LoadLocation(zone)
	if err != nil {
		return err
	}
	// Stable generated IDs make interrupted runs and repeated invocations safe.
	for start := 1; existing < count; start += 1000 {
		ctx := context.Background()
		conn, err := beginImmediate(ctx, db)
		if err != nil {
			return err
		}
		end := start + 999
		for i := start; i <= end; i++ {
			if existing >= count {
				break
			}
			id := fmt.Sprintf("large-%s-%06d", tenant, i)
			learner := "learner-" + id
			enrollment := "enroll-" + id
			cert := "cert-" + id
			audit := "audit-" + id
			name := fmt.Sprintf("Demo Learner %03d", i%250) // duplicates for search and sort checks
			instant := time.Date(2026, 1, 10+(i%20), 13, 30, 0, 0, time.UTC)
			utc := instant.Format(time.RFC3339)
			local := instant.In(loc)
			serial := fmt.Sprintf("%s-L%06d", strings.ToUpper(tenant), i)
			if _, err = conn.ExecContext(ctx, `INSERT OR IGNORE INTO learners(id,tenant_id,name) VALUES(?,?,?)`, learner, tenant, name); err != nil {
				break
			}
			if _, err = conn.ExecContext(ctx, `INSERT OR IGNORE INTO enrollments(id,tenant_id,course_id,learner_id,enrolled_at_utc,completed_at_utc) VALUES(?,?,?,?,?,?)`, enrollment, tenant, course, learner, utc, utc); err != nil {
				break
			}
			var res sql.Result
			if res, err = conn.ExecContext(ctx, `INSERT OR IGNORE INTO certificates(id,tenant_id,enrollment_id,serial_number,issued_at_utc,issued_at_local,issued_local_date,status) VALUES(?,?,?,?,?,?,?,'issued')`, cert, tenant, enrollment, serial, utc, local.Format(time.RFC3339), local.Format("2006-01-02")); err != nil {
				break
			}
			if n, _ := res.RowsAffected(); n == 1 {
				existing++
			}
			if _, err = conn.ExecContext(ctx, `INSERT OR IGNORE INTO audit_events(id,tenant_id,certificate_id,actor_admin_id,action,occurred_at_utc) VALUES(?,?,?,?,'issued',?)`, audit, tenant, cert, admin, utc); err != nil {
				break
			}
		}
		if err != nil {
			conn.ExecContext(ctx, "ROLLBACK")
			conn.Close()
			return err
		}
		_, err = conn.ExecContext(ctx, "COMMIT")
		conn.Close()
		if err != nil {
			return err
		}
	}
	return nil
}
