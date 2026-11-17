import { useEffect, useState } from 'react'
import { exportCertificates, query, request, tenantPath } from './api.js'

const sortOptions = [
  ['issued_local_date', 'Issue date'],
  ['learner_name', 'Learner'],
  ['course_title', 'Course'],
  ['serial_number', 'Serial number'],
  ['status', 'Status'],
]
const pageSize = 20

function Badge({ value }) {
  return <span className={`badge badge-${value.replaceAll('_', '-')}`}>{value.replaceAll('_', ' ')}</span>
}

function localTime(value) {
  return value ? value.replace('T', ' ').slice(0, 16) : '—'
}

function Message({ children, retry }) {
  return <div className="empty" role="alert"><strong>{children}</strong>{retry && <p><button className="button subtle" onClick={retry}>Try again</button></p>}</div>
}

function Pagination({ page, totalPages, total, limit, onPage }) {
  const first = total ? (page - 1) * limit + 1 : 0
  const last = Math.min(page * limit, total)
  return <div className="pagination"><span>{first}–{last} of {total}</span><div><button disabled={page <= 1} onClick={() => onPage(page - 1)}>← Previous</button><span>Page {page} of {totalPages || 1}</span><button disabled={page >= totalPages} onClick={() => onPage(page + 1)}>Next →</button></div></div>
}

function CertificateDetail({ email, tenant, id, onClose, onChanged }) {
  const [certificate, setCertificate] = useState(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [confirm, setConfirm] = useState(false)
  const [reason, setReason] = useState('')
  const [revoking, setRevoking] = useState(false)
  const [notice, setNotice] = useState('')
  const [reload, setReload] = useState(0)

  useEffect(() => {
    const controller = new AbortController()
    setLoading(true)
    setError('')
    request(tenantPath(tenant.id, `/certificates/${encodeURIComponent(id)}`), { email, signal: controller.signal })
      .then(response => { if (!controller.signal.aborted) setCertificate(response) })
      .catch(err => { if (err.name !== 'AbortError') setError(err.message) })
      .finally(() => { if (!controller.signal.aborted) setLoading(false) })
    return () => controller.abort()
  }, [email, tenant.id, id, reload])

  async function revoke() {
    if (revoking || !certificate || reason.length > 200) return
    setRevoking(true)
    setError('')
    try {
      const response = await request(tenantPath(tenant.id, `/certificates/${encodeURIComponent(id)}/revoke`), {
        email, method: 'POST', body: reason ? { reason } : {},
      })
      setCertificate(response.certificate)
      setNotice(response.result === 'already_revoked' ? 'This certificate was already revoked.' : 'Certificate revoked.')
      setConfirm(false)
      onChanged()
    } catch (err) { setError(err.message) }
    finally { setRevoking(false) }
  }

  return <div className="drawer-backdrop" onMouseDown={event => { if (event.target === event.currentTarget) onClose() }}><aside className="drawer" aria-label="Certificate details"><div className="drawer-header"><div><div className="eyebrow">CERTIFICATE DETAILS</div><h2>{certificate?.serial_number || 'Certificate'}</h2></div><button className="icon-button" onClick={onClose} aria-label="Close details">×</button></div>
    {loading ? <p role="status">Loading certificate…</p> : error && !certificate ? <Message retry={() => setReload(value => value + 1)}>{error}</Message> : certificate && <><Badge value={certificate.status} /><div className="detail-list"><div><span>Serial number</span><strong>{certificate.serial_number}</strong></div><div><span>Learner</span><strong>{certificate.learner.name}</strong></div><div><span>Course</span><strong>{certificate.course.title}</strong></div><div><span>Tenant</span><strong>{tenant.name}</strong></div><div><span>Issued locally</span><strong>{localTime(certificate.issued_at_local)}<small>{tenant.time_zone}</small></strong></div><div><span>Local date</span><strong>{certificate.issued_local_date}</strong></div></div>{notice && <div className="notice" role="status">{notice}</div>}<div className="drawer-actions"><button className="button subtle" onClick={onClose}>Close</button>{certificate.status === 'issued' && <button className="button danger" onClick={() => setConfirm(true)}>Revoke certificate</button>}</div></>}
    {error && certificate && !confirm && <p className="inline-error" role="alert">{error}</p>}
  </aside>{confirm && certificate && <div className="modal-backdrop"><div className="modal" role="dialog" aria-modal="true" aria-labelledby="revoke-title"><h2 id="revoke-title">Revoke this certificate?</h2><p>{certificate.serial_number} for {certificate.learner.name} will be marked revoked.</p><label className="field-label" htmlFor="revoke-reason">Reason (optional)</label><input id="revoke-reason" maxLength={200} value={reason} onChange={event => setReason(event.target.value)} />{error && <p className="inline-error" role="alert">{error}</p>}<div><button className="button subtle" disabled={revoking} onClick={() => setConfirm(false)}>Cancel</button><button className="button danger" disabled={revoking} onClick={revoke}>{revoking ? 'Revoking…' : 'Confirm revoke'}</button></div></div></div>}</div>
}

function Certificates({ email, tenant, refresh }) {
  const [search, setSearch] = useState('')
  const [sortBy, setSortBy] = useState('issued_local_date')
  const [sortDir, setSortDir] = useState('desc')
  const [page, setPage] = useState(1)
  const [data, setData] = useState(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [reload, setReload] = useState(0)
  const [selectedId, setSelectedId] = useState(null)
  const [exporting, setExporting] = useState(false)
  const [exportProgress, setExportProgress] = useState('')
  const [exportError, setExportError] = useState('')

  useEffect(() => {
    const controller = new AbortController()
    setLoading(true)
    setError('')
    setData(null)
    const params = query({ search: search.trim(), page, limit: pageSize, sort_by: sortBy, sort_dir: sortDir })
    request(tenantPath(tenant.id, `/certificates?${params}`), { email, signal: controller.signal })
      .then(response => { if (!controller.signal.aborted) setData(response) })
      .catch(err => { if (err.name !== 'AbortError') setError(err.message) })
      .finally(() => { if (!controller.signal.aborted) setLoading(false) })
    return () => controller.abort()
  }, [email, tenant.id, search, page, sortBy, sortDir, refresh, reload])

  async function download() {
    if (exporting) return
    setExporting(true)
    setExportError('')
    setExportProgress('')
    try { await exportCertificates({ email, tenantId: tenant.id, search, sortBy, sortDir, onProgress: setExportProgress }) }
    catch (err) { setExportError(err.message) }
    finally { setExporting(false) }
  }

  return <><div className="page-intro"><div><div className="eyebrow">{tenant.name.toUpperCase()} TENANT · {tenant.time_zone}</div><h1>Certificates</h1><p>Find, review, export, and revoke issued certificates.</p></div><button className="button primary" onClick={download} disabled={exporting}>{exporting ? `Preparing CSV${exportProgress ? ` · ${exportProgress}` : '…'}` : '↓ Export CSV'}</button></div>
    {exportError && <p className="inline-error" role="alert">CSV export failed: {exportError}</p>}
    <section className="panel"><div className="panel-heading"><div><h2>Certificate list</h2><p>Search this tenant’s records and open a row for details.</p></div></div><div className="toolbar"><label className="search-field"><span>⌕</span><input aria-label="Search learner or serial" placeholder="Search learner or serial…" value={search} onChange={event => { setSearch(event.target.value); setPage(1) }} /></label><div className="toolbar-right"><label>Sort by <select aria-label="Sort by" value={sortBy} onChange={event => { setSortBy(event.target.value); setPage(1) }}>{sortOptions.map(([value, label]) => <option key={value} value={value}>{label}</option>)}</select></label><button className="button subtle" onClick={() => { setSortDir(value => value === 'asc' ? 'desc' : 'asc'); setPage(1) }} aria-label={`Sort ${sortDir === 'asc' ? 'descending' : 'ascending'}`}>{sortDir === 'asc' ? '↑ Asc' : '↓ Desc'}</button></div></div>
      {loading ? <Message>Loading certificates…</Message> : error ? <Message retry={() => setReload(value => value + 1)}>{error}</Message> : <><div className="table-scroll">{data.items.length ? <table><thead><tr><th>Learner</th><th>Course</th><th>Serial number</th><th>Issued locally</th><th>Status</th><th></th></tr></thead><tbody>{data.items.map(item => <tr key={item.id}><td className="strong-cell">{item.learner.name}</td><td>{item.course.title}</td><td className="mono">{item.serial_number}</td><td>{localTime(item.issued_at_local)}</td><td><Badge value={item.status} /></td><td><button className="row-open" onClick={() => setSelectedId(item.id)} aria-label={`Open ${item.serial_number}`}>→</button></td></tr>)}</tbody></table> : <Message>No certificates found</Message>}</div><Pagination page={data.page} totalPages={data.total_pages} total={data.total} limit={data.limit} onPage={setPage} /></>}
    </section>{selectedId && <CertificateDetail key={selectedId} id={selectedId} email={email} tenant={tenant} onClose={() => setSelectedId(null)} onChanged={() => setReload(value => value + 1)} />}</>
}

function Courses({ email, tenant }) {
  const [courses, setCourses] = useState([])
  const [courseLoading, setCourseLoading] = useState(true)
  const [courseError, setCourseError] = useState('')
  const [courseReload, setCourseReload] = useState(0)
  const [courseId, setCourseId] = useState('')
  const [page, setPage] = useState(1)
  const [enrollments, setEnrollments] = useState(null)
  const [enrollmentLoading, setEnrollmentLoading] = useState(false)
  const [enrollmentError, setEnrollmentError] = useState('')
  const [enrollmentReload, setEnrollmentReload] = useState(0)
  const [note, setNote] = useState('')
  const [operation, setOperation] = useState(null)
  const [issuing, setIssuing] = useState(false)
  const [issueError, setIssueError] = useState('')
  const [result, setResult] = useState(null)

  useEffect(() => {
    const controller = new AbortController()
    setCourseLoading(true)
    setCourseError('')
    request(tenantPath(tenant.id, '/courses'), { email, signal: controller.signal })
      .then(response => {
        if (controller.signal.aborted) return
        setCourses(response.items)
        setCourseId(previous => response.items.some(item => item.id === previous) ? previous : response.items[0]?.id || '')
      })
      .catch(err => { if (err.name !== 'AbortError') setCourseError(err.message) })
      .finally(() => { if (!controller.signal.aborted) setCourseLoading(false) })
    return () => controller.abort()
  }, [email, tenant.id, courseReload])

  useEffect(() => {
    if (!courseId) return
    const controller = new AbortController()
    setEnrollmentLoading(true)
    setEnrollmentError('')
    setEnrollments(null)
    request(tenantPath(tenant.id, `/courses/${encodeURIComponent(courseId)}/enrollments?${query({ page, limit: pageSize })}`), { email, signal: controller.signal })
      .then(response => { if (!controller.signal.aborted) setEnrollments(response) })
      .catch(err => { if (err.name !== 'AbortError') setEnrollmentError(err.message) })
      .finally(() => { if (!controller.signal.aborted) setEnrollmentLoading(false) })
    return () => controller.abort()
  }, [email, tenant.id, courseId, page, enrollmentReload])

  async function submit(op) {
    if (issuing) return
    setIssuing(true)
    setIssueError('')
    try {
      const response = await request(tenantPath(tenant.id, '/certificates/bulk-issue'), {
        email, method: 'POST', idempotencyKey: op.key,
        body: { course_id: op.courseId, ...(op.note ? { note: op.note } : {}) },
      })
      setResult(response)
      setOperation(null)
      setEnrollmentReload(value => value + 1)
    } catch (err) { setIssueError(err.message) }
    finally { setIssuing(false) }
  }

  function startIssue() {
    if (issuing || !courseId) return
    const op = { key: crypto.randomUUID(), courseId, note: note.trim() }
    setOperation(op)
    setResult(null)
    submit(op)
  }

  return <><div className="page-intro"><div><div className="eyebrow">{tenant.name.toUpperCase()} TENANT · {tenant.time_zone}</div><h1>Course completion review</h1><p>Check learner completion before issuing certificates.</p></div></div>
    <section className="panel"><div className="panel-heading"><div><h2>Choose a course</h2><p>Review completion and certificate state for each learner.</p></div></div>{courseLoading ? <Message>Loading courses…</Message> : courseError ? <Message retry={() => setCourseReload(value => value + 1)}>{courseError}</Message> : courses.length === 0 ? <Message>No courses available</Message> : <div className="course-picker"><label htmlFor="course">Course</label><select id="course" value={courseId} disabled={issuing || Boolean(operation)} onChange={event => { setCourseId(event.target.value); setPage(1); setResult(null); setIssueError('') }}>{courses.map(course => <option key={course.id} value={course.id}>{course.title}</option>)}</select><span>{enrollments?.total ?? '—'} learners enrolled</span></div>}</section>
    {courseId && <section className="panel"><div className="panel-heading"><div><h2>Enrollments</h2><p>Completed learners without a certificate are eligible.</p></div><button className="button primary" onClick={startIssue} disabled={issuing || Boolean(operation) || !enrollments || enrollmentLoading || Boolean(enrollmentError)}>{issuing ? 'Issuing…' : 'Issue eligible certificates'}</button></div><div className="issue-note"><label htmlFor="issue-note">Audit note (optional)</label><input id="issue-note" maxLength={200} value={note} disabled={issuing || Boolean(operation)} onChange={event => setNote(event.target.value)} placeholder="Reason for this operation" /></div>
      {enrollmentLoading ? <Message>Loading enrollments…</Message> : enrollmentError ? <Message retry={() => setEnrollmentReload(value => value + 1)}>{enrollmentError}</Message> : enrollments && <><div className="table-scroll">{enrollments.items.length ? <table><thead><tr><th>Learner</th><th>Completion</th><th>Certificate</th><th>Eligibility</th></tr></thead><tbody>{enrollments.items.map(row => <tr key={row.learner_id}><td className="strong-cell">{row.learner_name}</td><td><Badge value={row.completion_status} /></td><td><Badge value={row.certificate_status} /></td><td>{row.completion_status === 'incomplete' ? 'Incomplete' : row.certificate_status !== 'none' ? 'Already certified' : 'Ready to issue'}</td></tr>)}</tbody></table> : <Message>No enrollments found</Message>}</div><Pagination page={enrollments.page} totalPages={enrollments.total_pages} total={enrollments.total} limit={enrollments.limit} onPage={setPage} /></>}
    </section>}
    {issueError && <div className="issue-error" role="alert"><strong>Issuance failed: {issueError}</strong>{operation && <div><button className="button primary" disabled={issuing} onClick={() => submit(operation)}>Retry same operation</button><button className="button subtle" disabled={issuing} onClick={() => { setOperation(null); setIssueError('') }}>Start a new operation</button></div>}</div>}
    {result && <section className="panel outcome-panel"><div className="panel-heading"><div><div className="eyebrow">OPERATION {result.status.toUpperCase()}</div><h2>Issue results</h2><p>Operation {result.operation_id}</p></div><div className="result-counts"><span>{result.summary.issued} issued</span><span>{result.summary.skipped} skipped</span><span>{result.summary.failed} failed</span></div></div><div className="table-scroll"><table><thead><tr><th>Learner</th><th>Result</th><th>Reason</th><th>Serial number</th></tr></thead><tbody>{result.outcomes.map(row => <tr key={row.learner_id}><td className="strong-cell">{row.learner_name}</td><td><Badge value={row.result} /></td><td>{row.reason?.replaceAll('_', ' ') || '—'}</td><td className="mono">{row.serial_number || '—'}</td></tr>)}</tbody></table></div></section>}
  </>
}

function Workspace({ email, tenant, admins, onActor }) {
  const [view, setView] = useState('certificates')
  return <div className="app-shell"><aside className="sidebar"><div className="brand"><div className="brand-mark">C</div><div><strong>Certificate<br />Console</strong><small>Admin workspace</small></div></div><div className="nav-caption">WORKSPACE</div><button className={`nav-item ${view === 'certificates' ? 'active' : ''}`} onClick={() => setView('certificates')}><span>▦</span> Certificates</button><button className={`nav-item ${view === 'courses' ? 'active' : ''}`} onClick={() => setView('courses')}><span>☷</span> Course review</button><div className="sidebar-bottom"><span className="preview-dot" /> {tenant.name} tenant<br /><small>{tenant.time_zone}</small></div></aside><main className="main"><header className="topbar"><div className="breadcrumb">Workspace <span>/</span> {view === 'certificates' ? 'Certificates' : 'Course review'}</div><div className="actor-control"><label htmlFor="actor">Viewing as</label><select id="actor" value={email} onChange={event => onActor(event.target.value)}>{admins.map(admin => <option key={admin.email} value={admin.email}>{admin.email}</option>)}</select><div className="avatar">{email[0]?.toUpperCase()}</div></div></header><div className="content">{view === 'certificates' ? <Certificates email={email} tenant={tenant} /> : <Courses email={email} tenant={tenant} />}</div></main></div>
}

export default function App() {
  const [admins, setAdmins] = useState([])
  const [adminLoading, setAdminLoading] = useState(true)
  const [adminError, setAdminError] = useState('')
  const [adminReload, setAdminReload] = useState(0)
  const [email, setEmail] = useState('')
  const [me, setMe] = useState(null)
  const [meLoading, setMeLoading] = useState(false)
  const [meError, setMeError] = useState('')
  const [meReload, setMeReload] = useState(0)

  useEffect(() => {
    const controller = new AbortController()
    setAdminLoading(true)
    setAdminError('')
    request('/demo/admins', { signal: controller.signal })
      .then(response => { if (!controller.signal.aborted) { setAdmins(response.items); setEmail(previous => response.items.some(item => item.email === previous) ? previous : response.items[0]?.email || '') } })
      .catch(err => { if (err.name !== 'AbortError') setAdminError(err.message) })
      .finally(() => { if (!controller.signal.aborted) setAdminLoading(false) })
    return () => controller.abort()
  }, [adminReload])

  useEffect(() => {
    if (!email) return
    const controller = new AbortController()
    setMe(null)
    setMeLoading(true)
    setMeError('')
    request('/me', { email, signal: controller.signal })
      .then(response => { if (!controller.signal.aborted) setMe(response) })
      .catch(err => { if (err.name !== 'AbortError') setMeError(err.message) })
      .finally(() => { if (!controller.signal.aborted) setMeLoading(false) })
    return () => controller.abort()
  }, [email, meReload])

  if (adminLoading) return <div className="startup"><p role="status">Loading admins…</p></div>
  if (adminError) return <div className="startup"><Message retry={() => setAdminReload(value => value + 1)}>{adminError}</Message></div>
  if (!email) return <div className="startup"><Message>No demo admins available</Message></div>
  if (meLoading || !me && !meError || me && me.email !== email) return <div className="startup"><p role="status">Loading workspace…</p></div>
  if (meError) return <div className="startup"><Message retry={() => setMeReload(value => value + 1)}>{meError}</Message><label htmlFor="actor-recovery">Viewing as</label><select id="actor-recovery" value={email} onChange={event => setEmail(event.target.value)}>{admins.map(admin => <option key={admin.email} value={admin.email}>{admin.email}</option>)}</select></div>
  return <Workspace key={email} email={email} tenant={me.tenant} admins={admins} onActor={setEmail} />
}
