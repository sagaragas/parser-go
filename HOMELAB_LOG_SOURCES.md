# Homelab log sources

This file is the repo-local source of truth for choosing homelab-backed benchmark corpora. It is derived from `/root/ansible` plus the `homelab-reference` skill, and it exists so later benchmark workers do not need to rediscover where real-world log slices should come from.

## Source-of-truth inputs

Use these files before changing this document:

- `/root/ansible/inventory.yml`
- `/root/ansible/group_vars/all.yml`
- `/root/ansible/host_vars/caddy.yml`
- `/root/ansible/host_vars/arr.yml`
- `/root/ansible/host_vars/homepage.yml`
- `/root/ansible/host_vars/status.yml`
- `/root/ansible/host_vars/speedtest.yml`
- `/root/ansible/host_vars/scrutiny.yml`
- `/root/ansible/host_vars/jelly.yml`
- `/root/ansible/host_vars/plex.yml`
- `/root/ansible/host_vars/torrent.yml`
- `/root/ansible/host_vars/adguard.yml`
- `/root/ansible/host_vars/unifi.yml`
- `/root/ansible/host_vars/pbs.yml`

Use inventory hostnames from those files in repo edits. Do not replace them with guessed raw-IP descriptions.

## Hard rules for corpus collection

1. **Prefer ingress-first capture.** Start with `caddy` unless there is a specific parser-coverage gap that requires an app-side fallback.
2. **Keep raw capture outside git.** Write any unsanitized slice to `/tmp`, another throwaway path, or an ignored workspace outside `/root/parser-go`.
3. **Sanitize before repo entry.** Only sanitized derivatives, hashes, manifests, and redaction reports may enter `benchmark/`, `evidence/`, or any commit.
4. **Capture bounded windows.** Pull a fixed time window or explicit line range, not an open-ended log history.
5. **Record provenance.** For every sanitized corpus, keep the source hostname, service name, capture window, route mix, corpus hash, and whether the slice is illustrative or representative in the benchmark metadata.
6. **Treat this file as internal coordination material.** It may name internal hostnames and service placement from `/root/ansible`; later publication-facing surfaces must translate that into sanitized public wording instead of copying this file verbatim.

## Approved sources

### Primary source: `caddy` ingress logs

- **Owning host/service:** `caddy` / native `caddy`
- **Source-of-truth refs:** `/root/ansible/inventory.yml`, `/root/ansible/host_vars/caddy.yml`
- **What it covers:** Central ingress for `home`, `sonarr`, `radarr`, `prowlarr`, `seerr`, `requestrr`, `cleanup`, `scrutiny`, `speed`, `jelly`, `qbit`, `uptime`, `status`, `tautulli`, plus a smaller set of more sensitive admin routes such as `adguard`, `unifi`, `pbs`, and `pve1`-`pve4`
- **How to collect without committing raw data:** Capture a bounded slice from the `caddy` host into a non-repo temp path. Prefer the systemd journal for the service, or the access-log sink configured in `/etc/caddy/Caddyfile` if the host is writing structured access logs there. Example collection pattern from the Ansible control node:

  ```sh
  cd /root/ansible
  ansible caddy -m shell -a "journalctl -u caddy --since '2026-03-28 00:00:00' --until '2026-03-28 01:00:00' --no-pager" > /tmp/caddy-ingress.raw
  ```

  Sanitize `/tmp/caddy-ingress.raw` immediately and only copy the sanitized derivative or its hash into repo-managed artifacts.
- **Why this is the preferred dataset:** It is the broadest and most representative parser-coverage source in the homelab because it sees cross-service ingress, mixed status codes, bots plus browsers, redirect traffic, asset fetches, API calls, and real path/query variation through a single consistent collection point.
- **Sanitization and publication constraints:** Remove or pseudonymize client IPs, cookies, authorization material, query-string secrets, referrer details, user-agent tokens, and internal-only host/path segments. Either drop or heavily redact requests hitting `adguard`, `unifi`, `pbs`, and `pve*` admin surfaces before anything publishable is produced.

### Secondary fallback: `arr` application logs

- **Owning host/service:** `arr` / `sonarr`, `radarr`, `prowlarr`, `seerr`, `requestrr`, `cleanuparr`
- **Source-of-truth refs:** `/root/ansible/host_vars/arr.yml`
- **How to collect without committing raw data:** Use the compose path from source of truth and write the raw output to a temp file outside the repo:

  ```sh
  cd /root/ansible
  ansible arr -m shell -a "cd /opt/arr && docker compose logs --no-color --since '1h' sonarr radarr prowlarr seerr requestrr cleanuparr" > /tmp/arr-stack.raw
  ```

- **Why this source is useful:** These logs are a good fallback when ingress slices are too thin for API-heavy paths, search/filter endpoints, or app-generated request shapes that later need dedicated parser coverage.
- **Sanitization and publication constraints:** Strip API keys, usernames, media titles, download paths, webhook URLs, and any query parameters that encode identifiers or secrets. Do not commit container logs verbatim.

### Secondary fallback: `homepage` dashboard logs

- **Owning host/service:** `homepage` / Docker `homepage`
- **Source-of-truth refs:** `/root/ansible/host_vars/homepage.yml`
- **How to collect without committing raw data:** Pull bounded compose logs from `/opt/homepage` into a temp file outside the repo, for example:

  ```sh
  cd /root/ansible
  ansible homepage -m shell -a "cd /opt/homepage && docker compose logs --no-color --since '1h' homepage" > /tmp/homepage.raw
  ```

- **Why this source is useful:** Useful for small, mostly-read-heavy request mixes: dashboard page loads, static assets, and widget-driven refresh traffic.
- **Sanitization and publication constraints:** Redact any service labels, widget targets, environment-derived values, or internal-only URLs that may leak from dashboard configuration.

### Secondary fallback: `status` monitor logs

- **Owning host/service:** `status` / Docker `uptime-kuma`
- **Source-of-truth refs:** `/root/ansible/host_vars/status.yml`
- **How to collect without committing raw data:** Pull bounded compose logs from `/opt/docker` on `status` into a temp file outside the repo, for example:

  ```sh
  cd /root/ansible
  ansible status -m shell -a "cd /opt/docker && docker compose logs --no-color --since '1h' uptime-kuma" > /tmp/status.raw
  ```

- **Why this source is useful:** Good for recurring health-check traffic, short request paths, redirects, and status-oriented UI/API activity that may not dominate the main ingress sample.
- **Sanitization and publication constraints:** Remove monitor names, endpoint URLs, notification targets, and any incident text that could reveal internal topology or service naming.

### Secondary fallback: `speedtest` logs

- **Owning host/service:** `speedtest` / Docker `speedtest-tracker`
- **Source-of-truth refs:** `/root/ansible/host_vars/speedtest.yml`
- **How to collect without committing raw data:** Pull bounded compose logs from `/opt/speedtest` into a temp file outside the repo, for example:

  ```sh
  cd /root/ansible
  ansible speedtest -m shell -a "cd /opt/speedtest && docker compose logs --no-color --since '1h' speedtest-tracker" > /tmp/speedtest.raw
  ```

- **Why this source is useful:** Adds a smaller but distinct mix of UI requests, scheduled-job traffic, and measurement endpoints that can broaden parser coverage beyond generic dashboards.
- **Sanitization and publication constraints:** Redact account identifiers, schedule metadata, server-selection details, and any captured external endpoint names before producing a sanitized corpus.

### Secondary fallback: `scrutiny` logs

- **Owning host/service:** `scrutiny` / Docker `scrutiny-web`
- **Source-of-truth refs:** `/root/ansible/host_vars/scrutiny.yml`
- **How to collect without committing raw data:** Pull bounded compose logs from `/opt/docker` on `scrutiny` into a temp file outside the repo, for example:

  ```sh
  cd /root/ansible
  ansible scrutiny -m shell -a "cd /opt/docker && docker compose logs --no-color --since '1h' scrutiny-web" > /tmp/scrutiny.raw
  ```

- **Why this source is useful:** Useful for admin-style UI traffic with predictable but different paths from the media and dashboard services, especially when the benchmark needs another non-media fallback behind `caddy`.
- **Sanitization and publication constraints:** Remove disk identifiers, host labels, SMART metadata, and any device naming that could reveal physical inventory.

### Secondary fallback: media-service UI logs

- **Owning host/service:** `jelly` / native `jellyfin`; `plex` / native `tautulli`
- **Source-of-truth refs:** `/root/ansible/host_vars/jelly.yml`, `/root/ansible/host_vars/plex.yml`
- **How to collect without committing raw data:** Capture a bounded journal slice on the owning host for the relevant native service unit and write it to a temp path outside the repo. Verify the unit name on-host before capture instead of guessing. Example patterns:

  ```sh
  cd /root/ansible
  ansible jelly -m shell -a "journalctl -u jellyfin --since '1h' --no-pager" > /tmp/jellyfin.raw
  ansible plex -m shell -a "journalctl -u tautulli --since '1h' --no-pager" > /tmp/tautulli.raw
  ```

- **Why this source is useful:** Use these only when the benchmark needs media-oriented request shapes such as long paths, artwork fetches, or range-request-heavy browsing behavior that is underrepresented in the primary ingress slice.
- **Sanitization and publication constraints:** Remove media titles, library paths, device names, usernames, session identifiers, and any playback metadata. `tautulli` is the preferred `plex`-side fallback because it is the Caddy-backed surface named in source of truth; do not assume the main Plex service is part of the same ingress path.

## Not approved by default

Do **not** use these as routine benchmark corpora unless a later feature explicitly justifies the risk and documents stronger redaction:

- `torrent` / `qbittorrent` from `/root/ansible/host_vars/torrent.yml` because torrent names, tracker URLs, download paths, and VPN-related details are too likely to leak sensitive data.
- `adguard` / `adguardhome`, `unifi` / `unifi`, `pbs` / `proxmox-backup`, and the `pve1`-`pve4` admin routes because they are control-plane surfaces with device identifiers, infrastructure topology, or administrative activity.
- `ansible` host logs because that host is the bastion/control node and can contain unrelated operational activity.

## Selection order for later workers

1. Start with a bounded `caddy` ingress slice.
2. If coverage is missing, add one fallback source that fills the gap instead of mixing many small corpora.
3. Sanitize first, hash the sanitized result, and keep the raw slice out of the repository.
4. When in doubt, choose the less sensitive source and document why.
