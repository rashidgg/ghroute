cask "ghroute" do
  arch arm: "arm64", intel: "amd64"

  version "0.1.0"
  sha256 arm:   "81863d32d6074f289cb4b44e4a9003c7f43e578b1dd1373d74f5e811c04b5915",
         intel: "fb65e5f2cdadbbf6357bc2aed66748c4d464f5ce976d1fcb24911070a1764432"

  url "https://github.com/rashidgg/ghroute/releases/download/v#{version}/ghroute-#{version}-darwin-#{arch}.tar.gz"
  name "ghroute"
  desc "Select the right GitHub account for each repository"
  homepage "https://github.com/rashidgg/ghroute"

  depends_on formula: "gh"

  installer script: {
    executable: "ghroute",
    args:       ["setup"],
    sudo:       false,
  }

  uninstall script: {
    executable: "ghroute",
    args:       ["uninstall"],
    sudo:       false,
  }

  zap trash: [
    "~/.cache/ghroute",
    "~/.local/state/ghroute",
  ]

  caveats <<~EOS
    ghroute sets up routing and adds ~/.local/bin to your shell's PATH.
    Open a new terminal, then use gh as usual.

    Turn routing off or on with: ghroute off / ghroute on
    Removal also cleans up shell and Git integration: brew uninstall ghroute
    Add --zap to also remove the remembered off preference and metadata cache.
  EOS
end
