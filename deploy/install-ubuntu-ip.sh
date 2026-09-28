#!/usr/bin/env bash
# Install the packaged AI Hub server on a fresh Ubuntu host with a public IPv4.
# Usage: sudo bash install-ubuntu-ip.sh <public-ipv4> <aihub-m0-runtime.zip>
set -euo pipefail

if [[ $EUID -ne 0 || $# -ne 2 ]]; then
  echo 'Usage: sudo bash install-ubuntu-ip.sh <public-ipv4> <aihub-m0-runtime.zip>' >&2
  exit 2
fi

ip=$1
archive=$2
python3 - "$ip" <<'PY'
import ipaddress, sys
address = ipaddress.ip_address(sys.argv[1])
if address.version != 4 or not address.is_global:
    raise SystemExit('A public IPv4 address is required')
PY
[[ -f $archive ]] || { echo "Package not found: $archive" >&2; exit 2; }
if [[ -e /etc/aihub.env ]] && ! grep -Fxq "AIHUB_ALLOWED_ORIGINS=https://${ip}" /etc/aihub.env; then
  echo 'An AI Hub installation for a different address already exists; refusing to change it' >&2
  exit 2
fi

export DEBIAN_FRONTEND=noninteractive
apt-get update
apt-get install -y postgresql nginx python3-venv unzip curl openssl
systemctl enable --now postgresql
unzip -tq "$archive" >/dev/null
id aihub >/dev/null 2>&1 || useradd --system --home-dir /opt/aihub --shell /usr/sbin/nologin aihub
install -d -m 0755 /opt/aihub
unzip -p "$archive" aihub-m0/aihub-linux-amd64 | install -m 0755 /dev/stdin /opt/aihub/aihub-linux-amd64
unzip -p "$archive" aihub-m0/aihub-admin-linux-amd64 | install -m 0755 /dev/stdin /opt/aihub/aihub-admin-linux-amd64
unzip -p "$archive" aihub-m0/aihub.service | install -m 0644 /dev/stdin /etc/systemd/system/aihub.service

if [[ ! -e /etc/aihub.env ]]; then
  db_password=$(openssl rand -hex 24)
  runuser -u postgres -- psql -v ON_ERROR_STOP=1 <<SQL
CREATE ROLE aihub LOGIN PASSWORD '$db_password';
CREATE DATABASE aihub OWNER aihub;
SQL
  umask 077
  cat > /etc/aihub.env <<EOF
AIHUB_ADDR=127.0.0.1:8080
AIHUB_DATABASE_URL=postgres://aihub:${db_password}@127.0.0.1:5432/aihub?sslmode=disable
AIHUB_ALLOWED_ORIGINS=https://${ip}
AIHUB_DEMO=false
EOF
  chmod 0600 /etc/aihub.env
fi
systemctl daemon-reload
systemctl enable --now aihub
systemctl restart aihub
curl --fail --silent --show-error --retry 12 --retry-connrefused --retry-delay 2 http://127.0.0.1:8080/ready >/dev/null

# Expose only ACME challenges until a publicly trusted certificate exists.
cert_dir="/etc/letsencrypt/live/${ip}"
install -d -m 0755 /var/www/letsencrypt/.well-known/acme-challenge
if [[ ! -s $cert_dir/fullchain.pem || ! -s $cert_dir/privkey.pem ]]; then
  cat > /etc/nginx/sites-available/aihub <<EOF
server {
    listen 80;
    server_name ${ip};
    location ^~ /.well-known/acme-challenge/ {
        root /var/www/letsencrypt;
    }
    location / { return 404; }
}
EOF
fi
if [[ -L /etc/nginx/sites-enabled/default ]]; then unlink /etc/nginx/sites-enabled/default; fi
ln -sfn /etc/nginx/sites-available/aihub /etc/nginx/sites-enabled/aihub
nginx -t
systemctl enable --now nginx
systemctl reload nginx

# Certbot 5.4+ supports webroot validation for IP certificates. The IP profile
# is short-lived, so renewal must run more often than for normal domain certs.
python3 -m venv /opt/aihub-certbot
/opt/aihub-certbot/bin/pip install --upgrade 'certbot>=5.4,<7'
if [[ ! -s $cert_dir/fullchain.pem || ! -s $cert_dir/privkey.pem ]]; then
  /opt/aihub-certbot/bin/certbot certonly \
    --webroot --webroot-path /var/www/letsencrypt \
    --preferred-profile shortlived --ip-address "$ip" \
    --non-interactive --agree-tos --register-unsafely-without-email
fi
[[ -s $cert_dir/fullchain.pem && -s $cert_dir/privkey.pem ]] || {
  echo "Certbot did not create the expected certificate in $cert_dir" >&2
  exit 1
}

cat > /etc/nginx/sites-available/aihub <<EOF
server {
    listen 80;
    server_name ${ip};
    location ^~ /.well-known/acme-challenge/ {
        root /var/www/letsencrypt;
    }
    location / { return 301 https://${ip}\$request_uri; }
}
server {
    listen 443 ssl;
    server_name ${ip};
    ssl_certificate ${cert_dir}/fullchain.pem;
    ssl_certificate_key ${cert_dir}/privkey.pem;
    location / {
        proxy_pass http://127.0.0.1:8080;
        proxy_set_header Host ${ip};
        proxy_set_header X-Real-IP \$remote_addr;
        proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto https;
    }
}
EOF
nginx -t
systemctl reload nginx

cat > /etc/systemd/system/aihub-cert-renew.service <<'EOF'
[Unit]
Description=Renew AI Hub IP certificate
[Service]
Type=oneshot
ExecStart=/opt/aihub-certbot/bin/certbot renew --quiet --deploy-hook "systemctl reload nginx"
EOF
cat > /etc/systemd/system/aihub-cert-renew.timer <<'EOF'
[Unit]
Description=Check AI Hub IP certificate twice daily
[Timer]
OnCalendar=*-*-* 03,15:00:00
Persistent=true
RandomizedDelaySec=30m
[Install]
WantedBy=timers.target
EOF
systemctl daemon-reload
systemctl enable --now aihub-cert-renew.timer
curl --fail --silent --show-error --connect-to "${ip}:443:127.0.0.1:443" "https://${ip}/ready" >/dev/null
echo "AI Hub is running at https://${ip}/"
