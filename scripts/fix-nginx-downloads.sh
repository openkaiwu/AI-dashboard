#!/usr/bin/env bash
set -euo pipefail
CONF=/etc/nginx/sites-available/aihub
if sudo awk '/listen 443/{f=1} f && /location \/downloads\//{found=1} END{exit !found}' "$CONF"; then
  echo "HTTPS downloads route already configured"
  exit 0
fi
sudo python3 - <<'PY'
from pathlib import Path
conf_path = Path('/etc/nginx/sites-available/aihub')
text = conf_path.read_text()
snippet = """    location /downloads/ {
        alias /var/www/aihub-downloads/;
        default_type application/vnd.android.package-archive;
        add_header Content-Disposition 'attachment; filename=\"aihub-mobile.apk\"';
    }
"""
marker = 'listen 443 ssl;'
if marker not in text:
    raise SystemExit('443 server block not found')
head, tail = text.split(marker, 1)
needle = '    location / {'
idx = tail.find(needle)
if idx < 0:
    raise SystemExit('proxy location block not found in 443 server')
Path('/etc/nginx/sites-available/aihub.new').write_text(head + marker + tail[:idx] + snippet + tail[idx:])
PY
sudo mv /etc/nginx/sites-available/aihub.new "$CONF"
sudo nginx -t
sudo systemctl reload nginx
echo "HTTPS downloads route installed"
