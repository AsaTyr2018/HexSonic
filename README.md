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

## Feature Breakdown

### Multi-User and Access Control
- OIDC-based login (Keycloak)
- Hierarchical roles: `user`, `member`, `moderator`, `admin`
- Creator badge gate for upload rights
- Owner-based management boundaries (owner + admin override)
- Persistent login with refresh-token flow

### Registration and Invite Flow
- Global registration toggle (admin-controlled)
- Invite-link based onboarding (works even if public registration is disabled)
- Invite lifecycle management (create/list/revoke/expiry)
- Dedicated invite registration entry flow (`/register?invite=...`)

### Library and Visibility Model
- Album and track management separated
- Public/private visibility per album and per track
- Hierarchical behavior for album visibility (album-level public propagation)
- Metadata edit support (title, artist, album, genre, covers, lyrics)

### Social and Community Features
- Album comments
- Track rating (stars)
- Public user profiles with follow support
- Uploader attribution in library views

### Discovery and Recommendations
- Dedicated Discovery landing page
- Global discovery modules (`Top Songs`, `Trending`, `Top Albums`)
- Personal discovery for logged-in users (`For You`)
- Recommendation scoring based on listening behavior, ratings, playlist adds, and skips
- Discovery-oriented playback source tracking
- `Jukebox` beta mode with adaptive rolling queue, live feedback controls, and personal radio-style playback

### Creator Analytics
- Dedicated `Creator Stats` view in the WebUI
- Plays, unique listeners, qualified listens, completed listens, playlist adds, and rating metrics
- Top tracks and top albums by creator
- Traffic-source visibility for creator uploads
- Time-window based analytics (`24h`, `7d`, `30d`, `90d`, `all`)

### Playback and Client Compatibility
- Web player + advanced popout player
- Playlist queue handling with direct track jump
- Background transcoding pipeline (derived formats for broader client support)
- OpenSubsonic-compatible API for external clients

### Admin and Operations
- User administration (roles, moderation, account actions)
- Registration control and invite administration
- Admin log/debug views in WebUI
- Prometheus and Grafana integration (proxied through HEXSONIC)
- Docker-first deployment with systemd autostart support

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
- Built-in discovery landing page with personal + global recommendation layers
- Beta `Jukebox` engine for personalized radio-style playback
- Creator-facing analytics for uploaded content
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


## Screenshots
<img width="1911" height="886" alt="grafik" src="https://github.com/user-attachments/assets/a802e5cc-35e6-44cc-800d-c7f4ccae7cdc" />
<img width="1657" height="621" alt="grafik" src="https://github.com/user-attachments/assets/5e08b7f9-169a-4e60-b21d-a7b0f2c2df44" />
<img width="1914" height="877" alt="grafik" src="https://github.com/user-attachments/assets/99b59d85-6926-4f06-bfe1-4a72fe8e239c" />
<img width="1058" height="744" alt="grafik" src="https://github.com/user-attachments/assets/f1910395-357c-4957-8793-418d303c756b" />
<img width="1909" height="878" alt="grafik" src="https://github.com/user-attachments/assets/2f553ccb-6515-42d4-84cd-103fb1201c46" />
<img width="1673" height="692" alt="grafik" src="https://github.com/user-attachments/assets/3f183055-ec4f-4695-a40b-19a67803f846" />
<img width="1732" height="793" alt="grafik" src="https://github.com/user-attachments/assets/8b99477e-4480-4c3b-8ee2-378b552d023c" />




