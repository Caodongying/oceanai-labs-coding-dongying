const base = '/api/v1'

export class ApiError extends Error {
  constructor(message, code, status) {
    super(message)
    this.name = 'ApiError'
    this.code = code
    this.status = status
  }
}

export function query(values) {
  const params = new URLSearchParams()
  for (const [key, value] of Object.entries(values)) {
    if (value !== undefined && value !== null && value !== '') params.set(key, String(value))
  }
  return params.toString()
}

async function errorFromResponse(response) {
  let payload
  try { payload = await response.json() } catch { /* The server may return a non-JSON error. */ }
  return new ApiError(payload?.error?.message || `Request failed (${response.status})`, payload?.error?.code || 'internal_error', response.status)
}

export async function request(path, { email, method = 'GET', body, idempotencyKey, signal } = {}) {
  const headers = {}
  if (email) headers['X-Admin-Email'] = email
  if (body !== undefined) headers['Content-Type'] = 'application/json'
  if (idempotencyKey) headers['Idempotency-Key'] = idempotencyKey
  const response = await fetch(`${base}${path}`, {
    method, headers, signal,
    ...(body !== undefined ? { body: JSON.stringify(body) } : {}),
  })
  if (!response.ok) throw await errorFromResponse(response)
  return response.json()
}

export function tenantPath(tenantId, suffix) {
  return `/tenants/${encodeURIComponent(tenantId)}${suffix}`
}

export async function exportCertificates({ email, tenantId, search, sortBy, sortDir, onProgress }) {
  const params = query({ search: search.trim(), sort_by: sortBy, sort_dir: sortDir })
  const response = await fetch(`${base}${tenantPath(tenantId, '/certificates/export')}?${params}`, {
    headers: { 'X-Admin-Email': email },
  })
  if (!response.ok) throw await errorFromResponse(response)

  const total = Number(response.headers.get('Content-Length')) || 0
  let blob
  if (response.body?.getReader) {
    const reader = response.body.getReader()
    const chunks = []
    let received = 0
    while (true) {
      const { done, value } = await reader.read()
      if (done) break
      chunks.push(value)
      received += value.byteLength
      onProgress?.(total ? `${Math.round(received / total * 100)}%` : `${Math.round(received / 1024)} KB`)
    }
    blob = new Blob(chunks, { type: 'text/csv;charset=utf-8' })
  } else {
    blob = await response.blob()
  }
  const url = URL.createObjectURL(blob)
  const link = document.createElement('a')
  link.href = url
  link.download = `certificates-${tenantId}.csv`
  document.body.appendChild(link)
  link.click()
  link.remove()
  setTimeout(() => URL.revokeObjectURL(url), 1000)
}
