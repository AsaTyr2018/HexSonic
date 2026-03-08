# HEXSONIC

[![Go](https://img.shields.io/badge/Go-1.22+-00ADD8?logo=go)](https://go.dev/)
[![Docker](https://img.shields.io/badge/Docker-Ready-2496ED?logo=docker)](https://www.docker.com/)
[![OIDC](https://img.shields.io/badge/Auth-OIDC%20%2F%20Keycloak-3B82F6)](https://www.keycloak.org/)
[![OpenSubsonic](https://img.shields.io/badge/API-OpenSubsonic-111827)](https://opensubsonic.netlify.app/)
[![Setup](https://img.shields.io/badge/Setup-One--Command-22C55E)](#quick-start-recommended)

Self-hosted multi-user music platform for modern creators, with strong support for AI music workflows.

HEXSONIC focuses on:
- upload and catalog management
- album/track metadata workflows
- private/public visibility control
- streaming + transcoding
- social features (ratings, comments, follows)
- role-based administration
- OpenSubsonic-compatible API endpoints

## Intended Use and Legal Notice

HEXSONIC is designed for hosting:
- AI-generated music
- original content you created
- content you are explicitly licensed to distribute

HEXSONIC is **not** intended for distributing copyrighted material without rights.

The operator of the deployed instance is solely responsible for:
- uploaded content
- copyright compliance
- applicable legal and regulatory obligations

Project contributors provide software only and do not assume liability for operator misuse.

## Why HEXSONIC

- Multi-user by design (user/member/mod/admin model)
- Creator-oriented upload and management flows
- Album + track ownership logic
- OIDC authentication (Keycloak)
- Admin observability (Grafana + Prometheus via app routes)
- One-command setup with randomized secrets

## Quick Start (Recommended)

Run the all-in-one installer:

```bash
./setup.sh
```

`setup.sh` will:
1. check/install required dependencies (Debian/Ubuntu)
2. generate random passwords/secrets
3. write `.env`
4. render Keycloak realm import with your generated values
5. build and start Docker services
6. print URLs and initial credentials

After completion, you get:
- Web UI URL
- Keycloak URL
- HEXSONIC admin login
- Keycloak bootstrap admin login

Credentials are also written to:
- `runtime/setup/initial-credentials.txt`

## Architecture (Docker Stack)

- `api` (Go HTTP API + integrated Web UI)
- `worker` (transcoding/background jobs)
- `postgres` (catalog + jobs + app data)
- `valkey` (cache/queues)
- `keycloak` (OIDC)
- `prometheus` (metrics)
- `grafana` (dashboards)
- `oauth2-proxy` (Prometheus auth bridge)

## Ports

Defaults (change in `.env`):
- `HEXSONIC_PUBLIC_PORT=18080`
- `KEYCLOAK_PUBLIC_PORT=18081`

Grafana and Prometheus are exposed through HEXSONIC routes:
- `/grafana/`
- `/prometheus/`

## Project Structure

- `cmd/hexsonic-api` API server entrypoint
- `cmd/hexsonic-worker` worker entrypoint
- `internal/` core backend modules
- `web/` Web UI assets
- `deploy/` docker/systemd/provisioning assets
- `scripts/` operational helpers
- `setup.sh` one-shot install/bootstrap

## Basic Operations

Start/refresh stack:

```bash
docker compose up -d --build
```

Stop stack:

```bash
docker compose down
```

View logs:

```bash
docker compose logs -f api worker
```

## systemd Autostart

Install compose autostart service:

```bash
sudo bash scripts/install_systemd.sh compose /opt/hexsonic
```

Service:
- `hexsonic-compose.service`

## OpenSubsonic Compatibility

HEXSONIC exposes OpenSubsonic-style endpoints (e.g. `/rest/...`) for client compatibility.

Notes:
- Some clients require legacy auth mode
- Some clients require explicit user/password login (not token-only)
- Compatibility coverage is broad but client behavior can vary

## Configuration

Use `.env.example` as reference for all supported variables.

Important:
- never commit real `.env` values
- never commit generated runtime artifacts
- rotate credentials when moving environments

## Security Checklist (Before Internet Exposure)

1. Put HEXSONIC behind a TLS reverse proxy (recommended: [DomNexDomain](https://github.com/AsaTyr2018/DomNexDomain) for simpler secure edge handling)
2. Restrict server firewall to required ports
3. Keep host + container images updated
4. Back up Postgres and runtime storage regularly
5. Keep registration settings and role grants under control
6. Audit admin logs and auth events periodically

## Development

Build:

```bash
make build
```

Test:

```bash
make test
```

Lint:

```bash
make lint
```

Smoke test:

```bash
./scripts/smoke_test.sh
```

Authenticated smoke test:

```bash
AUTH_TOKEN="<ACCESS_TOKEN>" ./scripts/smoke_test.sh
```

## Community Standards

- [Code of Conduct](CODE_OF_CONDUCT.md)
- [Contributing Guide](CONTRIBUTING.md)
- [Security Policy](SECURITY.md)
- [Support](SUPPORT.md)

## Maintainer

- AsaTyr2018 (`hauke.lenz@lenz-service.de`)

## License

Licensed under the Mozilla Public License 2.0 (MPL-2.0).  
See [LICENSE](LICENSE).
