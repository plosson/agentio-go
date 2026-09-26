# agentio-go — repository guide

## What this is

A Go port of the Bun `agentio` CLI
([github.com/plosson/agentio](https://github.com/plosson/agentio)): a CLI that
gives LLM agents access to services such as Gmail, Google Drive, Slack, JIRA,
GitHub, Dropbox, Revolut and SQL, as plain commands that pipe and script.

The Bun CLI is the reference implementation. This port keeps its commands,
output, errors and vault format, so both CLIs share one vault. Comments that
name a `src/…` or `tests/…` file refer to the Bun repository.

- **Profiles.** Every service supports several named accounts.
- **One encrypted vault.** All configuration and credentials live in one
  passphrase-encrypted file, byte-compatible with the Bun CLI.
- **A hub.** `agentio daemon start` serves credentials to remote agents and
  hosts the admin UI.

## Commands

| Command | What it does |
| --- | --- |
| `make build` | Build `./bin/agentio`; version from `git describe --tags --always --dirty` |
| `make test` | `go test -race -count=1 ./...` |
| `make lint` | gofmt check, `go vet ./...`, staticcheck (pinned in the Makefile) |
| `make fmt` | `gofmt -w .` |
| `make golden BUN_REPO=…` | Regenerate the Bun goldens (see below) |
| `go run ./cmd/agentio …` | Run from source |

CI (`.github/workflows/ci.yml`) also checks `go mod tidy` and
`CGO_ENABLED=0 go build ./...`, on Linux and macOS. The binary must stay
CGO-free.

The command reference is generated, so do not copy it into documentation:
`agentio --help`, `agentio <service> --help`, `agentio docs`.

## Layout

| Path | Contents |
| --- | --- |
| `cmd/agentio/` | Entry point |
| `internal/cli/` | Command tree, Commander-style help, `docs`/`skill`, vault, key, daemon, status |
| `internal/plugins/contract.go`, `registry.go` | The plugin contract and the catalog |
| `internal/plugins/<service>/` | One folder per service; Google services under `google/` |
| `internal/plugins/acme`, `board`, `ping` | Fake plugins, registered by tests only |
| `internal/host/` | Runs plugin commands: profiles, prompts, read-only checks, output |
| `internal/vault/`, `internal/profile/`, `internal/auth/` | Encrypted vault, profiles, credentials and refresh |
| `internal/daemon/` | HTTP hub; `ui/index.html` is the admin UI, owned by this repository |
| `internal/jsvalue/`, `internal/nodefs/` | JavaScript and Node semantics the output depends on (key order, numbers, file errors) |
| `internal/testbox/` | Test isolation and fake-network helpers |
| `internal/golden/` | Bun goldens for parity tests, and their regeneration |

## Adding a service

1. Add `internal/plugins/<service>/` with `New() *plugins.Plugin` declaring its
   commands, credentials and refresh rules. `internal/plugins/rss/` is a small
   example.
2. Register it in `registerServices` in `internal/cli/root.go`.
3. If the Bun CLI has the service, keep its command names, options, credential
   JSON and messages identical, so a vault written by either CLI works with the
   other.

## Conventions

- Standard Go formatting (`gofmt`); code must pass `go vet` and staticcheck.
- Package and file names are lowercase; file names use underscores.
- Errors a user sees carry a code, a message and a suggestion: `clierr.New` in
  the host, the `Fail` function the contract hands to plugins.
- Results go to stdout in a form an LLM can read; progress and errors go to
  stderr.
- Vault writes keep the order of keys and every key the Go types do not model.
  Build credential objects with `jsvalue`, not Go maps, when order matters.

## Testing

- Tests live next to the code (`*_test.go`), and golden files in `testdata/`.
  `internal/plugins/falco/testdata/bun` holds output captured from the Bun CLI.
- Call `testbox.Isolate(t)` at the start of every test that touches the vault,
  profiles, tokens or `HOME`. It points `HOME` at a temp directory, sets
  `AGENTIO_TEST=1` and clears the `AGENTIO_*` variables and process caches.
- With `AGENTIO_TEST=1`, the vault refuses to write outside the OS temp
  directory. Never weaken that guard, and never run a test or the binary
  against a real vault (`~/.config/agentio` or the path in its `vault.path`).
- Tests do not use the network. Fake services with `httptest` servers and the
  `testbox` helpers.
- Tests are adversarial: malformed input, wrong passphrases, cut connections,
  read-only profiles, not only the happy path.

## Bun goldens

Parity with the Bun CLI is checked against output captured from it, kept as
JSON under `testdata/bun/` next to the tests (`internal/cli`,
`internal/daemon`, `internal/vault`) and read through `internal/golden`: the
help of every command, the Commander wrapping, `docs` and `skill`, the vault
bytes each service and vault command writes, the vault wire format and key
order, and the admin UI and hub API responses. Tests never run Bun. The
vaults there are fixtures under test passphrases, not secrets.

When the Bun CLI changes, or a test's cases do, regenerate them from a Bun
checkout with its dependencies installed, and review the diff:

```sh
make golden BUN_REPO=/path/to/agentio   # sets AGENTIO_BUN_REPO for go test
```

Bun groups Go does not have yet (`notYetPorted` in
`internal/cli/help_test.go`) are left out of the capture; once Go has one, the
tests fail until the goldens are regenerated with it.

## Commits and pull requests

- Conventional Commits: `feat:`, `fix:`, `test:`, `docs:`, `ci:`, `build:`,
  `chore:`, `refactor:`.
- A pull request has a short summary and rationale, links to issues, and
  updated documentation when behaviour changes.

## Secrets

Never commit vault files, exported configs or tokens. A vault export is
encrypted but is still a secret.
