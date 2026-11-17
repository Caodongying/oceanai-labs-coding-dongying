# Decision log

## Accepted AI suggestion

- Decision: AI listed the priority of backend tasks after AGENTS.md is done. I accepted it as the implementation flow looks natural to me: database including seed data comes first, API endpoints follows and the shell scripts are the last part.
- **Session:** [`01a1e9a6-5731-78a0-b50a-a0c13d639050` (backend), task-priority response](sessions/rollout-2026-11-17T21-15-55-01a1e9a6-5731-78a0-b50a-a0c13d639050.jsonl#L51).
- **Verification evidence:** The implementation contains [migrations and repeatable seeds](../backend/internal/app/database.go#L40), [tenant-scoped API routes](../backend/internal/app/api.go#L61), and [startup](../scripts/dev.sh) and [verification](../scripts/test.sh) scripts. The [backend session's Docker test build](sessions/rollout-2026-11-17T21-15-55-01a1e9a6-5731-78a0-b50a-a0c13d639050.jsonl#L481) reports `go test ./...` and `go vet ./...` completing successfully.
- **Related commits:** [backend MVP `f354c50`](https://github.com/Caodongying/oceanai-labs-coding-dongying/commit/f354c505caf32538de53e7cb3a4f28e381288cc7) and [verification `87112e3`](https://github.com/Caodongying/oceanai-labs-coding-dongying/commit/87112e3af06021f6750d979bdb5d90fc358f86e2).

## Rejected or corrected AI suggestion

- Decision: AI didn't provide the database models by default when writing plannings in AGENTS.md. I explicitly asked for it, checked the generated models, and adopted them.
- **Session:** [`01a1e988-c976-7982-859c-f419811beae5` (planning), initial draft](sessions/rollout-2026-11-17T20-43-38-01a1e988-c976-7982-859c-f419811beae5.jsonl#L85), [model-review request and proposal](sessions/rollout-2026-11-17T20-43-38-01a1e988-c976-7982-859c-f419811beae5.jsonl#L133), [idempotency check](sessions/rollout-2026-11-17T20-43-38-01a1e988-c976-7982-859c-f419811beae5.jsonl#L150), and [approval to update the contract](sessions/rollout-2026-11-17T20-43-38-01a1e988-c976-7982-859c-f419811beae5.jsonl#L162).
- **Verification evidence:** The resulting [database design](../AGENTS.md#L165) specifies tenant-aware relationships, unique enrollment certificates, idempotency operations, audit uniqueness, and transaction rules. The [SQLite schema](../backend/internal/app/database.go#L40) implements these constraints; [backend tests](../backend/internal/app/api_test.go#L149) check retries, duplicate prevention, and audit behavior.
- **Related commit:** [shared contract `38ad31a`](https://github.com/Caodongying/oceanai-labs-coding-dongying/commit/38ad31a0f4c2934021a810a76fcd8ca1363be3cb).

## Scope or design tradeoff

- Decision: Honestly AI didn't state any obvious scope or provide alternatives for me as I asked it to divide the implementation plan to two parts clearly at the beginning. I also specified the tech stack that should be used in this project. The suggestions on task priority is also adopted. However I didn't ask AI to do the benchmark.
- **Sessions:** [`01a1e988-c976-7982-859c-f419811beae5` (planning), worktree and contract request](sessions/rollout-2026-11-17T20-43-38-01a1e988-c976-7982-859c-f419811beae5.jsonl#L36); [`01a1e9a6-5731-78a0-b50a-a0c13d639050` (backend), MVP instruction](sessions/rollout-2026-11-17T21-15-55-01a1e9a6-5731-78a0-b50a-a0c13d639050.jsonl#L58).
- **Verification evidence:** [Worktree ownership and shared API rules](../AGENTS.md#L19) record the split. The [frontend API client](../frontend/src/api.js) uses the shared `/api/v1` contract, and the [backend router](../backend/internal/app/api.go#L61) serves it. The required A6 scale check is present in [the backend test](../backend/internal/app/api_test.go#L266).
- **Related commits:** [shared contract `38ad31a`](https://github.com/Caodongying/oceanai-labs-coding-dongying/commit/38ad31a0f4c2934021a810a76fcd8ca1363be3cb), [backend MVP `f354c50`](https://github.com/Caodongying/oceanai-labs-coding-dongying/commit/f354c505caf32538de53e7cb3a4f28e381288cc7), and [frontend implementation `718ff91`](https://github.com/Caodongying/oceanai-labs-coding-dongying/commit/718ff91fad87627185922992bb45261eff9bc13b).
