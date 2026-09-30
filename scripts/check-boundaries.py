from pathlib import Path
import re
root = Path(__file__).resolve().parents[1] / 'server'
allowed = {
 'auth': {'audit','db','httpx'},
 'sync': {'auth','db','httpx'},
 'jobs': {'httpx'},
 'audit': set(), 'db': set(), 'httpx': set(),
 'quota': set(), 'notification': {'quota'}, 'provider': set(),
 'connector': {'auth','codex','cursor','official','httpx','quota','config'},
 'codex': set(), 'cursor': set(),
 'conversation': {'auth','db','httpx','jobs','workspace'},
 'config': {'auth','db','httpx','workspace'},
 'workspace': {'auth','db','httpx','audit'},
 'promotion': {'auth','db','httpx'},
 'telemetry': set(),
}
errors=[]
for p in (root/'internal').rglob('*.go'):
 if p.name.endswith('_test.go'): continue
 text=p.read_text(encoding='utf-8');owner=p.relative_to(root/'internal').parts[0]
 for target in re.findall(r'"aihub.dev/server/internal/([^"/]+)',text):
  if owner in allowed and target!=owner and target not in allowed[owner]: errors.append(f'{p}: disallowed {owner} -> {target}')
 if 'modernc.org/sqlite' in text: errors.append(f'{p}: SQLite server forbidden')
 for sql in re.findall(r'`([^`]+)`',text):
  # Writes (INTO/UPDATE) stay locked to the owning module; reads (FROM/JOIN) may be
  # granted to explicitly whitelisted reader modules (cross-domain aggregation).
  write_owner={'sync_notes':'sync','sync_events':'sync','sync_streams':'sync','applied_operations':'sync','devices':'auth','sessions':'auth','projects':'conversation','conversations':'conversation','conversation_branches':'conversation','conversation_messages':'conversation','conversation_imports':'conversation','conversation_raw_snapshots':'conversation','config_assets':'config','config_versions':'config','config_bindings':'config','config_discoveries':'config','workspaces':'workspace','workspace_members':'workspace','workspace_invites':'workspace','workspace_comments':'workspace','workspace_events':'workspace','promotions':'promotion','promotion_sources':'promotion','promotion_observations':'promotion','promotion_watchlists':'promotion','promotion_notifications':'promotion'}
  read_grants={'workspace_members':{'conversation','config'},'promotions':{'telemetry'},'promotion_observations':{'telemetry'},'devices':{'telemetry'},'conversation_imports':{'telemetry'},'workspace_events':{'telemetry'}}
  for kw,table in re.findall(r'\b(INTO|UPDATE|FROM|JOIN)\s+([a-z_]+)',sql,re.I):
    if table not in write_owner: continue
    if kw in ('INTO','UPDATE'):
      if owner!=write_owner[table]: errors.append(f'{p}: {table} write ownership')
    else:
      if owner!=write_owner[table] and owner not in read_grants.get(table,set()):
        errors.append(f'{p}: {table} read ownership')
if errors: raise SystemExit('\n'.join(errors))
print('Module dependencies and auth/sync table ownership: PASS')
