#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
RUNTIME_DIR="${ROOT_DIR}/runtime"
ENV_FILE="${ROOT_DIR}/.env"
ENV_EXAMPLE="${ROOT_DIR}/.env.example"
REALM_TEMPLATE="${ROOT_DIR}/deploy/keycloak/realm-hexsonic.template.json"
REALM_RENDERED="${ROOT_DIR}/runtime/keycloak-import/realm-hexsonic.json"
CREDENTIALS_FILE="${ROOT_DIR}/runtime/setup/initial-credentials.txt"

log() { printf '[setup] %s\n' "$*"; }
die() { printf '[setup][error] %s\n' "$*" >&2; exit 1; }

choose_compose_cmd() {
  if docker compose version >/dev/null 2>&1; then
    COMPOSE_CMD=(docker compose)
  elif command -v docker-compose >/dev/null 2>&1; then
    COMPOSE_CMD=(docker-compose)
  else
    die "Docker Compose not found."
  fi
}

is_debian_like() {
  [[ -f /etc/debian_version ]]
}

install_deps_if_missing() {
  local missing=()
  local cmds=(docker openssl curl awk sed ip)
  local cmd
  for cmd in "${cmds[@]}"; do
    command -v "$cmd" >/dev/null 2>&1 || missing+=("$cmd")
  done
  if docker compose version >/dev/null 2>&1 || command -v docker-compose >/dev/null 2>&1; then
    :
  else
    missing+=("docker-compose")
  fi

  if [[ "${#missing[@]}" -eq 0 ]]; then
    return
  fi

  if ! is_debian_like; then
    die "Missing dependencies: ${missing[*]} (automatic install currently supports Debian/Ubuntu only)."
  fi
  if [[ "${EUID:-$(id -u)}" -ne 0 ]]; then
    die "Missing dependencies: ${missing[*]}. Re-run setup as root for auto-install."
  fi

  log "Installing missing dependencies: ${missing[*]}"
  apt-get update -y
  apt-get install -y --no-install-recommends \
    ca-certificates curl openssl iproute2 gawk sed jq docker.io docker-compose-plugin
}

escape_sed() {
  printf '%s' "$1" | sed -e 's/[\/&]/\\&/g'
}

rand_alnum() {
  local length="$1"
  openssl rand -base64 96 | tr -dc 'A-Za-z0-9' | head -c "$length"
}

rand_hex() {
  local bytes="$1"
  openssl rand -hex "$bytes"
}

rand_b64url_32() {
  openssl rand -base64 32 | tr '+/' '-_' | tr -d '='
}

detect_lan_ip() {
  local ip_addr
  ip_addr="$(ip route get 1.1.1.1 2>/dev/null | awk '/src/ {for(i=1;i<=NF;i++) if($i=="src"){print $(i+1); exit}}')"
  if [[ -z "${ip_addr}" ]]; then
    ip_addr="$(hostname -I 2>/dev/null | awk '{print $1}')"
  fi
  [[ -n "${ip_addr}" ]] || ip_addr="127.0.0.1"
  printf '%s' "$ip_addr"
}

ensure_files() {
  [[ -f "${ENV_EXAMPLE}" ]] || die ".env.example not found"
  [[ -f "${REALM_TEMPLATE}" ]] || die "Keycloak realm template not found"
  mkdir -p "${RUNTIME_DIR}/keycloak-import" "${RUNTIME_DIR}/setup"
}

write_env_file() {
  if [[ -f "${ENV_FILE}" ]]; then
    local backup="${ENV_FILE}.bak.$(date +%s)"
    cp "${ENV_FILE}" "${backup}"
    log "Existing .env backed up to ${backup}"
  fi

  cat > "${ENV_FILE}" <<EOF
HEXSONIC_HTTP_ADDR=:8080
HEXSONIC_DB_PASSWORD=${HEXSONIC_DB_PASSWORD}
HEXSONIC_DATABASE_URL=postgres://hexsonic:${HEXSONIC_DB_PASSWORD}@postgres:5432/hexsonic?sslmode=disable
HEXSONIC_REDIS_ADDR=valkey:6379
HEXSONIC_REDIS_PASSWORD=
HEXSONIC_REDIS_DB=0
HEXSONIC_STORAGE_ROOT=/data
HEXSONIC_TEMP_ROOT=/data/temp
HEXSONIC_SIGNING_KEY=${HEXSONIC_SIGNING_KEY}
HEXSONIC_SUBSONIC_SECRET_KEY=${HEXSONIC_SUBSONIC_SECRET_KEY}
HEXSONIC_SIGNED_URL_TTL=15m
HEXSONIC_MAX_UPLOAD_BYTES=2147483648
HEXSONIC_FFMPEG_BIN=ffmpeg
HEXSONIC_FFPROBE_BIN=ffprobe
HEXSONIC_ENABLE_DERIVED_SYNC=false
HEXSONIC_AUTH_REQUIRED=true
HEXSONIC_OIDC_ISSUER_URL=http://keycloak:8080/realms/hexsonic
HEXSONIC_OIDC_BROWSER_ISSUER_URL=${HEXSONIC_OIDC_BROWSER_ISSUER_URL}
HEXSONIC_OIDC_AUDIENCE=
HEXSONIC_OIDC_CLIENT_ID=hexsonic-api
HEXSONIC_OIDC_CLIENT_SECRET=${HEXSONIC_OIDC_CLIENT_SECRET}
HEXSONIC_OIDC_ADMIN_USER=${HEXSONIC_OIDC_ADMIN_USER}
HEXSONIC_OIDC_ADMIN_PASSWORD=${HEXSONIC_OIDC_ADMIN_PASSWORD}
HEXSONIC_PROMETHEUS_URL=http://prometheus:9090/prometheus
HEXSONIC_GRAFANA_URL=http://grafana:3000
HEXSONIC_PROMETHEUS_PROXY_URL=http://oauth2-proxy:4180
HEXSONIC_GRAFANA_PROXY_URL=http://grafana:3000
HEXSONIC_PUBLIC_BASE_URL=${HEXSONIC_PUBLIC_BASE_URL}
KEYCLOAK_BOOTSTRAP_ADMIN_USERNAME=${KEYCLOAK_BOOTSTRAP_ADMIN_USERNAME}
KEYCLOAK_BOOTSTRAP_ADMIN_PASSWORD=${KEYCLOAK_BOOTSTRAP_ADMIN_PASSWORD}
KEYCLOAK_HOSTNAME=${KEYCLOAK_HOSTNAME}
OAUTH2_PROXY_COOKIE_SECRET=${OAUTH2_PROXY_COOKIE_SECRET}
OAUTH2_PROXY_COOKIE_SECURE=true
HEXSONIC_PUBLIC_PORT=${HEXSONIC_PUBLIC_PORT}
KEYCLOAK_PUBLIC_PORT=${KEYCLOAK_PUBLIC_PORT}
EOF
  chmod 600 "${ENV_FILE}"
}

render_realm_import() {
  cp "${REALM_TEMPLATE}" "${REALM_RENDERED}"
  sed -i \
    -e "s/__HEXSONIC_OIDC_CLIENT_SECRET__/$(escape_sed "${HEXSONIC_OIDC_CLIENT_SECRET}")/g" \
    -e "s/__HEXSONIC_ADMIN_USERNAME__/$(escape_sed "${HEXSONIC_ADMIN_USERNAME}")/g" \
    -e "s/__HEXSONIC_ADMIN_PASSWORD__/$(escape_sed "${HEXSONIC_ADMIN_PASSWORD}")/g" \
    -e "s/__HEXSONIC_ADMIN_EMAIL__/$(escape_sed "${HEXSONIC_ADMIN_EMAIL}")/g" \
    "${REALM_RENDERED}"
  chmod 600 "${REALM_RENDERED}"
}

start_stack() {
  log "Building and starting docker compose stack"
  (cd "${ROOT_DIR}" && "${COMPOSE_CMD[@]}" up -d --build)
}

wait_for_api() {
  local health_url="http://127.0.0.1:${HEXSONIC_PUBLIC_PORT}/healthz"
  local i
  log "Waiting for API health endpoint: ${health_url}"
  for i in $(seq 1 120); do
    if curl -fsS "${health_url}" >/dev/null 2>&1; then
      return 0
    fi
    sleep 2
  done
  return 1
}

write_credentials_summary() {
  cat > "${CREDENTIALS_FILE}" <<EOF
HEXSONIC setup completed at $(date -Is)

Web UI: ${HEXSONIC_PUBLIC_BASE_URL}
Keycloak: ${KEYCLOAK_HOSTNAME}
Grafana (via HEXSONIC): ${HEXSONIC_PUBLIC_BASE_URL}/grafana/
Prometheus (via HEXSONIC): ${HEXSONIC_PUBLIC_BASE_URL}/prometheus/

HEXSONIC Admin (realm user)
  username: ${HEXSONIC_ADMIN_USERNAME}
  password: ${HEXSONIC_ADMIN_PASSWORD}

Keycloak Bootstrap Admin (master realm)
  username: ${KEYCLOAK_BOOTSTRAP_ADMIN_USERNAME}
  password: ${KEYCLOAK_BOOTSTRAP_ADMIN_PASSWORD}

Stored files:
  .env: ${ENV_FILE}
  realm import: ${REALM_RENDERED}
EOF
  chmod 600 "${CREDENTIALS_FILE}"
}

main() {
  install_deps_if_missing
  choose_compose_cmd
  docker info >/dev/null 2>&1 || die "Docker daemon is not reachable."
  ensure_files

  local lan_ip
  lan_ip="$(detect_lan_ip)"
  HEXSONIC_PUBLIC_PORT="${HEXSONIC_PUBLIC_PORT:-18080}"
  KEYCLOAK_PUBLIC_PORT="${KEYCLOAK_PUBLIC_PORT:-18081}"
  HEXSONIC_PUBLIC_BASE_URL="${HEXSONIC_PUBLIC_BASE_URL:-http://${lan_ip}:${HEXSONIC_PUBLIC_PORT}}"
  KEYCLOAK_HOSTNAME="${KEYCLOAK_HOSTNAME:-http://${lan_ip}:${KEYCLOAK_PUBLIC_PORT}}"
  HEXSONIC_OIDC_BROWSER_ISSUER_URL="${HEXSONIC_OIDC_BROWSER_ISSUER_URL:-${KEYCLOAK_HOSTNAME}/realms/hexsonic}"

  HEXSONIC_DB_PASSWORD="${HEXSONIC_DB_PASSWORD:-$(rand_alnum 28)}"
  HEXSONIC_SIGNING_KEY="${HEXSONIC_SIGNING_KEY:-$(rand_hex 32)}"
  HEXSONIC_SUBSONIC_SECRET_KEY="${HEXSONIC_SUBSONIC_SECRET_KEY:-$(rand_hex 32)}"
  HEXSONIC_OIDC_CLIENT_SECRET="${HEXSONIC_OIDC_CLIENT_SECRET:-$(rand_alnum 40)}"
  OAUTH2_PROXY_COOKIE_SECRET="${OAUTH2_PROXY_COOKIE_SECRET:-$(rand_b64url_32)}"

  KEYCLOAK_BOOTSTRAP_ADMIN_USERNAME="${KEYCLOAK_BOOTSTRAP_ADMIN_USERNAME:-kcadmin}"
  KEYCLOAK_BOOTSTRAP_ADMIN_PASSWORD="${KEYCLOAK_BOOTSTRAP_ADMIN_PASSWORD:-$(rand_alnum 24)}"
  HEXSONIC_OIDC_ADMIN_USER="${HEXSONIC_OIDC_ADMIN_USER:-${KEYCLOAK_BOOTSTRAP_ADMIN_USERNAME}}"
  HEXSONIC_OIDC_ADMIN_PASSWORD="${HEXSONIC_OIDC_ADMIN_PASSWORD:-${KEYCLOAK_BOOTSTRAP_ADMIN_PASSWORD}}"

  HEXSONIC_ADMIN_USERNAME="${HEXSONIC_ADMIN_USERNAME:-admin}"
  HEXSONIC_ADMIN_PASSWORD="${HEXSONIC_ADMIN_PASSWORD:-$(rand_alnum 24)}"
  HEXSONIC_ADMIN_EMAIL="${HEXSONIC_ADMIN_EMAIL:-admin@hexsonic.local}"

  write_env_file
  render_realm_import
  start_stack

  if wait_for_api; then
    log "API is healthy."
  else
    log "API did not become healthy in time. Check logs with: ${COMPOSE_CMD[*]} logs -f api"
  fi

  write_credentials_summary

  printf '\n'
  printf 'HEXSONIC setup complete.\n'
  printf 'Web UI: %s\n' "${HEXSONIC_PUBLIC_BASE_URL}"
  printf 'Keycloak: %s\n' "${KEYCLOAK_HOSTNAME}"
  printf '\n'
  printf 'HEXSONIC Admin:\n'
  printf '  username: %s\n' "${HEXSONIC_ADMIN_USERNAME}"
  printf '  password: %s\n' "${HEXSONIC_ADMIN_PASSWORD}"
  printf '\n'
  printf 'Keycloak Bootstrap Admin:\n'
  printf '  username: %s\n' "${KEYCLOAK_BOOTSTRAP_ADMIN_USERNAME}"
  printf '  password: %s\n' "${KEYCLOAK_BOOTSTRAP_ADMIN_PASSWORD}"
  printf '\n'
  printf 'Credentials file: %s\n' "${CREDENTIALS_FILE}"
}

main "$@"
