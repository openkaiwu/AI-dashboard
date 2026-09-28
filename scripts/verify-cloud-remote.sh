#!/usr/bin/env bash
set -euo pipefail
CONFIG="${1:-/home/ubuntu/verify-secrets.json}"
SERVER=$(python3 -c "import json;print(json.load(open('$CONFIG')).get('server','https://127.0.0.1'))")
EMAIL=$(python3 -c "import json;print(json.load(open('$CONFIG'))['email'])")
PASSWORD=$(python3 -c "import json;print(json.load(open('$CONFIG'))['password'])")
BRIDGE_TOKEN=$(python3 -c "import json; d=json.load(open('$CONFIG')); print(d.get('bridge_token',''))")

pass=0; fail=0
check(){ local name="$1" ok="$2" detail="$3"; if [[ "$ok" == "1" ]]; then echo "[PASS] $name - $detail"; pass=$((pass+1)); else echo "[FAIL] $name - $detail"; fail=$((fail+1)); fi; }

ready=$(curl -sk "$SERVER/ready")
check '/ready' "$(echo "$ready" | grep -q '"status":"ok"' && echo 1 || echo 0)" "$ready"
meta=$(curl -sk "$SERVER/api/v1/meta")
check '/api/v1/meta' "$(echo "$meta" | grep -q '"protocol":1' && echo 1 || echo 0)" "$meta"
login=$(curl -sk -X POST "$SERVER/api/v1/auth/login" -H 'Content-Type: application/json' -d "$(python3 -c "import json; print(json.dumps({'email':'$EMAIL','password':'$PASSWORD','device_name':'verify-remote'}))")")
token=$(python3 -c "import json,sys; print(json.load(sys.stdin).get('token',''))" <<<"$login" 2>/dev/null || true)
email=$(python3 -c "import json,sys; print(json.load(sys.stdin).get('user',{}).get('email',''))" <<<"$login" 2>/dev/null || true)
check 'login' "$([[ -n "$token" ]] && echo 1 || echo 0)" "$email"
if [[ -n "$token" ]]; then
  overview=$(curl -sk -H "Authorization: Bearer $token" "$SERVER/api/v1/codex/overview")
  check 'codex/overview' "$(echo "$overview" | grep -q 'generated_at' && echo 1 || echo 0)" "$(echo "$overview" | python3 -c "import json,sys; d=json.load(sys.stdin); print('alerts='+str(len(d.get('alerts',[]))))" 2>/dev/null || echo err)"
  bridges=$(curl -sk -H "Authorization: Bearer $token" "$SERVER/api/v1/codex/bridges")
  count=$(python3 -c "import json,sys; print(len(json.load(sys.stdin).get('bridges',[])))" <<<"$bridges" 2>/dev/null || echo 0)
  check 'codex/bridges' "1" "count=$count"
fi
if [[ -n "$token" ]]; then
  recent=$(python3 -c "import json,sys,datetime; d=json.loads(sys.argv[1]); now=datetime.datetime.now(datetime.timezone.utc); out='none';
for b in d.get('bridges',[]):
 ra=b.get('received_at');
 if ra and not b.get('revoked_at'):
  t=datetime.datetime.fromisoformat(ra.replace('Z','+00:00'));
  if (now-t).total_seconds()<86400: out=f\"{b.get('name')} {ra}\"; break
print(out)" "$bridges")
  check 'bridge/live-upload' "$([[ "$recent" != none ]] && echo 1 || echo 0)" "$recent"
fi
apk=$(curl -skI "$SERVER/downloads/aihub-mobile.apk" | tr -d '\r')
size=$(echo "$apk" | grep -i '^Content-Length:' | awk '{print $2}')
check 'apk/download' "$(echo "$apk" | grep -qi 'application/vnd.android.package-archive' && [[ "$size" == "58080630" ]] && echo 1 || echo 0)" "bytes=$size"
echo "Summary: $pass passed, $fail failed"
rm -f "$CONFIG"
[[ "$fail" -eq 0 ]]
