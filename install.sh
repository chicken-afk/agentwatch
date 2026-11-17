#!/bin/sh
# agentwatch installer: downloads the latest release for this machine, checks its
# checksum, and installs it to ~/.local/bin (no Go needed).
#
#   curl -fsSL https://raw.githubusercontent.com/chicken-afk/agentwatch/main/install.sh | sh
#
# Options (environment variables):
#   AGENTWATCH_INSTALL_DIR  where to put the binary (default: ~/.local/bin)
#   AGENTWATCH_VERSION      a release tag such as v0.2.0 (default: the latest release)
set -eu

REPO="chicken-afk/agentwatch"
BASE_URL="${AGENTWATCH_BASE_URL:-https://github.com/$REPO/releases}"
INSTALL_DIR="${AGENTWATCH_INSTALL_DIR:-$HOME/.local/bin}"
VERSION="${AGENTWATCH_VERSION:-latest}"

say() { printf '%s\n' "$*"; }
fail() { printf 'agentwatch install: %s\n' "$*" >&2; exit 1; }

# --- which build fits this machine ---------------------------------------------
case "$(uname -s)" in
  Darwin) os=darwin ;;
  Linux) os=linux ;;
  *) fail "unsupported system $(uname -s) (macOS and Linux only; on other systems: go install github.com/$REPO@latest)" ;;
esac
case "$(uname -m)" in
  x86_64 | amd64) arch=amd64 ;;
  arm64 | aarch64) arch=arm64 ;;
  *) fail "unsupported CPU $(uname -m) (amd64 and arm64 only)" ;;
esac
asset="agentwatch_${os}_${arch}.tar.gz"

if [ "$VERSION" = latest ]; then
  url="$BASE_URL/latest/download"
else
  url="$BASE_URL/download/$VERSION"
fi

# --- tools ----------------------------------------------------------------------
if command -v curl >/dev/null 2>&1; then
  fetch() { curl -fsSL "$1" -o "$2"; }
elif command -v wget >/dev/null 2>&1; then
  fetch() { wget -q "$1" -O "$2"; }
else
  fail "needs curl or wget"
fi
if command -v sha256sum >/dev/null 2>&1; then
  sha256() { sha256sum "$1" | cut -d' ' -f1; }
elif command -v shasum >/dev/null 2>&1; then
  sha256() { shasum -a 256 "$1" | cut -d' ' -f1; }
else
  fail "needs sha256sum or shasum to verify the download"
fi
command -v tar >/dev/null 2>&1 || fail "needs tar"

# --- download, verify, install ---------------------------------------------------
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT INT TERM

say "Downloading $asset ($VERSION) …"
fetch "$url/$asset" "$tmp/$asset" ||
  fail "could not download $url/$asset (is there a published release for $os/$arch?)"
fetch "$url/checksums.txt" "$tmp/checksums.txt" || fail "could not download checksums.txt"

want="$(grep " $asset\$" "$tmp/checksums.txt" | cut -d' ' -f1)"
[ -n "$want" ] || fail "$asset is not listed in checksums.txt"
got="$(sha256 "$tmp/$asset")"
[ "$want" = "$got" ] || fail "checksum mismatch for $asset: the download is corrupt or was tampered with"

tar -xzf "$tmp/$asset" -C "$tmp" agentwatch || fail "could not unpack $asset"
mkdir -p "$INSTALL_DIR"
mv "$tmp/agentwatch" "$INSTALL_DIR/agentwatch.new"
chmod 755 "$INSTALL_DIR/agentwatch.new"
mv "$INSTALL_DIR/agentwatch.new" "$INSTALL_DIR/agentwatch"
say "Installed $("$INSTALL_DIR/agentwatch" version) to $INSTALL_DIR/agentwatch"

# --- PATH ------------------------------------------------------------------------
case ":$PATH:" in
  *":$INSTALL_DIR:"*)
    say ""
    say "Run it with:  agentwatch"
    ;;
  *)
    line="export PATH=\"$INSTALL_DIR:\$PATH\""
    case "$(basename "${SHELL:-sh}")" in
      zsh) add="echo '$line' >> ~/.zshrc && . ~/.zshrc" ;;
      bash) add="echo '$line' >> ~/.bashrc && . ~/.bashrc" ;;
      fish) add="fish_add_path $INSTALL_DIR" ;; # persists by itself in fish
      *) add="echo '$line' >> ~/.profile && . ~/.profile" ;;
    esac
    say ""
    say "$INSTALL_DIR is not on your PATH yet. Add it once with:"
    say ""
    say "  $add"
    say ""
    say "Then run:  agentwatch"
    ;;
esac
say "Using Claude Code? Also run once:  agentwatch install"
