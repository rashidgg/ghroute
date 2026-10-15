# Publishing the Homebrew package

The selected tap is `rashidgg/homebrew-tap`; users refer to it as `rashidgg/tap`.
The cask and tap README are in `packaging/homebrew`. They are prepared locally;
neither the tap nor the release has been published by this work.

## Package design

The package uses Homebrew's native cask DSL and checksummed macOS binaries.
`installer script` calls ghroute's noninteractive `setup` command; `uninstall
script` calls `uninstall` before Homebrew removes the executable. Both run as the
current user without sudo. Homebrew installs `gh` as a dependency; Git comes from
the developer tools required by Homebrew. Users do not need Go.

A formula cannot provide this lifecycle: formula install hooks use an isolated
home, and formulas have no supported user cleanup hook on uninstall. A cask's
documented vendor installer hooks support the required automatic setup and
cleanup. No GoReleaser, CI release workflow, or additional installer framework is
used. Go supplies the small application-specific integration commands.

Standard Go linker flags supply the version and `ManagedBinary=self`. Setup
links `~/.local/bin/ghroute` to the package's staged executable; each upgrade
refreshes that link. The cask deliberately has no additional `binary` artifact:
setup manages PATH, and there is no later prefix-link operation that could fail
after activating routing. The official CLI is located through standard Homebrew
paths, then PATH, excluding ghroute's own aliases.

Homebrew runs the same uninstall hook during upgrades. A generated `disabled`
marker preserves an explicit off choice across this cycle; the removable
`install.json` receipt tracks integrations. `brew uninstall ghroute` removes the
integrations and package, while `brew uninstall --zap ghroute` also removes the
marker and account metadata cache. Neither removes repository identities or
unrelated SSH/Git settings.

## Preparing a release

Run tests before building. Replace the version consistently for each release:

```sh
go test ./...
mkdir -p dist/arm64 dist/amd64
CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -trimpath -ldflags '-s -w -X github.com/rashidgg/ghroute/internal/app.Version=0.1.0 -X github.com/rashidgg/ghroute/internal/install.ManagedBinary=self' -o dist/arm64/ghroute ./cmd/ghroute
CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -trimpath -ldflags '-s -w -X github.com/rashidgg/ghroute/internal/app.Version=0.1.0 -X github.com/rashidgg/ghroute/internal/install.ManagedBinary=self' -o dist/amd64/ghroute ./cmd/ghroute
COPYFILE_DISABLE=1 tar -czf dist/ghroute-0.1.0-darwin-arm64.tar.gz -C dist/arm64 ghroute
COPYFILE_DISABLE=1 tar -czf dist/ghroute-0.1.0-darwin-amd64.tar.gz -C dist/amd64 ghroute
shasum -a 256 dist/ghroute-0.1.0-darwin-*.tar.gz
```

Copy each exact SHA-256 into its architecture entry in `Casks/ghroute.rb`.
Validate downloaded binaries under macOS Gatekeeper before publication. The
current local build does not provide Developer ID signing or notarization;
distribution validation must not disable quarantine or system protections.
Only Apple silicon execution has been tested on this machine; Intel is cross-built.

Local validation passed native cask install, reinstall while off, cleanup-error
retention, uninstall, and zap using the locally built executable as the temporary
cask's vendor command. The unmodified default-quarantine install held the unsigned
payload before program startup and was stopped. Those hook tests do not establish
that the downloadable release passes Gatekeeper. Obtain Developer ID signing and
notarization, rebuild the archives and checksums, and repeat the unmodified
download/install check before publishing for users.

Validate the tap using native Homebrew commands:

```sh
brew style rashidgg/tap/ghroute
brew audit --cask --strict --online rashidgg/tap/ghroute
brew install rashidgg/tap/ghroute
ghroute off
brew reinstall ghroute
ghroute on
brew uninstall ghroute
```

Run lifecycle checks with an isolated HOME and Git configuration so they cannot
change a developer's real shell or credentials. Check that reinstall preserves
off, the shim invokes official `gh`, and uninstall removes only owned links,
helper entries, and PATH blocks. Casks have no formula-style `test do` stanza;
the Go tests cover the vendor lifecycle, including partial failure cleanup.
Before publication, use a temporary local tap and substitute local `file://`
archive URLs. Public URL and downloaded-artifact checks must wait for publication.

## Publishing the first release and tap

The source repository is `rashidgg/ghroute`, with default branch `main`.
Release and tap publication remain separate steps, after signing and downloaded
artifact validation are complete:

1. Tag the tested source as `v0.1.0` and create the matching GitHub release.
2. Upload both architecture archives, retaining their exact bytes and checksums.
3. Publish `packaging/homebrew` to `rashidgg/homebrew-tap`.
4. Validate the public installation and removal, then remove pending-publication
   notices from project and tap documentation.

No token belongs in the package or either repository. Use the official GitHub
CLI's existing authentication for publication.

References: [Homebrew's tap guide](https://docs.brew.sh/How-to-Create-and-Maintain-a-Tap)
and [Cask Cookbook](https://docs.brew.sh/Cask-Cookbook).
