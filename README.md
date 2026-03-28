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
cp .env.example .env
```

Edit `.env` and set at least:

```env
WIREGATE_BOOTSTRAP_ADMIN_EMAIL=admin@yourcompany.com
WIREGATE_BOOTSTRAP_ADMIN_PASSWORD=change-me-to-a-strong-password
```

If you want the frontend reachable only from a local reverse proxy on the same
host, bind it to localhost instead of all interfaces:

```env
WIREGATE_FRONTEND_PORT=127.0.0.1:5656
WIREGATE_FRONTEND_TLS_PORT=127.0.0.1:56443
```

Then start WireGate:

```bash
docker compose up -d
```

If you already cloned this repository earlier, update it first:

```bash
git pull
cp .env.example .env   # only if .env does not exist yet
docker compose up -d
```

Open:

- `http://your-host` for default HTTP installs
- `https://your-host` if you mounted TLS cert/key in `.env`

Default published version in this repo:

- `WIREGATE_VERSION=1.0.5`

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
docker compose up -d
```

For production, a reverse proxy such as Caddy, Traefik, or nginx is still the
recommended path.

## Update Flow

This installer points the updater at:

- `https://raw.githubusercontent.com/zivego/wiregate-releases/main/manifest.json`

So the **Update** page checks published releases from this repository.

Starting with `1.0.5`, compose-managed updates persist `WIREGATE_VERSION` and
use a stable project-local compose override for future `docker compose up -d`
runs.

`1.0.5` also fixes reverse-proxy cookie-auth handling for mutating UI actions
such as **Update Now** and **Force logout** behind Caddy/Cloudflare-style
setups.

## Stop / Remove

Stop containers:

```bash
docker compose down
```

Remove containers **and data volumes**:

```bash
docker compose down -v
```

## Changelog

- latest release manifest: [manifest.json](./manifest.json)
- GitHub releases: https://github.com/zivego/wiregate-releases/releases

## Troubleshooting

If Docker prints:

```text
Error parsing config file (/root/.docker/config.json): is a directory
```

your host Docker CLI config is broken. Fix it on the host:

```bash
mkdir -p /root/.docker
rm -rf /root/.docker/config.json
printf '{}\n' > /root/.docker/config.json
chmod 600 /root/.docker/config.json
unset DOCKER_CONFIG
```
