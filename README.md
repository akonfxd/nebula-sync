# nebula-sync

Synchronize Pi-hole v6.x configuration to replicas.

This project is not a part of the [official Pi-hole project](https://github.com/pi-hole), but uses the api provided by Pi-hole instances to perform the synchronization actions.

> ### ⚠️ This is a fork, and it diverges substantially
>
> This is a fork of [lovelaze/nebula-sync](https://github.com/lovelaze/nebula-sync).
> It is **not** a drop-in replacement: configuration is stored in a database
> instead of being read from the environment on every start, and the service
> now ships a web console. An existing deployment will keep working, but it
> will not behave the way the upstream README describes.
>
> Read [What is different in this fork](#what-is-different-in-this-fork) before
> upgrading, and keep a copy of your environment file.

## What is different in this fork

| | upstream | this fork |
|---|---|---|
| Where configuration lives | environment variables, read on every start | a SQLite database; the environment seeds it **once** |
| Changing configuration | edit the environment, restart | edit it in the console, no restart |
| Sync settings | one set, applied to every replica | **a profile per replica** — each can sync a different set of sections |
| Web interface | none | a built-in console, compiled into the binary |
| Accounts | none | multi-user, `admin` and `viewer` roles, bcrypt passwords, session cookies |
| HTTP server | `/health` only, always on | `/health` plus a JSON api under `/api/v1`, served only when `API_ENABLED=true` |
| Pi-hole passwords at rest | only ever in the environment | AES-GCM encrypted in the database under `ENCRYPTION_KEY` |
| First start with no configuration | fatal error | starts anyway and waits for you in the console |
| Run history | last outcome in memory | every run stored, with per-replica timing and errors |
| `/health` before the first sync | 500 | 200 — an unconfigured install is not a broken one |

Everything upstream can do, this can still do, and the environment variables
below are unchanged. The difference is that they are now a *starting point*
rather than the permanent source of truth.

### Upgrading from upstream

Two things will surprise you.

**`/health` is no longer always on.** Upstream served it on port 8080 from
every run. Here the HTTP server starts only when `API_ENABLED=true`, which the
container image sets for you but the bare binary does not. If you monitor
`/health` and run the binary, set `API_ENABLED=true`.

**Your environment stops being read after the first start.** The first run
copies `PRIMARY`, `REPLICAS`, `FULL_SYNC`, `CRON`, `RUN_GRAVITY` and every
`SYNC_*` and `WEBHOOK_*` variable into the database and never looks at them
again. Editing your compose file afterwards changes nothing, and the service
logs each variable it is ignoring. Change them in the console instead — or
delete the database to start over.

### Features

- **Full sync**: Use Pi-hole Teleporter for full synchronization.
- **Selective sync**: Selective feature synchronization.
- **Cron schedule**: Run on cron schedule.
- **Per-replica profiles**: give each replica its own sync settings.
- **Web console**: manage instances, profiles, schedule and accounts in a browser.
- **Run history**: see what each replica did, how long it took and why it failed.

## Installation

> **There is no published image for this fork — you build it.**
> `ghcr.io/lovelaze/nebula-sync` and `docker.io/lovelaze/nebula-sync` are the
> *upstream* images and contain none of what this README describes.

```bash
docker build -t nebula-sync .
```

The build does the whole job: it compiles the console with Node and embeds it
in the binary. Nothing needs installing on the host.

### Docker Compose (recommended)

The image enables the console and stores its database in `/data`, so the only
thing it really needs is a volume and a port.

```yaml
---
services:
  nebula-sync:
    image: nebula-sync
    container_name: nebula-sync
    ports:
      - "8080:8080"
    volumes:
      - nebula-sync:/data
    environment:
      # Keep this safe. Without it, Pi-hole passwords are stored in plain text,
      # and if you lose it the stored passwords cannot be read back.
      - ENCRYPTION_KEY=${NEBULA_SYNC_KEY}
      # Optional: seed the configuration on the very first start.
      - PRIMARY=http://ph1.example.com|password
      - REPLICAS=http://ph2.example.com|password,http://ph3.example.com|password
      - FULL_SYNC=true
      - RUN_GRAVITY=true
      - CRON=0 * * * *

volumes:
  nebula-sync:
```

Then open `http://localhost:8080` and create the first account.

Generate a key with:

```bash
openssl rand -hex 32
```

### Docker CLI

```bash
docker run -d \
  --name nebula-sync \
  -p 8080:8080 \
  -v nebula-sync:/data \
  -e ENCRYPTION_KEY="$(openssl rand -hex 32)" \
  nebula-sync
```

That is a complete install. No `PRIMARY`, no `REPLICAS` — add them in the
console. (Save that key somewhere; generating it inline like this means you do
not have a copy.)

### Binary

Build from a checkout. The console is a separate build step, so this needs
Node 22 as well as Go:

```bash
make web-deps   # npm ci, once
make build      # builds the console, then the binary
```

`go build .` on its own still works and is what CI does for the no-Node case —
it just produces a binary that serves the api and logs
`No console was built into this binary`.

> `go install github.com/lovelaze/nebula-sync@latest` installs **upstream**,
> not this fork. The module path is unchanged, so there is no import path that
> resolves here.

Run it:

```bash
# console off by default, like upstream
nebula-sync run

# with the console, on http://localhost:8080
API_ENABLED=true nebula-sync run

# read envs from a file
nebula-sync run --env-file .env
```

> **Note:** unlike the container, the binary has the console **off** by default
> and writes `./nebula-sync.db`. With `API_ENABLED` unset it behaves like
> upstream, including refusing to start when nothing is configured.

## First run

With the console enabled there are two ways to start.

**Nothing configured.** The service starts, warns that nothing is configured,
and serves the console. Open it, create the first administrator, add the
primary and the replicas, and press Sync. Nothing needs restarting.

**Seeded from the environment.** If `PRIMARY`, `REPLICAS` and `FULL_SYNC` are
set the first time the database is created, they are written into it and the
service syncs immediately, exactly as upstream would.

Either way, **after that first start the database is authoritative**. Changing
`PRIMARY` or `SYNC_CONFIG_DNS` later has no effect; the service logs the
variables it is ignoring and carries on with what is stored. The variables that
behave this way are `PRIMARY`, `PRIMARY_FILE`, `REPLICAS`, `REPLICAS_FILE`,
`FULL_SYNC`, `RUN_GRAVITY`, `CRON`, and everything beginning with `SYNC_` or
`WEBHOOK_`.

If you set `API_ADMIN_USERNAME` and `API_ADMIN_PASSWORD`, that account is
created for you; otherwise the console shows a one-time screen to create the
first one.

## The console

Served on `API_ADDR`, from the same process and the same port as the api.

- **Dashboard** — current state, last and next run, the schedule, a Sync now
  button, per-replica results and the run history.
- **Instances** — add, edit, reorder, enable and disable the primary and the
  replicas, and choose which profile each replica uses.
- **Profiles** — named sets of sync settings. Each replica points at one, so
  one replica can take everything while another takes only `dns.upstreams`.
- **Users** — accounts and roles.
- **Account** — change your own password.

Two roles: **admin** can change anything, **viewer** can read everything and
change nothing (not even trigger a sync).

Leaving a password field blank when editing an instance keeps the stored
password. Passwords are never sent back to the browser — the api reports only
whether one is set.

## Configuration

### Runtime environment variables

These are read on **every** start and are not stored in the database. They are
new in this fork, except `TZ` and the `CLIENT_*` family.

| Name | Default | Example | Description |
|------|---------|---------|-------------|
| `DB_PATH` | `nebula-sync.db` | `/data/nebula-sync.db` | Where the configuration database lives. The container image sets `/data/nebula-sync.db`. |
| `ENCRYPTION_KEY` | n/a | 64 hex characters | 32-byte key, hex or base64, used to encrypt Pi-hole passwords at rest. Without it they are stored in plain text and the service warns on every start. |
| `ENCRYPTION_KEY_FILE` | n/a | `/run/secrets/key` | Read the key from a file instead (Docker secrets). |
| `PASSWORD_COST` | `12` | `10` | bcrypt work factor for console accounts. Lower it on slow hardware, where signing in can otherwise take seconds. |
| `API_ENABLED` | `false` | `true` | Serve the console and the api. **The container image sets this to `true`.** |
| `API_ADDR` | `:8080` | `127.0.0.1:8080` | Listen address. The default binds every interface; give it a host to restrict it. |
| `API_ADMIN_USERNAME` | n/a | `admin` | Create this administrator on an empty database. |
| `API_ADMIN_PASSWORD` | n/a | | Its password. Without both, the console shows a first-run screen instead. |
| `API_ADMIN_PASSWORD_FILE` | n/a | `/run/secrets/admin` | Read the admin password from a file. |
| `API_SESSION_TTL` | `168h` | `24h` | How long a sign-in lasts. |
| `API_ALLOWED_ORIGINS` | n/a | `https://sync.example.com` | Extra origins allowed to make write requests. Needed only when the console is reached through a reverse proxy under a different name. |
| `TZ` | n/a | `Europe/London` | Timezone for logs and cron. |
| `CLIENT_SKIP_TLS_VERIFICATION` | `false` | `true` | Skip TLS certificate verification when talking to Pi-hole. |
| `CLIENT_RETRY_DELAY_SECONDS` | `1` | `5` | Seconds between connection attempts. |
| `CLIENT_TIMEOUT_SECONDS` | `20` | `60` | HTTP client timeout. |

> **Losing `ENCRYPTION_KEY` means losing the stored Pi-hole passwords.** They
> cannot be recovered; you would have to re-enter them in the console. Back it
> up somewhere other than next to the database.

### Seed environment variables

Everything in this section is read **only when the database is empty**. After
that it is ignored, and the console is where you change these settings. They
are documented here because they still work for first-run provisioning, and
because the console presents the same options under the same names.

#### Required on a first run without the console

If `API_ENABLED` is false, these are required and the service exits without
them. With the console enabled they are all optional.

| Name | Default | Example | Description |
|------|---------|---------|-------------|
| `PRIMARY` | n/a | `http://ph1.example.com\|password` | The primary Pi-hole |
| `REPLICAS` | n/a | `http://ph2.example.com\|password,http://ph3.example.com\|password` | The replica Pi-holes |
| `FULL_SYNC` | n/a | `true` | Whether to perform a full synchronization |

> **Note:** When `FULL_SYNC=true`, the system will perform a full Teleporter import/export from the primary Pi-hole to the replicas. This will synchronize all settings and configurations.

> **Docker secrets:** `PRIMARY` and `REPLICAS` support Docker secrets when defined as `PRIMARY_FILE` and `REPLICAS_FILE`. See the note regarding the default user and the Docker secrets example below.

#### Optional

| Name | Default | Example | Description |
|------|---------|---------|-------------|
| `CRON` | n/a | `0 * * * *` | Cron schedule for synchronization |
| `RUN_GRAVITY` | `false` | `true` | Run gravity after syncing |

The schedule is also editable in the console at runtime, which is the only way
to change it without recreating the database.

#### Selective sync

> **Note:** these apply only if `FULL_SYNC=false`. They seed the **default**
> profile. Additional profiles are created in the console.

| Name | Default | Description |
|------|---------|-------------|
| `SYNC_CONFIG_DNS` | false | Synchronize DNS settings |
| `SYNC_CONFIG_DHCP` | false | Synchronize DHCP settings |
| `SYNC_CONFIG_NTP` | false | Synchronize NTP settings |
| `SYNC_CONFIG_RESOLVER` | false | Synchronize resolver settings |
| `SYNC_CONFIG_DATABASE` | false | Synchronize database settings |
| `SYNC_CONFIG_MISC` | false | Synchronize miscellaneous settings |
| `SYNC_CONFIG_DEBUG` | false | Synchronize debug settings |
| `SYNC_GRAVITY_DHCP_LEASES` | false | Synchronize DHCP leases |
| `SYNC_GRAVITY_GROUP` | false | Synchronize groups |
| `SYNC_GRAVITY_AD_LIST` | false | Synchronize ad lists |
| `SYNC_GRAVITY_AD_LIST_BY_GROUP` | false | Synchronize ad lists by group |
| `SYNC_GRAVITY_DOMAIN_LIST` | false | Synchronize domain lists |
| `SYNC_GRAVITY_DOMAIN_LIST_BY_GROUP` | false | Synchronize domain lists by group |
| `SYNC_GRAVITY_CLIENT` | false | Synchronize clients |
| `SYNC_GRAVITY_CLIENT_BY_GROUP` | false | Synchronize clients by group |

#### Config filters

Include and exclude lists for individual configuration keys. Include wins if
both are set.

| Name | Example | Description |
|------|---------|-------------|
| `SYNC_CONFIG_DNS_INCLUDE` | `upstreams,interface` | DNS config keys to include |
| `SYNC_CONFIG_DNS_EXCLUDE` | `upstreams,interface` | DNS config keys to exclude |
| `SYNC_CONFIG_DHCP_INCLUDE` | `active,start` | DHCP config keys to include |
| `SYNC_CONFIG_DHCP_EXCLUDE` | `active,start` | DHCP config keys to exclude |
| `SYNC_CONFIG_NTP_INCLUDE` | `ipv4,sync` | NTP config keys to include |
| `SYNC_CONFIG_NTP_EXCLUDE` | `ipv4,sync` | NTP config keys to exclude |
| `SYNC_CONFIG_RESOLVER_INCLUDE` | `resolveIPv4,networkNames` | Resolver config keys to include |
| `SYNC_CONFIG_RESOLVER_EXCLUDE` | `resolveIPv4,networkNames` | Resolver config keys to exclude |
| `SYNC_CONFIG_DATABASE_INCLUDE` | `DBimport,maxDBdays` | Database config keys to include |
| `SYNC_CONFIG_DATABASE_EXCLUDE` | `DBimport,maxDBdays` | Database config keys to exclude |
| `SYNC_CONFIG_MISC_INCLUDE` | `nice,delay_startup` | Misc config keys to include |
| `SYNC_CONFIG_MISC_EXCLUDE` | `nice,delay_startup` | Misc config keys to exclude |
| `SYNC_CONFIG_DEBUG_INCLUDE` | `database,networking` | Debug config keys to include |
| `SYNC_CONFIG_DEBUG_EXCLUDE` | `database,networking` | Debug config keys to exclude |

### Webhooks

Nebula Sync can invoke webhooks depending if a sync succeeded or failed. URL is required for the webhook to trigger. Both success and failure webhooks use the same environment variable pattern. Webhooks have a timeout of 10 seconds.

> **Note:** Replace `<OUTCOME>` with either `SUCCESS` or `FAILURE`.

| Name | Default | Example | Description |
|------|---------|---------|-------------|
| `WEBHOOK_SYNC_<OUTCOME>_URL` | n/a | `https://www.example.com/webhook` | URL to invoke for the webhook |
| `WEBHOOK_SYNC_<OUTCOME>_METHOD` | `POST` | `GET` | The HTTP method for the webhook |
| `WEBHOOK_SYNC_<OUTCOME>_BODY` | n/a | `this is my webhook body` | The body of the webhook request |
| `WEBHOOK_SYNC_<OUTCOME>_HEADERS` | n/a | `header1:foo,header2:bar` | HTTP headers to set for the webhook request in the format `key:value` separated by comma. Any whitespace will be used verbatim, no string trimming. |

Additionally, you can skip TLS verification for all webhooks if necessary:

| Name | Default | Example | Description |
|------|---------|---------|-------------|
| `WEBHOOK_CLIENT_SKIP_TLS_VERIFICATION` | `false` | `true` | Skips TLS certificate verification |

> **Fork note:** webhooks are seeded from the environment and stored like
> everything else, but there is **no screen and no api route to edit them yet**.
> To change them you currently have to recreate the database. This is the
> largest known gap.

#### Integration examples:

##### healthcheck.io:

```
WEBHOOK_SYNC_SUCCESS_URL=https://hc-ping.com/{your-slug-or-guid-here}
WEBHOOK_SYNC_FAILURE_URL=https://hc-ping.com/{your-slug-or-guid-here}/fail
```

##### Apprise:

```
WEBHOOK_SYNC_FAILURE_URL=http://localhost:8080/notify
WEBHOOK_SYNC_FAILURE_BODY=urls=mailto://user:pass@gmail.com&body=test message
```

##### A service that needs JSON:

```
WEBHOOK_SYNC_FAILURE_URL=https://www.example.com/notify.json
WEBHOOK_SYNC_FAILURE_BODY={"hello":"world"}
WEBHOOK_SYNC_FAILURE_HEADERS=Content-Type:application/json
```

## HTTP API

Everything the console does, it does through this. JSON in, JSON out, session
cookie for authentication. Writes require an `Origin` header matching the
server, or one listed in `API_ALLOWED_ORIGINS`.

| Method and path | Role | |
|---|---|---|
| `GET /health` | none | 200 unless the last sync failed |
| `GET /api/v1/info` | none | version, whether an account exists, whether you are signed in |
| `POST /api/v1/auth/bootstrap` | none | create the first administrator; 409 once one exists |
| `POST /api/v1/auth/login` | none | sign in |
| `POST /api/v1/auth/logout` | any | sign out |
| `GET /api/v1/auth/session` | any | who you are |
| `POST /api/v1/auth/password` | any | change your own password |
| `GET /api/v1/schema` | any | the sections, tables, roles and filters the server supports |
| `GET /api/v1/status` | any | current state and the last run |
| `GET /api/v1/runs` | any | run history |
| `GET /api/v1/schedule` | any | the cron schedule and the next run |
| `PUT /api/v1/schedule` | admin | change or clear the schedule |
| `POST /api/v1/sync` | admin | start a sync; 409 if one is running or nothing is configured |
| `GET /api/v1/instances` | any | the Pi-holes, without their passwords |
| `POST`, `PUT`, `DELETE /api/v1/instances[/{id}]` | admin | add, edit and remove them |
| `GET /api/v1/profiles` | any | the sync profiles |
| `POST`, `PUT`, `DELETE /api/v1/profiles[/{id}]` | admin | add, edit and remove them |
| `GET`, `POST`, `PUT`, `DELETE /api/v1/users[/{id}]` | admin | accounts |

`/api/v1/auth/login` and `/api/v1/auth/bootstrap` are rate limited to 10
attempts per 15 minutes per client address, answering `429` with a
`Retry-After` header.

`/api/v1/schema` exists so clients do not have to hardcode the vocabulary. It
also reports which sections cannot be synchronized, and why.

## Notes / Known issues

### `webserver` and `files` cannot be synchronized

Both are refused, by the api and in the console, where the checkbox is disabled
and shows the reason. Synchronizing `webserver` would copy the primary's API
password onto every replica and lock you out of them.

### Back up the database, not just the environment

`DB_PATH` now holds your instances, profiles, schedule, accounts and run
history. The environment file no longer describes your deployment. Back up the
database file and the `ENCRYPTION_KEY` separately.

### Default user of Docker container / Docker secrets example

By default, the Docker container runs as user `1001`. If you are using Docker secrets, the user that is running the container will need read permissions to the files that the Docker secrets reference. If the user does not have the right permissions you will receive an error `Failed to initialize service error="open /run/secrets/primary: permission denied"`. To avoid this error, either make sure to `chown 1001 ./your/secretfiles && chmod 400 ./your/secretfiles` or use the [`user` directive in Docker Compose](https://docs.docker.com/reference/compose-file/services/#user) to change the user that the container runs as to a user of your choice - and then make sure to update your secret files' ownership to that user. In the example [docker-compose-with-secrets.yml](examples/docker-compose-with-secrets.yml), user `1234` owns `./secrets/primary.txt` and `./secrets/replicas.txt` and both have `-r--------` permissions.

The same applies to `ENCRYPTION_KEY_FILE` and `API_ADMIN_PASSWORD_FILE`, and to
the `/data` volume, which must be writable by that user.

### App passwords and authentication errors

When using Pi-hole's app passwords ("Configure app password" in the Web interface / API settings page) with nebula-sync, you should enable the Pi-hole setting `webserver.api.app_sudo` on your `REPLICAS` servers or you may receive authentication errors. To configure this setting, perform one of the following:
- From the command line, execute `sudo pihole-FTL --config webserver.api.app_sudo true`. Repeat for each replica.
- From the Pi-hole web UI, go to Settings -> All Settings. Toggle the "Modified settings / All settings" slider in the upper right to show "All settings". Choose the "Webserver and API" section. Check the "Enabled" box under `webserver.api.app_sudo` and then click "Save & Apply". Repeat for each replica.
- With the text editor of your choice, edit the `/etc/pihole/pihole.toml` file. Find the `app_sudo = false` line under `[webserver.api]` and modify it to read `app_sudo = true`. Save the file and restart the pihole-FTL service. Repeat for each replica.

### Other known gaps

- Webhooks cannot be edited after the first start (see above).
- Retry settings are global; they are not part of a profile.
- There is no audit log, and no per-account lockout — repeated failed
  sign-ins are limited by client address, not by username.
- Replica ordering is a number field, not drag and drop.
- The console follows your system's light/dark preference and offers no override.

## Examples

Upstream's env and docker-compose examples are
[here](https://github.com/lovelaze/nebula-sync/tree/main/examples). They still
work as a way to seed a first start, but they use upstream's image name and
describe the environment-only model, so read them alongside
[What is different in this fork](#what-is-different-in-this-fork).

## Disclaimer

This project is an unofficial, community-maintained project and is not affiliated with the [official Pi-hole project](https://github.com/pi-hole). It aims to add sync/replication features not available in the core Pi-hole product but operates independently of Pi-hole LLC. Although tested across various environments, using any software from the Internet involves inherent risks. See the [license](LICENSE) for more details.

Pi-hole and the Pi-hole logo are [registered trademarks](https://pi-hole.net/trademark-rules-and-brand-guidelines) of Pi-hole LLC.
