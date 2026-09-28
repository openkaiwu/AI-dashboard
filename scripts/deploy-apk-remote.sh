#!/usr/bin/env bash
set -euo pipefail
sudo mkdir -p /var/www/aihub-downloads
if [[ -f /home/ubuntu/aihub-mobile.apk ]]; then
  sudo mv /home/ubuntu/aihub-mobile.apk /var/www/aihub-downloads/aihub-mobile.apk
  sudo chmod 644 /var/www/aihub-downloads/aihub-mobile.apk
fi

if [[ -f /home/ubuntu/nginx-aihub.conf ]]; then
  sudo cp /home/ubuntu/nginx-aihub.conf /etc/nginx/sites-available/aihub
else
  bash /home/ubuntu/fix-nginx-downloads.sh
fi
rm -f /home/ubuntu/nginx-downloads.conf /home/ubuntu/nginx-aihub.conf
sudo nginx -t
sudo systemctl reload nginx
ls -lh /var/www/aihub-downloads/aihub-mobile.apk
