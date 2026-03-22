#!/bin/sh
# Pick HTTPS config only when real certs are mounted; otherwise serve plain HTTP.
# Check file size > 0 to ignore /dev/null bind-mounts.
#
# nginx loads ALL *.conf files from /etc/nginx/conf.d/, so we must remove
# the unused config to avoid loading both HTTP and TLS server blocks.
if [ -s /etc/nginx/certs/tls.crt ] && [ -s /etc/nginx/certs/tls.key ]; then
  cp /etc/nginx/conf.d/tls.conf /etc/nginx/conf.d/default.conf
  rm -f /etc/nginx/conf.d/http.conf /etc/nginx/conf.d/tls.conf
  echo "[frontend] TLS enabled (443)"
else
  cp /etc/nginx/conf.d/http.conf /etc/nginx/conf.d/default.conf
  rm -f /etc/nginx/conf.d/http.conf /etc/nginx/conf.d/tls.conf
  echo "[frontend] plain HTTP (80) — use a reverse proxy (Caddy, Traefik) for TLS"
fi
exec nginx -g 'daemon off;'
