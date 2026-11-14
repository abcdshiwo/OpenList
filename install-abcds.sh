#!/usr/bin/env bash
# OpenList (abcds fork) one-click installer. Defaults to PostgreSQL (Docker).
# Usage: install-abcds.sh --binary /path/to/openlist [--name openlist] [--port 5244]
#        [--data-dir /opt/openlist] [--url URL] [--domain example.com] [--nginx]
set -euo pipefail

NAME=openlist; HTTP_PORT=5244; DATA_DIR=""; PG_PORT=5432; BINARY=""; URL=""; DOMAIN=""; WITH_NGINX=0
log(){ echo "[install] $*"; }
die(){ echo "[error] $*" >&2; exit 1; }
usage(){ grep '^#' "$0" | head -5; exit 0; }

while [ $# -gt 0 ]; do case "$1" in
  --name) NAME="$2"; shift 2;;
  --port) HTTP_PORT="$2"; shift 2;;
  --data-dir) DATA_DIR="$2"; shift 2;;
  --binary) BINARY="$2"; shift 2;;
  --url) URL="$2"; shift 2;;
  --domain) DOMAIN="$2"; shift 2;;
  --nginx) WITH_NGINX=1; shift;;
  --pg-port) PG_PORT="$2"; shift 2;;
  -h|--help) usage;;
  *) die "unknown arg: $1";;
esac; done

[ "$(id -u)" = 0 ] || die "must run as root"
DATA_DIR="${DATA_DIR:-/opt/$NAME}"
PGC="$NAME-pg"; SVC="$NAME"
command -v curl >/dev/null 2>&1 || die "curl is required"
command -v openssl >/dev/null 2>&1 || die "openssl is required"

# --- Docker ---
if ! command -v docker >/dev/null 2>&1; then
  log "docker not found, installing..."
  if command -v apt-get >/dev/null 2>&1; then apt-get update -qq && apt-get install -y -qq docker.io; 
  else die "docker not found; please install Docker first"; fi
fi
systemctl enable --now docker >/dev/null 2>&1 || true

# --- PostgreSQL (Docker) ---
mkdir -p "$DATA_DIR"
if docker ps -a --format '{{.Names}}' | grep -qx "$PGC"; then
  log "reusing existing postgres container: $PGC"
  docker start "$PGC" >/dev/null 2>&1 || true
  PGPASS=$(grep '^PASSWORD=' "$DATA_DIR/pg_credentials" 2>/dev/null | cut -d= -f2 || true)
  [ -n "$PGPASS" ] || die "container exists but no saved password in $DATA_DIR/pg_credentials"
else
  PGPASS=$(openssl rand -hex 16)
  echo "PASSWORD=$PGPASS" > "$DATA_DIR/pg_credentials"; chmod 600 "$DATA_DIR/pg_credentials"
  log "starting postgres container: $PGC"
  docker run -d --name "$PGC" --restart unless-stopped \
    -e POSTGRES_USER=openlist -e POSTGRES_PASSWORD="$PGPASS" -e POSTGRES_DB=openlist \
    -e PGDATA=/var/lib/postgresql/data/pgdata \
    -v "$DATA_DIR/pgdata:/var/lib/postgresql/data" \
    -p "127.0.0.1:$PG_PORT:5432" \
    postgres:16-alpine \
    -c shared_buffers=256MB -c work_mem=32MB -c maintenance_work_mem=256MB -c max_connections=100 >/dev/null
fi
log "waiting for postgres..."
for i in $(seq 1 30); do docker exec "$PGC" pg_isready -U openlist >/dev/null 2>&1 && break; sleep 2; done
docker exec "$PGC" pg_isready -U openlist >/dev/null 2>&1 || die "postgres not ready"

# --- OpenList binary ---
TMPBIN=$(mktemp)
if [ -n "$BINARY" ]; then [ -f "$BINARY" ] || die "binary not found: $BINARY"; cp "$BINARY" "$TMPBIN";
elif [ -n "$URL" ]; then log "downloading binary: $URL"; curl -fsSL "$URL" -o "$TMPBIN";
else die "provide --binary PATH or --url URL (prebuilt linux/amd64 OpenList binary)"; fi
install -m755 "$TMPBIN" "$DATA_DIR/openlist"; rm -f "$TMPBIN"

# --- config (partial; OpenList merges defaults on first run) ---
if [ -f "$DATA_DIR/config.json" ]; then cp "$DATA_DIR/config.json" "$DATA_DIR/config.json.bak.$(date +%s)"; fi
cat > "$DATA_DIR/config.json" <<EOF
{
  "database": { "type": "postgres", "host": "127.0.0.1", "port": $PG_PORT, "user": "openlist", "password": "$PGPASS", "name": "openlist", "table_prefix": "x_", "ssl_mode": "disable" },
  "scheme": { "address": "127.0.0.1", "http_port": $HTTP_PORT, "https_port": -1 },
  "dist_dir": ""
}
EOF

# --- systemd ---
cat > "/etc/systemd/system/$SVC.service" <<EOF
[Unit]
Description=OpenList ($NAME, abcds fork, PostgreSQL)
After=network.target docker.service
Wants=docker.service

[Service]
Type=simple
WorkingDirectory=$DATA_DIR
ExecStart=$DATA_DIR/openlist server --data $DATA_DIR
Restart=on-failure
RestartSec=5

[Install]
WantedBy=multi-user.target
EOF
systemctl daemon-reload
systemctl enable --now "$SVC" >/dev/null 2>&1

# --- verify ---
code=0
for i in $(seq 1 30); do code=$(curl -s -o /dev/null -w '%{http_code}' "http://127.0.0.1:$HTTP_PORT/ping" 2>/dev/null || true); [ "$code" = 200 ] && break; sleep 1; done
if [ "$code" != 200 ]; then journalctl -u "$SVC" -n 30 --no-pager; die "service failed to start"; fi
ADMINPWD=$(journalctl -u "$SVC" --no-pager | grep -o 'initial password is: .*' | tail -1 | sed 's/.*initial password is: //' | tr -d '[:space:]' || true)

log "OK: OpenList ($NAME) is running on 127.0.0.1:$HTTP_PORT with PostgreSQL"
log "data dir : $DATA_DIR"
log "pg creds : $DATA_DIR/pg_credentials"
log "admin    : admin / ${ADMINPWD:-<run: journalctl -u $SVC | grep password>}"

# --- optional nginx reverse proxy ---
if [ "$WITH_NGINX" = 1 ] && [ -n "$DOMAIN" ]; then
  if [ -d /etc/nginx/conf.d ]; then
    cat > "/etc/nginx/conf.d/$DOMAIN.conf" <<EOF
server {
    listen 80;
    server_name $DOMAIN;
    client_max_body_size 0;
    location / {
        proxy_pass http://127.0.0.1:$HTTP_PORT;
        proxy_set_header Host \$host;
        proxy_set_header X-Real-IP \$remote_addr;
        proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto \$scheme;
        proxy_buffering off;
        proxy_request_buffering off;
    }
}
EOF
    nginx -t >/dev/null 2>&1 && nginx -s reload >/dev/null 2>&1 && log "nginx reverse proxy configured for $DOMAIN (add TLS via acme.sh)" || log "nginx conf written but reload failed; check nginx -t"
  else
    log "nginx not found at /etc/nginx; reverse-proxy $DOMAIN -> 127.0.0.1:$HTTP_PORT manually, then add TLS"
  fi
fi
