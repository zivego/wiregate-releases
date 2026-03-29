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

- `WIREGATE_VERSION=1.0.6`

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

## Install Linux Agent (Client)

Use this when you want to connect a Linux host to your WireGate server.

Prerequisites:

- a running WireGate server URL such as `https://wiregate.example.com`
- an enrollment token created in the WireGate admin UI
- Linux host with `wireguard-tools`
- root access for config apply and service install
- Go toolchain if you want to build the client from source

Install dependencies:

```bash
sudo apt-get update
sudo apt-get install -y wireguard-tools
```

Build the Linux client from source:

```bash
git clone https://github.com/zivego/wiregate.git
cd wiregate
go build -o wiregate-agent-linux ./cmd/wiregate-agent-linux
sudo install -m 0755 ./wiregate-agent-linux /usr/local/bin/wiregate-agent-linux
```

Enroll the client:

```bash
sudo /usr/local/bin/wiregate-agent-linux enroll \
  --server https://wiregate.example.com \
  --token <enrollment-token>
```

Install and start the systemd service:

```bash
sudo /usr/local/bin/wiregate-agent-linux install-service
sudo systemctl status wiregate-agent-linux
```

Useful follow-up commands:

```bash
sudo journalctl -u wiregate-agent-linux -f
sudo /usr/local/bin/wiregate-agent-linux check-in
sudo /usr/local/bin/wiregate-agent-linux uninstall-service
```

## Changelog

- latest release manifest: [manifest.json](./manifest.json)
- GitHub releases: https://github.com/zivego/wiregate-releases/releases
