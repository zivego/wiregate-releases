# WireGate Releases

Published WireGate release manifests, changelogs, and install-ready Docker
artifacts.

This repository is for installing **published releases**, not for building the
app from source.

## Quick Start

Installs the current published release from GHCR:

```bash
git clone https://github.com/zivego/wiregate-releases.git
cd wiregate-releases
cp deploy/compose/.env.example .env
```

Edit `.env` and set at least:

```env
WIREGATE_BOOTSTRAP_ADMIN_EMAIL=admin@yourcompany.com
WIREGATE_BOOTSTRAP_ADMIN_PASSWORD=change-me-to-a-strong-password
```

Then start WireGate:

```bash
docker compose -f deploy/compose/docker-compose.yml up -d
```

Open:

- `http://your-host` for default HTTP installs
- `https://your-host` if you mounted TLS cert/key in `.env`

Default published version in this repo:

- `WIREGATE_VERSION=1.0.3`

To install a different published version, change `WIREGATE_VERSION` in `.env`.

## What This Installer Does

The default compose file:

- pulls published images from GHCR
- starts backend + frontend + embedded WireGuard gateway
- opens:
  - frontend HTTP on port `80`
  - frontend HTTPS on port `443` when certs are mounted
  - WireGuard UDP on port `55182`
- enables in-app updates via `wiregate-releases/manifest.json`

It does **not** build local `dev` images.

## Important

Do **not** use:

```bash
docker compose up --build
```

in this repository if your goal is to install a release.

`--build` is for local source builds and can produce a `dev` build instead of a
published release.

## TLS

Built-in TLS is optional.

If you want the frontend container itself to serve HTTPS, set:

```env
WIREGATE_TLS_CERT_FILE=./certs/tls.crt
WIREGATE_TLS_KEY_FILE=./certs/tls.key
WIREGATE_COOKIE_INSECURE=false
```

Then recreate the stack:

```bash
docker compose -f deploy/compose/docker-compose.yml up -d
```

For production, a reverse proxy such as Caddy, Traefik, or nginx is still the
recommended path.

## Update Flow

This installer points the updater at:

- `https://raw.githubusercontent.com/zivego/wiregate-releases/main/manifest.json`

So the **Update** page checks published releases from this repository.

## Stop / Remove

Stop containers:

```bash
docker compose -f deploy/compose/docker-compose.yml down
```

Remove containers **and data volumes**:

```bash
docker compose -f deploy/compose/docker-compose.yml down -v
```

## Changelog

- latest release manifest: [manifest.json](./manifest.json)
- GitHub releases: https://github.com/zivego/wiregate-releases/releases
