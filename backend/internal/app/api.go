package app

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

type server struct{ db *sql.DB }
type actor struct{ ID, TenantID, Email, TenantName, TimeZone string }
type course struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}
type named struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}
type certificate struct {
	ID              string `json:"id"`
	SerialNumber    string `json:"serial_number"`
	Learner         named  `json:"learner"`
	Course          course `json:"course"`
	IssuedAtUTC     string `json:"issued_at_utc"`
	IssuedAtLocal   string `json:"issued_at_local"`
	IssuedLocalDate string `json:"issued_local_date"`
	Status          string `json:"status"`
}
type outcome struct {
	LearnerID     string  `json:"learner_id"`
	LearnerName   string  `json:"learner_name"`
	Result        string  `json:"result"`
	Reason        *string `json:"reason"`
	CertificateID *string `json:"certificate_id"`
	SerialNumber  *string `json:"serial_number"`
}
type bulkResult struct {
	OperationID string `json:"operation_id"`
	Status      string `json:"status"`
	Course      course `json:"course"`
	Summary     struct {
		Issued  int `json:"issued"`
		Skipped int `json:"skipped"`
		Failed  int `json:"failed"`
	} `json:"summary"`
	Outcomes []outcome `json:"outcomes"`
}

func Router(db *sql.DB) *gin.Engine {
	s := &server{db: db}
	r := gin.New()
	r.Use(gin.Recovery())
	r.GET("/api/v1/demo/admins", s.admins)
	p := r.Group("/api/v1")
	p.Use(s.auth)
	p.GET("/me", s.me)
	p.GET("/tenants/:tenant_id/courses", s.courses)
	p.GET("/tenants/:tenant_id/courses/:course_id/enrollments", s.enrollments)
	p.GET("/tenants/:tenant_id/certificates", s.certificates)
	p.GET("/tenants/:tenant_id/certificates/export", s.export)
	p.GET("/tenants/:tenant_id/certificates/:certificate_id", s.detail)
	p.POST("/tenants/:tenant_id/certificates/bulk-issue", s.bulkIssue)
	p.POST("/tenants/:tenant_id/certificates/:certificate_id/revoke", s.revoke)
	r.NoRoute(func(c *gin.Context) { fail(c, 404, "not_found", "Resource not found") })
	return r
}

func fail(c *gin.Context, status int, code, message string) {
	c.JSON(status, gin.H{"error": gin.H{"code": code, "message": message}})
}
func bad(c *gin.Context)      { fail(c, 400, "bad_request", "Invalid request") }
func internal(c *gin.Context) { fail(c, 500, "internal_error", "Internal server error") }
func notFound(c *gin.Context) { fail(c, 404, "not_found", "Resource not found") }
func validQuery(c *gin.Context, allowed ...string) bool {
	m := map[string]bool{}
	for _, k := range allowed {
		m[k] = true
	}
	for k, v := range c.Request.URL.Query() {
		if !m[k] || len(v) != 1 {
			return false
		}
	}
	return true
}
func noQuery(c *gin.Context) bool { return validQuery(c) }
func decodeBody(c *gin.Context, dst any) error {
	if c.ContentType() != "application/json" {
		return errors.New("content type")
	}
	raw, err := io.ReadAll(io.LimitReader(c.Request.Body, 1<<20))
	if err != nil {
		return err
	}
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return errors.New("JSON object required")
	}
	dec := json.NewDecoder(bytes.NewReader(trimmed))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return err
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		return errors.New("trailing JSON")
	}
	return nil
}
func uid(prefix string) string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	return prefix + hex.EncodeToString(b[:])
}
func utcNow() string { return time.Now().UTC().Format(time.RFC3339) }

func (s *server) auth(c *gin.Context) {
	email := strings.TrimSpace(c.GetHeader("X-Admin-Email"))
	var a actor
	err := s.db.QueryRowContext(c.Request.Context(), `SELECT a.id,a.tenant_id,a.email,t.name,t.time_zone FROM admins a JOIN tenants t ON t.id=a.tenant_id WHERE a.email=?`, email).Scan(&a.ID, &a.TenantID, &a.Email, &a.TenantName, &a.TimeZone)
	if err == sql.ErrNoRows || email == "" {
		fail(c, 401, "unauthorized", "Unauthorized")
		c.Abort()
		return
	}
	if err != nil {
		internal(c)
		c.Abort()
		return
	}
	if tid := c.Param("tenant_id"); tid != "" && tid != a.TenantID {
		notFound(c)
		c.Abort()
		return
	}
	c.Set("actor", a)
	c.Next()
}
func getActor(c *gin.Context) actor { return c.MustGet("actor").(actor) }

func (s *server) admins(c *gin.Context) {
	if !noQuery(c) {
		bad(c)
		return
	}
	rows, err := s.db.QueryContext(c.Request.Context(), `SELECT a.email,a.tenant_id,t.name FROM admins a JOIN tenants t ON t.id=a.tenant_id ORDER BY a.email`)
	if err != nil {
		internal(c)
		return
	}
	defer rows.Close()
	items := []gin.H{}
	for rows.Next() {
		var email, tid, name string
		if err = rows.Scan(&email, &tid, &name); err != nil {
			internal(c)
			return
		}
		items = append(items, gin.H{"email": email, "tenant_id": tid, "tenant_name": name})
	}
	if rows.Err() != nil {
		internal(c)
		return
	}
	c.JSON(200, gin.H{"items": items})
}
func (s *server) me(c *gin.Context) {
	if !noQuery(c) {
		bad(c)
		return
	}
	a := getActor(c)
	c.JSON(200, gin.H{"email": a.Email, "tenant": gin.H{"id": a.TenantID, "name": a.TenantName, "time_zone": a.TimeZone}})
}
func (s *server) courses(c *gin.Context) {
	if !noQuery(c) {
		bad(c)
		return
	}
	a := getActor(c)
	rows, err := s.db.QueryContext(c.Request.Context(), `SELECT id,title FROM courses WHERE tenant_id=? ORDER BY title,id`, a.TenantID)
	if err != nil {
		internal(c)
		return
	}
	defer rows.Close()
	items := []course{}
	for rows.Next() {
		var x course
		if err = rows.Scan(&x.ID, &x.Title); err != nil {
			internal(c)
			return
		}
		items = append(items, x)
	}
	if rows.Err() != nil {
		internal(c)
		return
	}
	c.JSON(200, gin.H{"items": items})
}
func pagination(c *gin.Context) (int, int, error) {
	page, limit := 1, 20
	var err error
	if v, ok := c.GetQuery("page"); ok {
		page, err = strconv.Atoi(v)
		if err != nil || page < 1 {
			return 0, 0, errors.New("page")
		}
	}
	if v, ok := c.GetQuery("limit"); ok {
		limit, err = strconv.Atoi(v)
		if err != nil || limit < 1 || limit > 100 {
			return 0, 0, errors.New("limit")
		}
	}
	return page, limit, nil
}
func pages(total, limit int) int {
	if total == 0 {
		return 0
	}
	return (total + limit - 1) / limit
}

func (s *server) enrollments(c *gin.Context) {
	if !validQuery(c, "page", "limit") {
		bad(c)
		return
	}
	page, limit, err := pagination(c)
	if err != nil {
		bad(c)
		return
	}
	a := getActor(c)
	courseID := c.Param("course_id")
	var exists int
	err = s.db.QueryRowContext(c.Request.Context(), `SELECT 1 FROM courses WHERE tenant_id=? AND id=?`, a.TenantID, courseID).Scan(&exists)
	if err == sql.ErrNoRows {
		notFound(c)
		return
	}
	if err != nil {
		internal(c)
		return
	}
	var total int
	err = s.db.QueryRowContext(c.Request.Context(), `SELECT COUNT(*) FROM enrollments WHERE tenant_id=? AND course_id=?`, a.TenantID, courseID).Scan(&total)
	if err != nil {
		internal(c)
		return
	}
	rows, err := s.db.QueryContext(c.Request.Context(), `SELECT e.learner_id,l.name,CASE WHEN e.completed_at_utc IS NULL THEN 'incomplete' ELSE 'completed' END,COALESCE(cert.status,'none') FROM enrollments e JOIN learners l ON l.tenant_id=e.tenant_id AND l.id=e.learner_id LEFT JOIN certificates cert ON cert.tenant_id=e.tenant_id AND cert.enrollment_id=e.id WHERE e.tenant_id=? AND e.course_id=? ORDER BY l.name,e.learner_id LIMIT ? OFFSET ?`, a.TenantID, courseID, limit, (page-1)*limit)
	if err != nil {
		internal(c)
		return
	}
	defer rows.Close()
	items := []gin.H{}
	for rows.Next() {
		var id, name, completion, status string
		if err = rows.Scan(&id, &name, &completion, &status); err != nil {
			internal(c)
			return
		}
		items = append(items, gin.H{"learner_id": id, "learner_name": name, "completion_status": completion, "certificate_status": status})
	}
	if rows.Err() != nil {
		internal(c)
		return
	}
	c.JSON(200, gin.H{"items": items, "page": page, "limit": limit, "total": total, "total_pages": pages(total, limit)})
}

type listOptions struct {
	Search, SortBy, SortDir, Order string
	Page, Limit                    int
}

var sortColumns = map[string]string{"issued_local_date": "cert.issued_local_date", "learner_name": "l.name COLLATE NOCASE", "course_title": "cr.title COLLATE NOCASE", "serial_number": "cert.serial_number COLLATE NOCASE", "status": "cert.status"}

func parseList(c *gin.Context, export bool) (listOptions, error) {
	o := listOptions{SortBy: "issued_local_date", SortDir: "desc", Page: 1, Limit: 20}
	allowed := []string{"search", "sort_by", "sort_dir"}
	if !export {
		allowed = append(allowed, "page", "limit")
	}
	if !validQuery(c, allowed...) {
		return o, errors.New("query")
	}
	o.Search = strings.TrimSpace(c.Query("search"))
	if v, present := c.GetQuery("sort_by"); present {
		o.SortBy = v
	}
	if v, present := c.GetQuery("sort_dir"); present {
		o.SortDir = v
	}
	col, ok := sortColumns[o.SortBy]
	if !ok || (o.SortDir != "asc" && o.SortDir != "desc") {
		return o, errors.New("sort")
	}
	if !export {
		var err error
		o.Page, o.Limit, err = pagination(c)
		if err != nil {
			return o, err
		}
	}
	o.Order = col + " " + strings.ToUpper(o.SortDir) + ",cert.id ASC"
	return o, nil
}

const certFrom = ` FROM certificates cert JOIN enrollments e ON e.tenant_id=cert.tenant_id AND e.id=cert.enrollment_id JOIN learners l ON l.tenant_id=e.tenant_id AND l.id=e.learner_id JOIN courses cr ON cr.tenant_id=e.tenant_id AND cr.id=e.course_id `
const certFields = `cert.id,cert.serial_number,l.id,l.name,cr.id,cr.title,cert.issued_at_utc,cert.issued_at_local,cert.issued_local_date,cert.status`

func scanCert(rows *sql.Rows) (certificate, error) {
	var x certificate
	err := rows.Scan(&x.ID, &x.SerialNumber, &x.Learner.ID, &x.Learner.Name, &x.Course.ID, &x.Course.Title, &x.IssuedAtUTC, &x.IssuedAtLocal, &x.IssuedLocalDate, &x.Status)
	return x, err
}
func scanCertRow(row *sql.Row) (certificate, error) {
	var x certificate
	err := row.Scan(&x.ID, &x.SerialNumber, &x.Learner.ID, &x.Learner.Name, &x.Course.ID, &x.Course.Title, &x.IssuedAtUTC, &x.IssuedAtLocal, &x.IssuedLocalDate, &x.Status)
	return x, err
}
func searchWhere() string {
	return ` WHERE cert.tenant_id=? AND (instr(lower(l.name),lower(?))>0 OR instr(lower(cert.serial_number),lower(?))>0)`
}

func (s *server) certificates(c *gin.Context) {
	o, err := parseList(c, false)
	if err != nil {
		bad(c)
		return
	}
	a := getActor(c)
	args := []any{a.TenantID, o.Search, o.Search}
	var total int
	err = s.db.QueryRowContext(c.Request.Context(), `SELECT COUNT(*)`+certFrom+searchWhere(), args...).Scan(&total)
	if err != nil {
		internal(c)
		return
	}
	q := `SELECT ` + certFields + certFrom + searchWhere() + ` ORDER BY ` + o.Order + ` LIMIT ? OFFSET ?`
	rows, err := s.db.QueryContext(c.Request.Context(), q, append(args, o.Limit, (o.Page-1)*o.Limit)...)
	if err != nil {
		internal(c)
		return
	}
	defer rows.Close()
	items := []certificate{}
	for rows.Next() {
		x, e := scanCert(rows)
		if e != nil {
			internal(c)
			return
		}
		items = append(items, x)
	}
	if rows.Err() != nil {
		internal(c)
		return
	}
	c.JSON(200, gin.H{"items": items, "page": o.Page, "limit": o.Limit, "total": total, "total_pages": pages(total, o.Limit), "sort_by": o.SortBy, "sort_dir": o.SortDir, "search": o.Search})
}
func (s *server) detail(c *gin.Context) {
	if !noQuery(c) {
		bad(c)
		return
	}
	a := getActor(c)
	x, err := scanCertRow(s.db.QueryRowContext(c.Request.Context(), `SELECT `+certFields+certFrom+` WHERE cert.tenant_id=? AND cert.id=?`, a.TenantID, c.Param("certificate_id")))
	if err == sql.ErrNoRows {
		notFound(c)
		return
	}
	if err != nil {
		internal(c)
		return
	}
	c.JSON(200, x)
}
func (s *server) export(c *gin.Context) {
	o, err := parseList(c, true)
	if err != nil {
		bad(c)
		return
	}
	a := getActor(c)
	q := `SELECT l.name,cr.title,cert.serial_number,cert.issued_local_date` + certFrom + searchWhere() + ` ORDER BY ` + o.Order
	rows, err := s.db.QueryContext(c.Request.Context(), q, a.TenantID, o.Search, o.Search)
	if err != nil {
		internal(c)
		return
	}
	defer rows.Close()
	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="certificates-%s.csv"`, a.TenantID))
	w := csv.NewWriter(c.Writer)
	_ = w.Write([]string{"learner_name", "course_title", "serial_number", "issued_local_date"})
	for rows.Next() {
		var name, title, serial, date string
		if err := rows.Scan(&name, &title, &serial, &date); err != nil {
			return
		}
		if err := w.Write([]string{name, title, serial, date}); err != nil {
			return
		}
	}
	w.Flush()
}

func (s *server) bulkIssue(c *gin.Context) {
	if !noQuery(c) {
		bad(c)
		return
	}
	a := getActor(c)
	key := strings.TrimSpace(c.GetHeader("Idempotency-Key"))
	if key == "" || len(key) > 128 {
		bad(c)
		return
	}
	var body struct {
		CourseID string  `json:"course_id"`
		Note     *string `json:"note"`
	}
	if decodeBody(c, &body) != nil {
		bad(c)
		return
	}
	body.CourseID = strings.TrimSpace(body.CourseID)
	if body.CourseID == "" {
		bad(c)
		return
	}
	note := ""
	if body.Note != nil {
		note = strings.TrimSpace(*body.Note)
		if len(note) > 200 {
			bad(c)
			return
		}
	}
	canonical, _ := json.Marshal(struct {
		CourseID string `json:"course_id"`
		Note     string `json:"note"`
	}{body.CourseID, note})
	hash := sha256.Sum256([]byte(a.TenantID + ":" + string(canonical)))
	fingerprint := hex.EncodeToString(hash[:])
	ctx := c.Request.Context()
	conn, err := beginImmediate(ctx, s.db)
	if err != nil {
		internal(c)
		return
	}
	defer conn.Close()
	defer conn.ExecContext(ctx, "ROLLBACK")
	var cr course
	err = conn.QueryRowContext(ctx, `SELECT id,title FROM courses WHERE tenant_id=? AND id=?`, a.TenantID, body.CourseID).Scan(&cr.ID, &cr.Title)
	if err == sql.ErrNoRows {
		notFound(c)
		return
	}
	if err != nil {
		internal(c)
		return
	}
	var oldHash, response string
	err = conn.QueryRowContext(ctx, `SELECT request_hash,response_json FROM bulk_issue_operations WHERE tenant_id=? AND idempotency_key=?`, a.TenantID, key).Scan(&oldHash, &response)
	if err == nil {
		if oldHash != fingerprint {
			fail(c, 409, "idempotency_conflict", "Idempotency key already used")
			return
		}
		c.Data(200, "application/json", []byte(response))
		return
	}
	if err != sql.ErrNoRows {
		internal(c)
		return
	}
	opID := uid("op-")
	now := utcNow()
	_, err = conn.ExecContext(ctx, `INSERT INTO bulk_issue_operations(id,tenant_id,actor_admin_id,idempotency_key,request_hash,request_body_json,response_json,completed_at_utc) VALUES(?,?,?,?,?,?,?,?)`, opID, a.TenantID, a.ID, key, fingerprint, string(canonical), "{}", now)
	if err != nil {
		internal(c)
		return
	}
	rows, err := conn.QueryContext(ctx, `SELECT e.id,l.id,l.name,e.completed_at_utc,cert.id FROM enrollments e JOIN learners l ON l.tenant_id=e.tenant_id AND l.id=e.learner_id LEFT JOIN certificates cert ON cert.tenant_id=e.tenant_id AND cert.enrollment_id=e.id WHERE e.tenant_id=? AND e.course_id=? ORDER BY l.id`, a.TenantID, cr.ID)
	if err != nil {
		internal(c)
		return
	}
	type enrollment struct {
		ID, LearnerID, Name string
		Completed, Cert     sql.NullString
	}
	es := []enrollment{}
	for rows.Next() {
		var e enrollment
		if err = rows.Scan(&e.ID, &e.LearnerID, &e.Name, &e.Completed, &e.Cert); err != nil {
			break
		}
		es = append(es, e)
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		internal(c)
		return
	}
	result := bulkResult{OperationID: opID, Status: "completed", Course: cr, Outcomes: []outcome{}}
	loc, err := time.LoadLocation(a.TimeZone)
	if err != nil {
		internal(c)
		return
	}
	for _, e := range es {
		x := outcome{LearnerID: e.LearnerID, LearnerName: e.Name}
		if !e.Completed.Valid {
			x.Result = "skipped"
			reason := "incomplete"
			x.Reason = &reason
			result.Summary.Skipped++
		} else if e.Cert.Valid {
			x.Result = "skipped"
			reason := "already_certified"
			x.Reason = &reason
			result.Summary.Skipped++
		} else {
			certID := uid("cert-")
			serial := strings.ToUpper(a.TenantID) + "-" + strings.ToUpper(uid(""))
			instant := time.Now().UTC().Truncate(time.Second)
			local := instant.In(loc)
			utc := instant.Format(time.RFC3339)
			_, err = conn.ExecContext(ctx, `INSERT INTO certificates(id,tenant_id,enrollment_id,serial_number,issued_at_utc,issued_at_local,issued_local_date,status) VALUES(?,?,?,?,?,?,?,'issued')`, certID, a.TenantID, e.ID, serial, utc, local.Format(time.RFC3339), local.Format("2006-01-02"))
			if err != nil {
				internal(c)
				return
			}
			var metadata any
			if note != "" {
				b, _ := json.Marshal(gin.H{"note": note})
				metadata = string(b)
			}
			_, err = conn.ExecContext(ctx, `INSERT INTO audit_events(id,tenant_id,certificate_id,actor_admin_id,action,occurred_at_utc,bulk_operation_id,metadata_json) VALUES(?,?,?,?,'issued',?,?,?)`, uid("audit-"), a.TenantID, certID, a.ID, utc, opID, metadata)
			if err != nil {
				internal(c)
				return
			}
			x.Result = "issued"
			x.CertificateID = &certID
			x.SerialNumber = &serial
			result.Summary.Issued++
		}
		result.Outcomes = append(result.Outcomes, x)
	}
	b, _ := json.Marshal(result)
	_, err = conn.ExecContext(ctx, `UPDATE bulk_issue_operations SET response_json=? WHERE id=?`, string(b), opID)
	if err != nil {
		internal(c)
		return
	}
	if _, err = conn.ExecContext(ctx, "COMMIT"); err != nil {
		internal(c)
		return
	}
	c.Data(200, "application/json", b)
}

func (s *server) revoke(c *gin.Context) {
	if !noQuery(c) {
		bad(c)
		return
	}
	a := getActor(c)
	var body struct {
		Reason *string `json:"reason"`
	}
	if decodeBody(c, &body) != nil {
		bad(c)
		return
	}
	reason := ""
	if body.Reason != nil {
		reason = strings.TrimSpace(*body.Reason)
		if len(reason) > 200 {
			bad(c)
			return
		}
	}
	ctx := c.Request.Context()
	conn, err := beginImmediate(ctx, s.db)
	if err != nil {
		internal(c)
		return
	}
	defer conn.Close()
	defer conn.ExecContext(ctx, "ROLLBACK")
	id := c.Param("certificate_id")
	var status string
	err = conn.QueryRowContext(ctx, `SELECT status FROM certificates WHERE tenant_id=? AND id=?`, a.TenantID, id).Scan(&status)
	if err == sql.ErrNoRows {
		notFound(c)
		return
	}
	if err != nil {
		internal(c)
		return
	}
	result := "already_revoked"
	if status == "issued" {
		now := utcNow()
		res, e := conn.ExecContext(ctx, `UPDATE certificates SET status='revoked',revoked_at_utc=?,revoked_by_admin_id=? WHERE tenant_id=? AND id=? AND status='issued'`, now, a.ID, a.TenantID, id)
		if e != nil {
			internal(c)
			return
		}
		n, _ := res.RowsAffected()
		if n == 1 {
			var metadata any
			if reason != "" {
				b, _ := json.Marshal(gin.H{"reason": reason})
				metadata = string(b)
			}
			_, err = conn.ExecContext(ctx, `INSERT INTO audit_events(id,tenant_id,certificate_id,actor_admin_id,action,occurred_at_utc,metadata_json) VALUES(?,?,?,?,'revoked',?,?)`, uid("audit-"), a.TenantID, id, a.ID, now, metadata)
			if err != nil {
				internal(c)
				return
			}
			result = "revoked"
		}
	}
	x, err := scanCertRow(conn.QueryRowContext(ctx, `SELECT `+certFields+certFrom+` WHERE cert.tenant_id=? AND cert.id=?`, a.TenantID, id))
	if err != nil {
		internal(c)
		return
	}
	if _, err = conn.ExecContext(ctx, "COMMIT"); err != nil {
		internal(c)
		return
	}
	c.JSON(200, gin.H{"certificate": x, "result": result})
}
