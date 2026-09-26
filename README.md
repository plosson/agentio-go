# agentio-go

A Go port of [agentio](https://github.com/plosson/agentio), the CLI that gives
LLM agents access to communication, productivity and tracking services as
plain commands that pipe and script.

It has the same commands, output and errors as the Bun CLI, and ships as one
static binary. It covers the encrypted vault, profiles, credential refresh,
the declarative plugin contract, and the credential hub (daemon and admin UI).

Services: Confluence, Discourse, Dropbox, Falco, GitHub, Gmail, Google
Calendar, Chat, Docs, Drive, Sheets, Slides, Apps Script and Tasks, JIRA,
Revolut, RSS, Slack and SQL. Telegram is not ported yet.

## Relation to the Bun CLI

The Bun CLI ([github.com/plosson/agentio](https://github.com/plosson/agentio))
is the reference implementation. This port follows its behaviour; comments
that name a `src/…` file refer to that repository.

The two CLIs share one vault. The on-disk format is the same: base64 of salt,
a 16-byte IV and AES-256-GCM ciphertext, with scrypt N=16384, r=8, p=1. A
vault written by one CLI opens in the other, and each keeps the keys it does
not know about. Both use the same `~/.config/agentio` directory: the vault
pointer, the passphrase file and the hub token.

## Install

```bash
go install github.com/plosson/agentio-go/cmd/agentio@latest
```

This installs a binary called `agentio`, the same name as the Bun CLI. If you
have both, the one earlier on your `PATH` wins.

## Build and test

| Command | What it does |
| --- | --- |
| `make build` | Build `./bin/agentio`, with the version from `git describe` |
| `make test` | `go test -race -count=1 ./...` |
| `make lint` | gofmt check, `go vet` and staticcheck |
| `make fmt` | `gofmt -w .` |
| `go run ./cmd/agentio …` | Run from source (version `0.0.0-dev`) |

The command reference is generated: run `agentio --help`,
`agentio <service> --help`, or `agentio docs` for the full reference written
for LLMs.

## Try it without a real vault

A home directory where agentio is already configured holds a live vault. Try
the binary under an empty home:

```bash
export HOME=$(mktemp -d)
export AGENTIO_PASSPHRASE='test-pass-123'
go run ./cmd/agentio vault init --passphrase "$AGENTIO_PASSPHRASE"
go run ./cmd/agentio rss info https://example.com/feed.xml
```

`--help`, `docs`, `plugin list` and `skill` only print text. Other commands
open the vault that the home points at. `logout` deletes the token file.
`vault reset --force`, `vault clear --force`, `vault import`,
`vault passphrase` and `vault set` rewrite the vault.

## Hub

```bash
agentio daemon start
```

The hub listens on `0.0.0.0:7890`. With `AGENTIO_PASSPHRASE` set, the vault
unlocks at startup. Otherwise, open `http://127.0.0.1:7890/ui` and unlock it
there. `GET /health` answers while the vault is locked.

## Layout

| Path | Contents |
| --- | --- |
| `cmd/agentio/` | Entry point |
| `internal/cli/` | Command tree, help, `docs`/`skill`, vault, key, daemon and status commands |
| `internal/plugins/` | Plugin contract and registry; one folder per service (Google services under `google/`) |
| `internal/host/` | Runs plugin commands: profiles, prompts, read-only checks, output |
| `internal/vault/`, `internal/profile/`, `internal/auth/` | Encrypted vault, profiles, credentials and refresh |
| `internal/daemon/` | HTTP hub: health, credential API, admin UI (`ui/index.html`) |
| `internal/testbox/` | Test isolation helpers |
| other `internal/` packages | Shared helpers (errors, JavaScript value semantics, retries, caches) |

## Adding a service

1. Create `internal/plugins/<service>/` with a `New() *plugins.Plugin` that
   declares the commands, credentials and refresh rules (see
   `internal/plugins/contract.go`, and `internal/plugins/rss/` for a small
   example).
2. Register it in `registerServices` in `internal/cli/root.go`.
3. Add tests next to it that call `testbox.Isolate(t)` and fake the service
   over HTTP.

When the service exists in the Bun CLI, keep its commands, credential JSON and
errors the same, so both CLIs can share the vault.

## License

MIT. See [LICENSE](LICENSE).
