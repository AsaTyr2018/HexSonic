#!/usr/bin/env bash
set -euo pipefail

MODE="${1:-compose}"
PROJECT_DIR="${2:-/opt/hexsonic}"

if [[ "$EUID" -ne 0 ]]; then
  echo "Run as root"
  exit 1
fi

mkdir -p /etc/hexsonic

if [[ "$MODE" == "compose" ]]; then
  install -m 0644 "$PROJECT_DIR/deploy/systemd/hexsonic-compose.service" /etc/systemd/system/hexsonic-compose.service
  systemctl daemon-reload
  systemctl enable --now hexsonic-compose.service
  systemctl status hexsonic-compose.service --no-pager
  exit 0
fi

if [[ "$MODE" == "native" ]]; then
  install -m 0644 "$PROJECT_DIR/deploy/systemd/hexsonic-api.service" /etc/systemd/system/hexsonic-api.service
  install -m 0644 "$PROJECT_DIR/deploy/systemd/hexsonic-worker.service" /etc/systemd/system/hexsonic-worker.service

  if ! id -u hexsonic >/dev/null 2>&1; then
    useradd --system --home /var/lib/hexsonic --shell /usr/sbin/nologin hexsonic
  fi

  mkdir -p /var/lib/hexsonic/data
  chown -R hexsonic:hexsonic /var/lib/hexsonic

  if [[ ! -f /etc/hexsonic/hexsonic.env ]]; then
    cp "$PROJECT_DIR/.env.example" /etc/hexsonic/hexsonic.env
    chmod 600 /etc/hexsonic/hexsonic.env
    chown hexsonic:hexsonic /etc/hexsonic/hexsonic.env
    echo "Created /etc/hexsonic/hexsonic.env. Update HEXSONIC_SIGNING_KEY before production use."
  fi

  systemctl daemon-reload
  systemctl enable --now hexsonic-api.service
  systemctl enable --now hexsonic-worker.service
  systemctl status hexsonic-api.service --no-pager
  systemctl status hexsonic-worker.service --no-pager
  exit 0
fi

echo "Usage: $0 [compose|native] [project_dir]"
exit 1
