# ghroute

**The right GitHub account for each repository.**

Keep your work and personal accounts signed in. ghroute selects an account for
each GitHub CLI command and HTTPS Git operation, so you can move between projects
without switching your active login.

Use the commands you already know:

```sh
cd ~/Projects/work-app
gh pr list                  # Uses your work repository's account

cd ~/Projects/side-project
gh pr list                  # Uses your personal repository's account
```

## Built for working across accounts

- **No account map to maintain.** ghroute learns from the accounts and
  organizations you already belong to on GitHub. There is no config file to edit.
- **Work in parallel.** Terminals, Git worktrees, and coding agents can use
  different accounts at the same time, without competing over `gh auth switch`.
- **Keep your workflow.** Use the official `gh` CLI for sign-in and your usual
  commands for pull requests, repository access, and HTTPS fetches and pushes.
- **Clear account choices.** When ownership is uncertain, ghroute checks access
  and proceeds only if it can identify one account. Ambiguous access produces an
  explanation instead of a guess.

## Get started

ghroute supports **GitHub.com on macOS**, with Homebrew on Apple silicon or Intel.

The Homebrew package is prepared for `rashidgg/tap`; publication is pending.
Once published, install and set up routing with one command:

```sh
brew install rashidgg/tap/ghroute
```

Homebrew installs ghroute and the official GitHub CLI. Setup automatically adds
`~/.local/bin` to your shell's PATH and connects GitHub HTTPS credentials.
Open a new terminal, sign in to each account with `gh auth login`, and use `gh`
as usual. Fish, Zsh, Bash, and POSIX sh are supported.

Working from a source checkout today? With Go 1.22 or later and the official `gh`
CLI installed, run:

```sh
go test ./... && go run ./cmd/ghroute on
```

Check which account a repository uses:

```sh
cd /path/to/your/repository
gh api user --jq .login
```

See [installation details](docs/usage.md#installation) for the exact changes and
supported setup.

## Your repository sets the context

An explicit repository takes priority over your working directory:

```sh
gh pr list -R your-org/your-repo
```

Otherwise, ghroute uses the checkout's repository context. With no repository
target or token override, it uses your active account. Fork creation selects the
destination account; creating a pull request from a fork uses the fork owner's
account.

Tokens remain managed by the official `gh` CLI. ghroute retrieves them when
needed and never stores them in its cache or Git configuration. Existing SSH key
selection stays with your SSH setup. GitHub Enterprise account routing is outside
this version's scope.

## Easy to manage

Refresh account and organization information whenever you need to:

```sh
~/.local/bin/ghroute refresh
```

Turn routing off while keeping ghroute installed:

```sh
ghroute off
```

Turn it back on with `ghroute on`. Upgrades remember your choice.

Remove the package and its shell and Git integration in one command:

```sh
brew uninstall ghroute
```

Add `--zap` to also remove the remembered off preference and account metadata
cache. Existing SSH settings and repository identities stay yours.

[Usage and troubleshooting](docs/usage.md) covers forks, commit identity, token
overrides, and uninstall behavior.
[Development](docs/development.md) explains the code layout and test workflow.
[Homebrew packaging](docs/releasing.md) covers the tap and release process.

## License

[MIT](LICENSE) · Copyright Rashid Goshtasbi
