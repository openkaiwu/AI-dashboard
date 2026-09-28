#!/usr/bin/env bash
set -euo pipefail
sudo cp /home/ubuntu/nginx-aihub.conf /etc/nginx/sites-available/aihub
sudo cp /home/ubuntu/downloads-index.html /var/www/aihub-downloads/index.html
sudo chmod 644 /var/www/aihub-downloads/index.html /var/www/aihub-downloads/aihub-mobile.apk
sudo nginx -t
sudo systemctl reload nginx
echo '--- listeners ---'
sudo ss -tlnp | grep -E ':80|:443|:8888' || true
echo '--- local tests ---'
curl -sI http://127.0.0.1/downloads/aihub-mobile.apk | head -3
curl -sI http://127.0.0.1:8888/downloads/aihub-mobile.apk | head -3
