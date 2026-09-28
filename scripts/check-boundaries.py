from pathlib import Path
import re
root = Path(__file__).resolve().parents[1] / 'server'
allowed = {
 'auth': {'audit','db','httpx'},
 'sync': {'auth','db','httpx'},
 'jobs': {'httpx'},
 'audit': set(), 'db': set(), 'httpx': set(),
 'quota': set(), 'notification': {'quota'}, 'provider': set(), 'connector': {'auth','codex','cursor','httpx','quota'}, 'codex': set(), 'cursor': set(),
}
errors=[]
for p in (root/'internal').rglob('*.go'):
 if p.name.endswith('_test.go'): continue
 text=p.read_text(encoding='utf-8');owner=p.relative_to(root/'internal').parts[0]
 for target in re.findall(r'"aihub.dev/server/internal/([^"/]+)',text):
  if owner in allowed and target!=owner and target not in allowed[owner]: errors.append(f'{p}: disallowed {owner} -> {target}')
 if 'modernc.org/sqlite' in text: errors.append(f'{p}: SQLite server forbidden')
 for sql in re.findall(r'`([^`]+)`',text):
  tables=set(re.findall(r'\b(?:INTO|UPDATE|FROM|JOIN)\s+([a-z_]+)',sql,re.I))
  if tables & {'sync_notes','sync_events','sync_streams','applied_operations'} and owner!='sync': errors.append(f'{p}: sync ownership')
  if tables & {'devices','sessions'} and owner!='auth': errors.append(f'{p}: auth ownership')
if errors: raise SystemExit('\n'.join(errors))
print('Module dependencies and auth/sync table ownership: PASS')
