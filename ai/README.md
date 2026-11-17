# AI interaction evidence — completion guide

Replace these placeholders with your original records before submission. Include every interactive AI session from planning through final documentation, including rejected suggestions and unsuccessful runs. Do not substitute a curated prompt list or screenshots for native exports.

- Put original conversation exports in `sessions/`. Keep prompts, full responses, tool calls, edits, commands, errors, timestamps, and model identifiers wherever the tool records them.
- Add one row per session to `session-index.csv`. Use one timezone consistently and link the actual export file and related commits.
- Complete `decision-log.md` with at least three material decisions and their session IDs, evidence, and commits.
- If a native export omits commands or edits, place the original activity log or a chronological equivalent in `tool-action-evidence/`.
- Complete `completeness-declaration.md` with every tool used, export limitations, and your own Section 6 statement.
- Review all evidence for credentials and private third-party data. Use narrow `[REDACTED: reason]` replacements. Do not add binary files or dependency caches as interaction evidence.

Keep `AGENTS.md` and similar instruction files in git.
