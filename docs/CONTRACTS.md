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

POST /api/v1/auth/register or /login: {email,password,device_name}.
Returns {token,refresh_token,expires_at,device_id,user:{id,email}}.
Each login creates a new device. Passwords: bcrypt cost 10, 8–72 UTF-8 bytes.
Access: 192 random bits, 15 minutes. Refresh: independently random, 30 days.
Database stores only SHA-256 token hashes. Provider credentials are unrelated and never accepted here.

POST /api/v1/auth/refresh: {refresh_token}. Rotates both tokens in one transaction.
Old access is invalid after rotation. Used refresh hashes remain as replay detectors.
Reusing a consumed refresh revokes that device, including its newer sessions; other devices are unaffected.
A lost refresh response may require fresh login; never silently weaken replay protection.

GET /api/v1/devices; DELETE /api/v1/devices/{id}; POST /api/v1/auth/logout.
Revocation is scoped to the authenticated owner. Logout revokes the current device.
Every protected request checks persistence; sync writes also lock the device row inside the transaction.
Requests already in flight may finish before a revocation commits. No new request after the revoke commit is accepted.
Audit contains user/device IDs and action names, not credentials, note payloads or passwords.
Authentication endpoints use a bounded in-memory IP throttle; reverse proxies should add edge rate limiting.

| Threat | Expected behavior |
| --- | --- |
| stolen access | short lifetime; owner can revoke the device immediately |
| stolen refresh/replay | token rotation; reuse revokes the device family |
| lost phone | revoke from another device; cannot remotely erase already cached offline data |
| server URL spoof/redirect | HTTPS and normal certificate verification; no userinfo/path/query in Server Profile; HTTP redirects refused |
| cross-profile credentials | vault/session keyed by profile; changing URL clears its session; cache keyed by origin and user |
| revoked session | queue retained; automatic network retry stops until fresh login |
| XSS | React escaped text, no HTML injection, same-origin deployment; Web system tokens live in origin localStorage and remain exposed to origin compromise |
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
