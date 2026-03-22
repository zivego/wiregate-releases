export function normalizeServerURL(serverURL: string): string {
  const trimmed = serverURL.trim();
  if (trimmed === "") {
    return "http://localhost:8080";
  }
  if (/^https?:\/\//i.test(trimmed)) {
    return trimmed;
  }
  return `http://${trimmed}`;
}

export function fallbackHost(serverURL: string): string {
  const normalized = normalizeServerURL(serverURL);
  try {
    return new URL(normalized).hostname || "127.0.0.1";
  } catch {
    return "127.0.0.1";
  }
}

export function resolveHostname(hostname: string, serverURL: string): string {
  return hostname.trim() || fallbackHost(serverURL);
}

function shellQuote(value: string): string {
  return `'${value.replace(/'/g, `'\\''`)}'`;
}

function powershellQuote(value: string): string {
  return `'${value.replace(/'/g, "''")}'`;
}

export function linuxInstallCommand(serverURL: string, token: string, hostname: string, binaryPath: string): string {
  const normalizedServerURL = normalizeServerURL(serverURL);
  const resolvedHostname = resolveHostname(hostname, serverURL);
  const resolvedBinaryPath = binaryPath.trim() || "/usr/local/bin/wiregate-agent-linux";
  const script = `set -euo pipefail; BINARY_PATH=${shellQuote(resolvedBinaryPath)}; SERVER_URL=${shellQuote(normalizedServerURL)}; ENROLLMENT_TOKEN=${shellQuote(token)}; AGENT_HOSTNAME=${shellQuote(resolvedHostname)}; test -x "$BINARY_PATH" || { echo "wiregate agent binary not found at $BINARY_PATH" >&2; exit 1; }; command -v wg-quick >/dev/null 2>&1 || { echo "wg-quick is required on the target host" >&2; exit 1; }; mkdir -p /var/lib/wiregate-agent /etc/wireguard; "$BINARY_PATH" enroll --server "$SERVER_URL" --token "$ENROLLMENT_TOKEN" --hostname "$AGENT_HOSTNAME" --apply=true; "$BINARY_PATH" install-service --binary-path "$BINARY_PATH" --state-file /var/lib/wiregate-agent/state.json --apply=true`;
  return `sudo bash -lc ${shellQuote(script)}`;
}

export function windowsInstallCommand(serverURL: string, token: string, hostname: string, binaryPath: string): string {
  const normalizedServerURL = normalizeServerURL(serverURL);
  const resolvedHostname = resolveHostname(hostname, serverURL);
  const resolvedBinaryPath = binaryPath.trim() || String.raw`C:\Program Files\Wiregate\wiregate-agent-windows.exe`;
  const script = `$server=${powershellQuote(normalizedServerURL)}; $token=${powershellQuote(token)}; $hostname=${powershellQuote(resolvedHostname)}; $binary=${powershellQuote(resolvedBinaryPath)}; if (-not (Test-Path $binary)) { throw "wiregate agent binary not found at $binary" }; & $binary enroll --server $server --token $token --hostname $hostname --apply=true; & $binary install-service --binary-path $binary --state-file 'C:\\ProgramData\\Wiregate\\state.json' --apply=true`;
  return `powershell -NoProfile -ExecutionPolicy Bypass -Command ${powershellQuote(script)}`;
}
