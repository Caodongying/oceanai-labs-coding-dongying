# Certificate console frontend

React/Vite frontend for the API in root `AGENTS.md`. Requires Node 18 or newer; `.nvmrc` selects Node 20.

From the repository root:

```sh
nvm use 20
npm --prefix frontend install
npm --prefix frontend run dev
npm --prefix frontend test
npm --prefix frontend run build
```

Or run the npm commands directly after `cd frontend`. Open the URL printed by Vite (normally http://localhost:5173). The development server proxies `/api/v1` to `http://localhost:8080`, so the backend must be running there. The backend worktree owns the integrated Compose setup and root start/test scripts.

The UI loads admin choices from `/api/v1/demo/admins`, resolves `/api/v1/me` for the selected email, and uses only the returned tenant ID in protected URLs. Every protected request sends `X-Admin-Email`. Switching actors clears the previous workspace. Certificate search, sort, pagination, detail, revocation, course enrollments, bulk results, and CSV export use the v1 API. Failed bulk requests can be retried with the same idempotency key and body. CSV downloads use authenticated `fetch`, then a browser Blob download.
