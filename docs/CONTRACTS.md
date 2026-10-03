# M0 contracts v1

## Ownership and versions

- apps/mobile: Flutter shell, Drift materialization, pending queue and secure credential references.
- apps/web: inherited React business UI plus IndexedDB reference client/PWA.
- server/internal/auth: users, devices, sessions; audit calls go through audit.Record.
- server/internal/sync: sync_notes, sync_streams, sync_events, applied_operations.
- server/internal/jobs: durable job/outbox leases; no external broker.
- server/internal/audit: lifecycle metadata, never payloads or credentials.
- provider / quota / notification / connector: capability and business domain boundaries.
- internal/api is the composition root. Inherited quota/account/notification SQL is a temporary legacy facade; new auth/sync SQL must remain in their owner modules. Moving the legacy facade into M1 services is a recorded follow-up, not an unclaimed completed refactor.
- bridge and extension: acquisition surfaces only; not trusted database writers, not enabled in M0.
- migrations owns cross-module DDL; domain modules may not mutate another domain's tables.
- Database migration version = ordered SQL filename; protocol=1, schema=1, connector_manifest_version=1 reserved. Unknown sync protocol returns 426. Breaking changes require a new ADR and version, not silent reinterpretation.
- PostgreSQL alone is authoritative. SQLite exists only in Flutter; IndexedDB only in Web. No dual-master.

The reproducible boundary check examines module imports, auth/sync table ownership and rejects the SQLite server driver.

## Auth and device lifecycle

POST /api/v1/auth/register 始终返回 403。首个管理员通过 `aihub-admin` 初始化，其后由管理员创建/启用账户。
POST /api/v1/auth/login: {email,password,device_name,device_kind,installation_id}，device_kind 为 desktop/mobile/web_admin。
Returns {token,refresh_token,expires_at,device_id,user:{id,email,role}}.
同一安装重复登录复用设备绑定；每账户 desktop/mobile 各限一台，换设备需要管理员解绑。Web 管理会话不占名额且只能访问管理 API。Passwords: bcrypt cost 10, 8–72 UTF-8 bytes.
Access: 192 random bits, 15 minutes. Refresh: independently random, 30 days.
Database stores only SHA-256 token hashes. Provider credentials are unrelated and never accepted here.

POST /api/v1/auth/refresh: {refresh_token}. Rotates both tokens in one transaction.
Old access is invalid after rotation. Used refresh hashes remain as replay detectors.
Reusing a consumed refresh revokes that device, including its newer sessions; other devices are unaffected.
A lost refresh response may require fresh login; never silently weaken replay protection.

GET /api/v1/devices; POST /api/v1/auth/logout。普通用户不得解绑；管理员使用 `/api/v1/admin/users/{id}/devices/{device}` 解绑。
Logout consumes the current session and clears local sensitive cache; device binding remains until admin unbinds it.
Every protected request checks persistence; sync writes also lock the device row inside the transaction.
Requests already in flight may finish before a revocation commits. No new request after the revoke commit is accepted.
Audit contains user/device IDs and action names, not credentials, note payloads or passwords.
Authentication endpoints use a bounded in-memory IP throttle; reverse proxies should add edge rate limiting.

| Threat | Expected behavior |
| --- | --- |
| stolen access | short lifetime; owner can revoke the device immediately |
| stolen refresh/replay | token rotation; reuse revokes the device family |
| lost phone | administrator unbinds the device; cannot remotely erase already cached offline data |
| server URL spoof/redirect | HTTPS and normal certificate verification; no userinfo/path/query in Server Profile; HTTP redirects refused |
| cross-profile credentials | vault/session keyed by profile; changing URL clears its session; cache keyed by origin, user and device |
| revoked session | queue retained; automatic network retry stops until fresh login |
| XSS | React escaped text, no HTML injection, same-origin deployment; desktop secrets use OS safe storage, browser admin session uses sessionStorage |
| native credential storage | Flutter secure storage; SQLite contains no access/refresh tokens |

## Sync wire v1

POST /api/v1/sync/push, one immutable operation per request:

~~~json
{"protocol":1,"operation_id":"client-generated-uuid","entity":"note","entity_id":"note-uuid","base_version":0,"op":"put","payload":{"title":"Continue later","body":"Draft"}}
~~~

The caller's user determines workspace; never trust a client-supplied tenant.
Only entity=note is accepted in M0. Payload is a strict object containing title/body.
Max title 200 UTF-8 bytes, body 12000 bytes, ID fields 100 bytes, request body 1 MiB.
Clients use shorter character limits; server remains the final validator.
base_version=0 creates; matching version updates/deletes; deletion produces a new version/tombstone.

200 applied: {status:"applied",event:{seq,workspace,entity,entity_id,op,version,changed_at,payload}}.
200 conflict: {status:"conflict",current:{entity_id,version,op,payload,...}}.
409 operation_id_reused: same ID with different canonical request bytes.
Unknown keys, entity or malformed values are rejected.

Applied and conflict results are persisted by (user_id,operation_id).
Identical replay returns the original result, even if the entity has since changed.
Conflict resolution creates a NEW operation_id with the last observed version.
Domain write + event + result are in one transaction; partial commits are not visible.

Each private workspace owns a sync_streams row. Writers lock it before allocating seq,
and hold the lock through commit. Do not replace with BIGSERIAL: sequence allocation order
alone does not guarantee commit order, so a pull cursor could skip a slow transaction.

GET /api/v1/sync/pull?cursor=epoch:seq&limit=100; limit 1–500.
Empty cursor replays all events. Returns {protocol:1,events,cursor,has_more}.
Pull snapshots a committed head and bounds the page by that head.
Client applies events and cursor atomically, never updates the cursor ahead of cache.
Wrong epoch, malformed or future cursor → 409 cursor_reset.
Reset drops only server materialization/cursor, preserves pending/conflicts, then replays.
No compaction is performed in M0; tombstones and idempotency results are retained.

| Case | Terminal state |
| --- | --- |
| duplicate/lost response retry | one domain change, one event, same result |
| reordered stale operation | persisted conflict, no silent write |
| concurrent base version | exactly one accepted change, other drafts preserved |
| crash before commit | no change visible; same operation safe to retry |
| crash after commit/before response | replay finds durable receipt |
| interrupted pull | previously committed page remains; failed page replayed |
| delete | versioned tombstone propagates; no resurrection by stale writes |
| cursor invalid | rebuild committed state without dropping local queue |
| device revoke | API denied, local queue retained, fresh login required |

Future conflict matrix: provider observations are server/connector-owned; user records use optimistic versions; conversation messages append-only; configs preserve version branches.
WebSocket may only be a wakeup hint, never the source of facts.

## Durable jobs

Enqueue inside the domain transaction. Claim uses FOR UPDATE SKIP LOCKED.
Lease expiry makes crashed work claimable again. Every claim gets a new lease token.
Acknowledgement checks id + lease token + unexpired lease; an old worker cannot acknowledge a new claim.
Delivery is at-least-once. Future external workers must make their side effects idempotent.

## Conversation portability v1 (M3, 2026-09-29)

Canonical graph: projects → conversations → branches → messages. A message's parent chain stays inside its branch; the branch root has no parent. Append-only: identical re-imports dedup, changed content appends new branches, history rows are never rewritten.

Dedup identity: UNIQUE(user_id, provider_slug, dedup_key). dedup_key is the source external id when provided, else the first 24 hex of the content hash (ordered branch/role/content triples), so archive round-trips dedup without server ids.

Sources v1: chatgpt_export (node-graph JSON), codex_cli_jsonl (tolerant line format, foreign lines skipped), archive (aihub.conversation-archive v1 envelope with frozen format/version/projects/conversations fields). New fields require a version bump, not reinterpretation.

v1 limits: import file ≤ 900KiB (inside the global 1MiB Guard), ≤ 50 conversations per batch, ≤ 20 branches and ≤ 4000 messages per conversation, ≤ 64KiB per message. Raising these is an ADR + object-storage decision (INH-442), not a silent relaxation.

Raw provenance: the exact uploaded bytes are retained per import and served only to the owning account; raw payloads never enter sync push/pull. Imports run through the durable job queue (kind conversation_import), API returns 202, status is polled, and each batch commits in one transaction — no partial imports. Markdown export is a rendering convenience, not a round-trip format.

## Portable config v1 (M4, 2026-09-29)

Asset kinds v1: mcp_server, prompt_template, agent_profile. Canonical content shapes are frozen: mcp_server requires a non-empty mcp_servers[] whose entries carry name and command|url; prompt_template requires body; agent_profile requires instructions.

Versions are append-only (UNIQUE(asset_id, version)); rollback appends a new version with the old content (created_by=rollback). History rows are never mutated.

Secrets: values under key names matching token/secret/password/api_key/authorization/cookie/private_key/credential are replaced by {"secret_ref":"env:NAME"} at import/create time; the value is discarded and only the NAME survives. Loss reports record path+reason, never values. No API response, export or sync payload may contain a secret value — regression-tested.

Reference transform v1: claude_desktop JSON → canonical → codex_cli TOML fragment. Secret refs render as ${NAME} placeholders with explicit loss entries; unknown fields are recorded, never silently dropped. Additional platforms and the reverse TOML parser are INH-473. Bindings are declarative targets only; file writes stay with the desktop bridge.

Bridge config discovery scans only explicitly authorized absolute directories (scan_directories, ≤8 entries), whitelisted filenames, ≤3 depth, ≤500 files, ≤256KiB per file, and uploads metadata plus secret KEY NAMES only — file contents and values never leave the machine. Config import content is capped at 256KiB in-handler.

## Workspace & collaboration v1 (M5, 2026-09-29)

Workspaces scope shared canonical assets only (conversations, config assets). Provider accounts, quotas, notifications and sync notes stay strictly personal and never enter a workspace.

Roles: owner (membership control, delete), editor (read + comment), viewer (read only). Enforcement: shared-resource reads resolve as `owner OR workspace-member`; only the resource owner mutates or unshares content; comments require editor+. Membership changes revoke access immediately (no grace window) and are audit-logged (`audit_events`: workspace_created/invited, member_added/removed, resource_shared, workspace_comment).

Invites are by email of an existing account, role-bound, pending until accepted by the matching session; acceptance and revocation are explicit. Sharing is per-resource (`workspace_id` on the row); the ownership check for the membership table stays inside the workspace package via `workspace.ReadableScope`.

Change hints v1 live on one route (`GET /api/v1/change-hints`) with two interchangeable deliveries (INH-513 closed 2026-10-02): Server-Sent Events and a WebSocket upgrade. Both carry identical semantics — per-user in-memory buffer (256 events), hello marker on connect, `Last-Event-ID` replay after reconnect (WebSocket clients may also pass `last_event_id` as a query parameter; the bearer token may arrive as `access_token` on the same query because browsers cannot set WS headers), coarse pointers only in payloads (never resource contents). Clients prefer WebSocket and fall back to SSE.

## Promotion intelligence v1 (M6, 2026-09-29)

Campaign identity: `content_hash` = provider + normalized title + normalized URL (tracking params stripped, host lowercased, trailing slash removed) + discount shape. First sight creates; later sightings only append per-source observations. The feed never shows the same campaign twice, and notifications fire at most once per user per promotion (PK `promotion_notifications`).

Time contract: `time_precision ∈ {exact, day, unknown}`; unknown means NO timestamps are stored or rendered — unknown end times are never fabricated into precise values. Precision `day` truncates to UTC midnight. Promotions already ended at submission are stored as expired; a ticker sweep archives rows whose `ends_at` passes after creation.

Sources: whitelisted kinds (official_blog, pricing_page, announcement, rss, user_submit). Official kinds are trusted (confidence high) and admin-gated; user submissions are medium. RSS/Atom ingestion reuses the shared dedup path; feed dates are recorded as day precision. Confidence is provenance-derived and rendered alongside every entry.

## Boundary read-grants (2026-09-30)

The ownership check distinguishes writes from reads. Writes (INSERT/UPDATE) to a protected table remain locked to its owning module with no exceptions. Reads (FROM/JOIN) of a protected table default to owner-only, with an explicit per-table reader whitelist in scripts/check-boundaries.py (`read_grants`): workspace_members may be read by conversation/config for shared-scope resolution; promotions/promotion_observations/devices/conversation_imports/workspace_events may be read by telemetry for the G1 observation report. New cross-domain reads require a new whitelist entry, not a silent query.
