# ghroute Homebrew tap

The right GitHub account for each repository, with automatic setup and cleanup.

This directory is the source for `rashidgg/homebrew-tap`. Publication is pending;
the commands below become available once the tap and release are public.

## Install

```sh
brew install rashidgg/tap/ghroute
```

Open a new terminal, sign in to your accounts with `gh auth login`, and use `gh`
as usual. The package installs the official GitHub CLI, sets up routing, and adds
`~/.local/bin` to your login shell's PATH. Fish, Zsh, Bash, and sh are supported.
No compiler or account configuration file is needed.

## Update

```sh
brew update
brew upgrade ghroute
```

## Turn routing off or on

```sh
ghroute off
ghroute on
```

Off restores normal `gh` behavior and your previous Git credential helpers while
keeping ghroute available. Upgrades and reinstalls remember that choice.

## Remove

```sh
brew uninstall ghroute
```

Removal cleans up ghroute's command links, Git helper, and marked PATH entries.
Open a new terminal afterward. Add `--zap` to also remove the remembered off
preference and metadata cache. SSH settings, existing Git rules, and repository
identities are preserved.

The package uses Homebrew's native cask installer and uninstall hooks, so setup
and cleanup happen with the package operation. No extra activation command is
needed. See the [project](https://github.com/rashidgg/ghroute) for details.
