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

Or run the npm commands directly after `cd frontend`. Open the URL printed by Vite (normally http://localhost:5173). For standalone development, the Vite server proxies `/api/v1` to `http://localhost:18080` by default. Set `API_PROXY_TARGET` to override it when the backend uses another address or port. Browser code always uses relative `/api/v1` URLs; it never calls the backend port directly.

After this frontend is merged into the backend worktree, use `API_PORT=18080 ./scripts/dev.sh` from the repository root and open http://localhost:3000. Run `API_PORT=18080 ./scripts/test.sh` for integrated checks. The backend worktree owns those scripts and the browser-facing proxy in its container setup.

The UI loads admin choices from `/api/v1/demo/admins`, resolves `/api/v1/me` for the selected email, and uses only the returned tenant ID in protected URLs. Every protected request sends `X-Admin-Email`. Switching actors clears the previous workspace. Certificate search, sort, pagination, detail, revocation, course enrollments, bulk results, and CSV export use the v1 API. Failed bulk requests can be retried with the same idempotency key and body. CSV downloads use authenticated `fetch`, then a browser Blob download.
