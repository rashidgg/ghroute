# Using ghroute

ghroute selects an account for each command. Authentication remains with the
official GitHub CLI, and the real active account is never switched automatically.

## Installation

ghroute supports GitHub.com on macOS with Apple silicon or Intel Homebrew.
GitHub Enterprise routing remains outside this version's scope. The integration
has been tested with `gh` 2.102.0.

The Homebrew cask is prepared for `rashidgg/tap`; it is not published yet. After
publication:

```sh
brew install rashidgg/tap/ghroute
```

Homebrew installs the official `gh` CLI and automatically sets up ghroute.
No compiler, source checkout, or separate activation command is needed. Sign in
to your accounts with `gh auth login`; `gh auth status` shows the saved accounts.

From a source checkout, with Go 1.22 or later installed:

```sh
go test ./... && go run ./cmd/ghroute on
```

Activation:

1. Installs `~/.local/bin/ghroute` and points `~/.local/bin/gh` at it. Homebrew
   builds use a symlink to the package's staged executable; upgrades refresh
   that link automatically. Source builds copy the running executable.
2. Records generated ownership state in `~/.local/state/ghroute/install.json`,
   with mode `0600`. This is not a user-edited configuration file.
3. Adds a marked PATH block to your login shell's startup files, placing
   `~/.local/bin` first. Open a new terminal afterward.
4. Enables `credential.https://github.com.useHttpPath` so Git includes the
   repository path when requesting credentials.
5. Appends an empty GitHub HTTPS credential helper to reset earlier helpers,
   followed by the absolute `ghroute credential` helper path.

New installations leave Homebrew's official `gh` symlink and upgrade policy
unchanged. Packaged and source builds look in the standard Apple silicon and
Intel Homebrew locations, then PATH, skipping ghroute's own shim to avoid recursion.

Repeat activation updates the executable without duplicating PATH blocks or
credential helper entries. Existing SSH configuration, URL rewrites, and
conditional identity includes are preserved. No global Git name or email is set.

### PATH and shells

By default, activation uses `$SHELL`. To activate a different shell:

```sh
ghroute on --shell zsh
```

Supported startup files:

| Shell | Files |
| --- | --- |
| Fish | `$XDG_CONFIG_HOME/fish/config.fish`, or `~/.config/fish/config.fish` |
| Zsh | `$ZDOTDIR/.zprofile` and `$ZDOTDIR/.zshrc`, or the same files in `~` |
| Bash | `~/.bashrc` and the first existing login file: `.bash_profile`, `.bash_login`, or `.profile`; creates `.bash_profile` if none exist |
| sh | `~/.profile` |

Existing contents, permissions, and dotfile symlinks are preserved. To manage
PATH yourself, use `ghroute on --no-path` and put `~/.local/bin` before the
official `gh` directory. No routing configuration is needed.

Homebrew's cask installer automatically performs this setup for the current user.
It uses ghroute's installer command without elevated privileges.

An installer cannot modify its parent shell's environment. Open a new terminal
for the startup change to take effect. GUI applications and coding agents must
also receive the updated PATH; restart them from an updated shell if necessary.
Use `command -v gh` to confirm that `~/.local/bin/gh` is selected.

### Existing installations

Installations made by the original Mac-specific version retain their recorded
Homebrew symlink override and `brew pin gh`, including during reinstall. Their
original target remains available for uninstall. New installations use only the
user PATH shim. To migrate an old installation, deactivate it first, then install
the Homebrew package:

```sh
~/.local/bin/ghroute uninstall
brew install rashidgg/tap/ghroute
```

The installer replaces the retained standalone copy with its managed alias.
These Homebrew commands require the pending tap publication. The cask uninstall
and reinstall cycle also migrates legacy integrations to PATH-only routing.

## Choosing an account

An explicit `-R` or `--repo` is considered first. Supported positional targets
include `gh repo clone`, `view`, `sync`, `delete`, and `create`, an owner supplied
to `gh repo list`, and `gh api repos/OWNER/REPO` paths. For `gh repo rename`, use
`-R` to select the repository; the normal positional argument is the new name.
`GH_REPO` supplies a fallback when the command does not provide a target.

Text supplied to body, title, message, notes, or comment flags is not a routing
target. Arguments after a lone `--` are also ignored for routing. The real CLI
still receives the original arguments and validates the command.

Without an explicit target, ghroute checks the remote marked `gh-resolved=base`,
then `upstream`, then `origin`. API `{owner}` and `{repo}` placeholders resolve
from `GH_REPO` or the checkout's repository context. Explicit URL hosts take
priority over `GH_HOST`; ordinary repository commands infer their host from the
checkout before falling back to `GH_HOST`.

Account and organization names match case-insensitively. A matching account login
wins over organization membership. When no account matches, or several accounts
belong to the same organization, ghroute probes the repository using each saved
account. Exactly one account must have access. A network or authentication failure
stops selection rather than being treated as evidence that another account should
be used. A successful unique result is remembered in the cache.

Owner-only commands cannot be probed without a repository. If an owner cannot be
matched uniquely, ghroute reports that limitation.

Outside a repository, with no explicit target or inherited token, ghroute uses
the active GitHub.com account. Authentication management, help, version, and
completion commands pass through to the real CLI. Other hosts also pass through.

## Forks and worktrees

`gh repo fork` uses the active account as the destination, or the account that
matches `--org`. The source repository does not choose the destination account.
The source argument is retained for the official CLI's fork and clone behavior.

Once the fork exists:

- HTTPS operations on `origin` use the account selected for the fork owner.
- HTTPS operations on `upstream` use the account selected for the original owner.
- `gh pr create` uses `origin` so the selected account can push the fork's branch.
- Use `-R owner/repo` when you want to target the upstream repository explicitly.

Each routed command receives its own token environment. Parallel terminals,
worktrees, and agents do not change the global active login. Worktrees share the
clone's Git configuration; ghroute serializes its initial identity writes using
the common Git directory.

## Commit identity and SSH

For routed invocations in a checkout with no local `user.email`, ghroute derives
local identity from the account selected for `origin`. It uses the GitHub profile
name, falling back to the login, and `ID+LOGIN@users.noreply.github.com` for email.
An existing local email prevents ghroute from changing either identity field.
Fork creation defers initialization until a later invocation sees the final fork
origin.

SSH continues to choose its own keys. ghroute does not rewrite SSH configuration
or add Git `insteadOf` rules.

Two compatibility defaults from the original installation remain in this
version: the SSH alias `github.com-rashidrefs` is recognized as GitHub.com, and
the `rashidrefs` account uses `rashid@refs.me` when its profile has no public
email. Other accounts use the normal noreply rule. Other custom SSH aliases are
not automatically resolved.

## Credentials and inherited tokens

Account discovery uses `gh auth status --json hosts`. Token retrieval uses the
real `gh auth token --hostname github.com --user LOGIN`, with all four GitHub
token environment variables removed for that lookup. ghroute does not read
tokens from `hosts.yml` or persist them in its own files.

An outside `GH_TOKEN` or `GITHUB_TOKEN` is preserved when the command has no
explicit repository. An explicit repository overrides inherited tokens. Routed
children receive `GH_TOKEN` and `GHROUTE_ROUTER=1`, with conflicting token
variables removed. A routed child with no repository context retains its parent's
selected account.

`gh auth token --user LOGIN` returns that user's stored token without requiring
account discovery or local identity initialization. Without `--user`, the token
command follows repository routing. These commands intentionally output a token;
use `gh api user --jq .login` when you only want to check the selected account.

Both the Git credential helper and `gh auth git-credential get` select from the
requested URL or host and path. They return Git's credential protocol response
with a terminating blank line. `store` and `erase` pass through to the real CLI.

## Refreshing learned accounts

Account IDs, organization memberships, profiles, and unique probe results live in
`~/.cache/ghroute/cache.json`, mode `0600`. This generated metadata is valid for
six hours; it is not a file to edit. Live auth status determines the active login,
and a changed set of saved logins invalidates the cache immediately. Cache updates
are locked and written atomically for concurrent processes.

To refresh immediately after an organization membership change:

```sh
~/.local/bin/ghroute refresh
```

## Troubleshooting

**The selected account is unexpected.** Run `gh api user --jq .login` in the
checkout. Check explicit repository flags, `GH_REPO`, inherited tokens, and the
configured base, upstream, and origin remotes. `gh auth status` reports the real
active login; it does not report ghroute's per-repository choice.

**Several accounts can access the repository.** ghroute will not guess. A known
account owner or unique organization membership can resolve ownership; otherwise
the command needs an unambiguous account. For a deliberate one-off operation,
invoke `"$(brew --prefix gh)/bin/gh"` directly with your chosen authentication.

**An account's token cannot be read.** Check that account with `gh auth status`
and sign it in again through the official CLI. ghroute fails rather than falling
back to the active account.

**Git reports that the request needs `owner/repo`.** Ensure
`credential.https://github.com.useHttpPath` is `true`. Installation enables it;
without it, Git may ask for a host-wide credential that cannot be routed safely.

**The official CLI runs instead of the router.** Check `command -v gh`, then
open a new terminal after `ghroute on`. Check for a shell alias or a later
PATH override in your startup files. GUI applications need the updated PATH too.
For legacy installations whose Homebrew link changed, restore the recorded
Homebrew target before reinstalling; ghroute refuses to overwrite an unexpected
link.

## Turning routing off

Use `ghroute off` to restore normal `gh` behavior and the previous Git credential
helpers while keeping ghroute available. The shim, executable, and PATH setup
remain; the shim passes commands straight to the official CLI without account
discovery, token selection, or identity changes. Existing shells keep working.

Run `ghroute on` to enable routing again. Homebrew upgrades do not reactivate it
after it has been turned off. A generated `~/.local/state/ghroute/disabled` marker
remembers this choice across upgrades and reinstalls; there is no config file to
edit. `install` remains an alias for `on`.
Use `uninstall` when you also want to remove the shim and PATH setup.

## Uninstalling

From any directory:

```sh
~/.local/bin/ghroute uninstall
```

Uninstall removes `~/.local/bin/gh`, ghroute's exact PATH blocks, and its
credential helper together with the adjacent empty reset it added. Original
helpers, including preexisting empty resets, remain in order. Later edits outside
the PATH blocks are preserved. If a managed block was edited, uninstall asks you
to restore or remove that block rather than discarding those changes.

For a Homebrew installation, a single command runs that cleanup and removes the
package:

```sh
brew uninstall ghroute
```

Add `--zap` to also remove the metadata cache and remembered off preference.
Cleanup failure stops ordinary removal, preserving the executable for a retry.

The Homebrew alias at `~/.local/bin/ghroute` is removed during uninstall;
standalone copied executables remain. Legacy installations also unpin `gh` and
restore their recorded Homebrew symlink.

SSH configuration, URL rewrites, conditional identity includes, the metadata
cache, `useHttpPath`, and any initialized local commit identities remain. The
restoration state is removed once uninstall succeeds. Open a new terminal to
refresh your shell's PATH.
