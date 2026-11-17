import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { act, cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import '@testing-library/jest-dom/vitest'
import App from './App.jsx'

const admins = { items: [
  { email: 'alice@east.test', tenant_id: 'east', tenant_name: 'East' },
  { email: 'bob@west.test', tenant_id: 'west', tenant_name: 'West' },
] }
const tenant = id => ({ id, name: id === 'east' ? 'East' : 'West', time_zone: id === 'east' ? 'Australia/Sydney' : 'Australia/Perth' })
const certificate = (id = 'east', status = 'issued') => ({
  id: `${id}-cert-1`, serial_number: `${id.toUpperCase()}-000001`,
  learner: { id: `${id}-learner-1`, name: 'Alex Lee' },
  course: { id: `${id}-course-1`, title: 'Safety 101' },
  issued_at_utc: '2026-01-10T13:30:00Z',
  issued_at_local: id === 'east' ? '2026-01-11T00:30:00+11:00' : '2026-01-10T21:30:00+08:00',
  issued_local_date: id === 'east' ? '2026-01-11' : '2026-01-10', status,
})
const list = (id = 'east', items = [certificate(id)]) => ({ items, page: 1, limit: 20, total: items.length, total_pages: items.length ? 1 : 0, sort_by: 'issued_local_date', sort_dir: 'desc', search: '' })
const json = (data, status = 200) => new Response(JSON.stringify(data), { status, headers: { 'Content-Type': 'application/json' } })
const deferred = () => {
  let resolve
  const promise = new Promise(done => { resolve = done })
  return { promise, resolve }
}

function mockApi(handler = () => null) {
  const mocked = vi.fn(async (url, options = {}) => {
    if (url === '/api/v1/demo/admins') return json(admins)
    if (url === '/api/v1/me') {
      const email = options.headers['X-Admin-Email']
      return json({ email, tenant: tenant(email.startsWith('alice') ? 'east' : 'west') })
    }
    return handler(url, options)
  })
  vi.stubGlobal('fetch', mocked)
  return mocked
}

beforeEach(() => { vi.stubGlobal('crypto', { randomUUID: () => 'operation-key-1' }) })
afterEach(() => { cleanup(); vi.restoreAllMocks(); vi.unstubAllGlobals() })

describe('certificate console', () => {
  it('loads admins, uses /me tenant, and clears East data on actor switch', async () => {
    const fetchMock = mockApi(url => json(list(url.includes('/west/') ? 'west' : 'east')))
    render(<App />)
    expect(await screen.findByText('EAST-000001')).toBeInTheDocument()
    fireEvent.change(screen.getByRole('combobox', { name: 'Viewing as' }), { target: { value: 'bob@west.test' } })
    expect(screen.queryByText('EAST-000001')).not.toBeInTheDocument()
    expect(await screen.findByText('WEST-000001')).toBeInTheDocument()
    const westCall = fetchMock.mock.calls.find(([url]) => url.includes('/tenants/west/certificates?'))
    expect(westCall[1].headers['X-Admin-Email']).toBe('bob@west.test')
    expect(fetchMock.mock.calls.some(([url]) => url.includes('/tenants/east/') && url.includes('west'))).toBe(false)
  })

  it('shows loading, empty, and retryable error list states', async () => {
    const first = deferred()
    let calls = 0
    mockApi(url => {
      if (url.includes('/certificates?')) {
        calls += 1
        if (calls === 1) return first.promise
        if (calls === 2) return json({ error: { code: 'internal_error', message: 'Temporarily unavailable' } }, 500)
        return json(list('east', []))
      }
      throw new Error(`Unexpected ${url}`)
    })
    render(<App />)
    expect(await screen.findByText('Loading certificates…')).toBeInTheDocument()
    await act(async () => first.resolve(json(list('east', []))))
    expect(await screen.findByText('No certificates found')).toBeInTheDocument()
    fireEvent.change(screen.getByRole('combobox', { name: 'Sort by' }), { target: { value: 'learner_name' } })
    expect(await screen.findByText('Temporarily unavailable')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Try again' }))
    expect(await screen.findByText('No certificates found')).toBeInTheDocument()
  })

  it('disables duplicate bulk submission and renders per-learner outcomes', async () => {
    const bulk = deferred()
    const fetchMock = mockApi(url => {
      if (url.endsWith('/courses')) return json({ items: [{ id: 'east-course-1', title: 'Safety 101' }] })
      if (url.includes('/enrollments?')) return json({ items: [
        { learner_id: 'east-learner-1', learner_name: 'Alex Lee', completion_status: 'completed', certificate_status: 'none' },
        { learner_id: 'east-learner-2', learner_name: 'Morgan Chen', completion_status: 'incomplete', certificate_status: 'none' },
      ], page: 1, limit: 20, total: 2, total_pages: 1 })
      if (url.endsWith('/bulk-issue')) return bulk.promise
      return json(list())
    })
    render(<App />)
    await screen.findByText('EAST-000001')
    fireEvent.click(screen.getByRole('button', { name: /Course review/ }))
    await screen.findByText('Ready to issue')
    fireEvent.click(screen.getByRole('button', { name: 'Issue eligible certificates' }))
    expect(screen.getByRole('button', { name: 'Issuing…' })).toBeDisabled()
    expect(fetchMock.mock.calls.filter(([url]) => url.endsWith('/bulk-issue'))).toHaveLength(1)
    await act(async () => bulk.resolve(json({ operation_id: 'op-1', status: 'completed', course: { id: 'east-course-1', title: 'Safety 101' }, summary: { issued: 1, skipped: 1, failed: 0 }, outcomes: [
      { learner_id: 'east-learner-1', learner_name: 'Alex Lee', result: 'issued', reason: null, certificate_id: 'east-cert-2', serial_number: 'EAST-000002' },
      { learner_id: 'east-learner-2', learner_name: 'Morgan Chen', result: 'skipped', reason: 'incomplete', certificate_id: null, serial_number: null },
    ] })))
    expect(await screen.findByText('Issue results')).toBeInTheDocument()
    expect(screen.getByText('EAST-000002')).toBeInTheDocument()
    expect(screen.getByText('1 skipped')).toBeInTheDocument()
    const requestOptions = fetchMock.mock.calls.find(([url]) => url.endsWith('/bulk-issue'))[1]
    expect(requestOptions.headers['Idempotency-Key']).toBe('operation-key-1')
    expect(JSON.parse(requestOptions.body)).toEqual({ course_id: 'east-course-1' })
  })

  it('reuses the same key and body when retrying an uncertain issue response', async () => {
    let attempts = 0
    const fetchMock = mockApi(url => {
      if (url.endsWith('/courses')) return json({ items: [{ id: 'east-course-1', title: 'Safety 101' }] })
      if (url.includes('/enrollments?')) return json({ items: [], page: 1, limit: 20, total: 0, total_pages: 0 })
      if (url.endsWith('/bulk-issue')) {
        attempts += 1
        if (attempts === 1) throw new TypeError('Network failed')
        return json({ operation_id: 'op-1', status: 'completed', course: { id: 'east-course-1', title: 'Safety 101' }, summary: { issued: 0, skipped: 0, failed: 0 }, outcomes: [] })
      }
      return json(list())
    })
    render(<App />)
    await screen.findByText('EAST-000001')
    fireEvent.click(screen.getByRole('button', { name: /Course review/ }))
    await screen.findByText('No enrollments found')
    fireEvent.click(screen.getByRole('button', { name: 'Issue eligible certificates' }))
    expect(await screen.findByText('Issuance failed: Network failed')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Retry same operation' }))
    expect(await screen.findByText('Issue results')).toBeInTheDocument()
    const attemptsOptions = fetchMock.mock.calls.filter(([url]) => url.endsWith('/bulk-issue')).map(([, options]) => options)
    expect(attemptsOptions).toHaveLength(2)
    expect(attemptsOptions[0].headers['Idempotency-Key']).toBe(attemptsOptions[1].headers['Idempotency-Key'])
    expect(attemptsOptions[0].body).toBe(attemptsOptions[1].body)
  })

  it('confirms revocation and preserves search and sort for protected export', async () => {
    const fetchMock = mockApi((url, options) => {
      if (url.endsWith('/certificates/east-cert-1/revoke')) return json({ certificate: certificate('east', 'revoked'), result: 'revoked' })
      if (url.endsWith('/certificates/east-cert-1')) return json(certificate())
      if (url.includes('/certificates/export?')) return new Response('learner_name,course_title,serial_number,issued_local_date\n', { status: 200, headers: { 'Content-Type': 'text/csv; charset=utf-8' } })
      if (url.includes('/certificates?')) return json(list())
      throw new Error(`Unexpected ${url} ${options.method}`)
    })
    const NativeURL = URL
    vi.stubGlobal('URL', class extends NativeURL {
      static createObjectURL = vi.fn(() => 'blob:test')
      static revokeObjectURL = vi.fn()
    })
    vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(() => {})
    render(<App />)
    await screen.findByText('EAST-000001')
    fireEvent.click(screen.getByRole('button', { name: 'Open EAST-000001' }))
    await screen.findByRole('button', { name: 'Revoke certificate' })
    fireEvent.click(screen.getByRole('button', { name: 'Revoke certificate' }))
    expect(screen.getByRole('dialog', { name: 'Revoke this certificate?' })).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Cancel' }))
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Revoke certificate' }))
    fireEvent.click(screen.getByRole('button', { name: 'Confirm revoke' }))
    expect(await screen.findByText('Certificate revoked.')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Close details' }))
    fireEvent.change(screen.getByRole('textbox', { name: 'Search learner or serial' }), { target: { value: 'Alex' } })
    fireEvent.change(screen.getByRole('combobox', { name: 'Sort by' }), { target: { value: 'learner_name' } })
    fireEvent.click(screen.getByRole('button', { name: 'Sort ascending' }))
    fireEvent.click(screen.getByRole('button', { name: '↓ Export CSV' }))
    await waitFor(() => expect(fetchMock.mock.calls.some(([url]) => url.includes('/certificates/export?'))).toBe(true))
    const [url, options] = fetchMock.mock.calls.find(([path]) => path.includes('/certificates/export?'))
    expect(new URL(url, 'http://localhost').searchParams.get('search')).toBe('Alex')
    expect(new URL(url, 'http://localhost').searchParams.get('sort_by')).toBe('learner_name')
    expect(new URL(url, 'http://localhost').searchParams.get('sort_dir')).toBe('asc')
    expect(options.headers['X-Admin-Email']).toBe('alice@east.test')
  })
})
