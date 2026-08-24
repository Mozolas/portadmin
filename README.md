# portadmin

An interactive terminal UI for the ports on your machine. It shows every process
listening on a TCP port, tells you which project it belongs to, and kills the one
you no longer need with a single keypress — no more `lsof -i :3000` followed by
`kill -9`.

```
portadmin  ·  local ports at a glance

  PORT    PROJECT              COMMAND                        PID     UPTIME
 ─────── ──────────────────── ────────────────────────────── ─────── ─────────
  3000    storefront           next dev                       48213   1m30s
  4000    @acme/api            node --enable-source-maps …    53840   20h44m
  5432    postgres             postgres -D /usr/local/var/…   1188    2d1h
  5173    admin-dashboard      vite                           61207   12m04s

SIGTERM sent to storefront (PID 48213) — press again within 2s for SIGKILL.
↑/↓ or j/k move · enter/x kill (again within 2s = SIGKILL) · r refresh · q quit
```

## Install

```sh
go install github.com/Mozolas/portadmin@latest
```

Make sure `$(go env GOPATH)/bin` is on your `PATH`, then run:

```sh
portadmin
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

| Key             | Action                                                     |
| --------------- | ---------------------------------------------------------- |
| `↑` / `↓`       | Move the selection                                          |
| `j` / `k`       | Move down / up (vim style)                                  |
| `enter` or `x`  | Send `SIGTERM` to the selected process                      |
| `enter` / `x` again within 2s | Escalate to `SIGKILL` for the same process    |
| `r`             | Refresh now                                                 |
| `q` / `esc`     | Quit                                                        |

The table refreshes automatically every 2 seconds, and the selection stays on the
same process across refreshes as long as it is still listening.

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
go test ./...   # unit tests for port parsing, project names, formatting and signals
go vet ./...
```

## License

MIT — see [LICENSE](LICENSE).
