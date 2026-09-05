# portadmin

An interactive terminal UI for the ports on your machine. It shows every process
listening on a TCP port, tells you which project it belongs to, and kills the one
you no longer need with a single keypress — no more `lsof -i :3000` followed by
`kill -9`.

Published Docker ports show the container that owns them instead of the runtime's
proxy process, so a port is never just "OrbStack Helper" or "com.docker.backend".

```
portadmin  ·  local ports at a glance

  PORT    PROJECT              COMMAND                                    PID     UPTIME
 ─────── ──────────────────── ────────────────────────────────────────── ─────── ─────────
  3000    storefront           next dev                                   48213   1m30s
  4000    @acme/api            node --enable-source-maps dist/main.js     53840   20h44m
  5173    admin-dashboard      vite                                       61207   12m04s
  5432    shop-stack           docker: shop-postgres · postgres:16        1188    2d1h
  6379    shop-stack           docker: shop-redis · redis:7               1188    2d1h

Stopped container shop-redis.
↑/↓ or j/u move · k/enter/x kill (again within 2s = force) · r refresh · q quit
```

## Install

```sh
go install github.com/Mozolas/portadmin@latest
```

Make sure `$(go env GOPATH)/bin` is on your `PATH`, then run:

```sh
portadmin
```

Pinning a version works too:

```sh
go install github.com/Mozolas/portadmin@v0.1.0
```

Prebuilt binaries for macOS and Linux (amd64 and arm64) are attached to every
[release](https://github.com/Mozolas/portadmin/releases):

```sh
tar -xzf portadmin_v0.1.1_darwin_arm64.tar.gz
xattr -d com.apple.quarantine portadmin   # macOS only
mv portadmin ~/.local/bin/                # or anywhere on your PATH
```

Or build from a clone:

```sh
git clone https://github.com/Mozolas/portadmin.git
cd portadmin
go build -o portadmin .
./portadmin
```

## Usage

`portadmin` takes no flags and needs no configuration — just run it.

Rows are grouped by project, so every port one project holds sits together; the
groups themselves are ordered by their lowest port.

| Key                     | Action                                                          |
| ----------------------- | --------------------------------------------------------------- |
| `↑` / `↓`               | Move the selection; it wraps around at both ends                  |
| `j` / `u`               | Move down / up                                                    |
| `g` / `G` (`home`/`end`)| Jump to the first / last row                                      |
| `k`, `enter` or `x`     | Stop the selected row: `SIGTERM` for a process, `docker stop` for a container |
| the same key again within 2s | Escalate to `SIGKILL` / `docker kill`                        |
| `r`                     | Refresh now                                                       |
| `q` / `esc`             | Quit                                                              |

The table refreshes automatically every 2 seconds, and the selection stays on the
same process across refreshes as long as it is still listening. Once it stops —
killed from here or on its own — the selection lands on the row above it rather
than back at the top.

Killing a container never signals the host process holding the port — that
process belongs to the container runtime, and signalling it would take down every
other container with it.

## Docker containers

If a local Docker engine is reachable, `portadmin` asks it which containers
publish which ports and rewrites those rows:

- **PROJECT** is the Compose project (`com.docker.compose.project`), or the
  container name for containers started outside Compose;
- **COMMAND** shows `docker: <container> · <image>`;
- **UPTIME** is how long the container has been running, not how long the
  runtime's proxy process has been up;
- published ports with no visible host process still get a row, with `-` in the
  PID column.

The engine socket is found via `DOCKER_HOST` or the usual locations of Docker
Desktop, OrbStack, Colima, Rancher Desktop and plain `dockerd`. No Docker means
no container rows — everything else works as before.

## How the project name is resolved

For every listening process `portadmin` reads its working directory and then:

1. looks for a `package.json` in that directory (walking up to 4 levels, so a dev
   server started from `apps/api` still shows the name of the nearest package),
   and uses its `name` field;
2. otherwise falls back to the name of the working directory itself.

Processes running from `/` — typically system daemons — show no project name.

## Notes

- macOS and Linux only.
- You only see the processes you are allowed to inspect. Run under `sudo` if you
  need to see and kill processes owned by another user.
- On macOS the working directory is not exposed by the process API, so
  `portadmin` falls back to a single batched `lsof` call for those processes.

## Development

```sh
go test ./...                            # port parsing, project names, formatting, signals, Docker API and UI
go vet ./...
.github/scripts/next-version_test.sh     # the release versioning rules
```

## Releases

Every push to `main` runs the tests on Linux and macOS and then releases
automatically. The version is derived from the commit subjects since the last
tag:

| Commit subject                          | Effect on `v0.4.1` |
| --------------------------------------- | ------------------- |
| `feat: …`                               | `v0.5.0`            |
| `fix: …`, `perf: …`, `refactor: …`      | `v0.4.2`            |
| anything else without a known prefix    | `v0.4.2`            |
| `docs: …`, `chore: …`, `ci: …`, `test: …`, `style: …`, `build: …` | no release |
| `feat!: …` or a `BREAKING CHANGE:` trailer | `v0.5.0` while below 1.0, `v1.0.0`-style major bump after |

Below `v1.0.0` a breaking change raises the minor, as semver intends: the major
stays at zero until the tool is declared stable. Add `[skip release]` to a commit
message to publish nothing at all.

A release builds `darwin/amd64`, `darwin/arm64`, `linux/amd64` and `linux/arm64`
with the version stamped into the binary (`portadmin --version`), attaches the
archives and their SHA-256 checksums, and writes release notes from the commit
subjects.

## License

MIT — see [LICENSE](LICENSE).
