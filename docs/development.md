# Developing ghroute

ghroute is a Go 1.22 module using the standard library and the existing `gh`, Git,
and Homebrew commands. It has no third-party Go dependencies.

## Code layout

- `cmd/ghroute/main.go` is the executable entry point and handles top-level errors.
- `internal/app` coordinates command dispatch, repository context, identity, and
  execution of the real CLI. Its repository and execution helpers live alongside
  their tests.
- `internal/model` contains shared account, request, target, and decision types.
  It has no process execution or filesystem responsibilities.
- `internal/process` owns the command runner and GitHub token environment helpers.
  The runner is injected into packages that interact with external commands.
  `locate.go` finds the official CLI while excluding wrapper aliases.
- `internal/parse` extracts routing metadata while preserving the original argv.
- `internal/route` makes pure account-selection decisions.
- `internal/discover` reads account metadata, maintains the cache, and probes
  repository access.
- `internal/credential` reads and writes Git's credential protocol.
- `internal/identity` reads Git remotes and applies local commit identity.
- `internal/install` plans, applies, and restores installation changes. Shell
  startup integration and executable installation live in `path.go` and
  `files.go`; legacy Homebrew ownership remains in the installer.
- `packaging/homebrew` contains the cask and tap README for
  `rashidgg/homebrew-tap`.

Keep selection rules in `route` and side effects in the packages that own them.
The application layer connects these responsibilities; the entry point stays
small. Put tests next to the behavior they exercise.

## Testing

From the project root:

```sh
go test ./...
```

Discovery tests inject fake command runners and API responses. Identity and
installation tests use temporary Git repositories and isolated Git configuration.
They do not require logged-in GitHub accounts or live API access. Token-related
assertions use placeholders and do not print credential values on failure.

When changing cache locking, worktree identity initialization, or other concurrent
behavior, also run:

```sh
go test -race ./...
```

Format Go changes with `gofmt`. Use Go's build and test commands directly; there
is no separate build framework or wrapper to maintain.

## Updating the local installation

Build with Go and run tests before activation:

```sh
go test ./... && go run ./cmd/ghroute on
```

Source installation copies its running executable; packaged installation links
to the staged Homebrew executable. Each cask upgrade runs the vendor uninstall
and setup commands, refreshing the link and preserving the off preference.
Release builds set the version and `ManagedBinary=self` with native Go linker flags.

Shell changes are marked, idempotent, and recorded for uninstall. Homebrew's native
cask lifecycle invokes the application-specific setup and cleanup, using the Go
standard library rather than another installer framework. The `on` command uses
Go's standard `flag` parser; `setup` is the package entrypoint and preserves off.

Installation tests use temporary homes, isolated Git configuration, injected
failures, and shell executables where available. They cover activation, off/on,
package reinstall, atomic Git helper restoration, and cleanup without GitHub
credentials or network access. See [release packaging](releasing.md) and
[usage](usage.md#installation).

## Design choices

Use the native Git configuration commands, official `gh` account/token APIs, and
Go standard-library facilities before introducing another dependency or custom
infrastructure.

The argument scanner is deliberately small. Go's `flag` package stops at the
first positional argument and cannot forward unknown, interspersed `gh` flags.
ghroute therefore extracts only the metadata needed for routing; the real CLI
owns argument validation and execution. Command-specific flag arity belongs in
`internal/parse`, with regression cases for values that could look like targets.

Account selection is separate from token retrieval. A missing selected account
token is a hard error. Probes must identify exactly one account; transport errors
are not proof that an account lacks access. Keep token values out of persisted
state and diagnostic errors.

The original installation's recorded Homebrew override is retained for backward
compatibility. New installs use a user PATH shim and support both standard macOS
Homebrew prefixes. The [original SSH and identity compatibility defaults](usage.md#commit-identity-and-ssh)
remain unchanged; broadening SSH alias discovery is a separate behavior change.
