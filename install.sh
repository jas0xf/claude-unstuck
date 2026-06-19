#!/bin/sh
# claude-unstuck installer — downloads the latest release binary for this
# platform. Usage: curl -fsSL https://raw.githubusercontent.com/jas0xf/claude-unstuck/main/install.sh | sh
set -eu

REPO="jas0xf/claude-unstuck"

os=$(uname -s | tr '[:upper:]' '[:lower:]')
case "$os" in
  linux|darwin) ;;
  *) echo "Unsupported OS: $os (Windows: download the .zip from GitHub Releases)" >&2; exit 1 ;;
esac

arch=$(uname -m)
case "$arch" in
  x86_64|amd64) arch=amd64 ;;
  arm64|aarch64) arch=arm64 ;;
  *) echo "Unsupported architecture: $arch" >&2; exit 1 ;;
esac

# Read the whole API response first, then parse it — piping curl straight into
# `grep -m1` makes grep close the pipe early, which makes curl print a harmless
# but alarming "Failed writing body" (broken pipe).
release=$(curl -fsSL "https://api.github.com/repos/$REPO/releases/latest")
tag=$(printf '%s\n' "$release" | grep -m1 '"tag_name"' | cut -d'"' -f4)
[ -n "$tag" ] || { echo "Could not determine latest release" >&2; exit 1; }
ver=${tag#v}

url="https://github.com/$REPO/releases/download/$tag/claude-unstuck_${ver}_${os}_${arch}.tar.gz"
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

echo "Downloading claude-unstuck $tag ($os/$arch)..."
curl -fsSL "$url" -o "$tmp/cu.tar.gz"
tar -xzf "$tmp/cu.tar.gz" -C "$tmp"

dest="/usr/local/bin"
if [ ! -w "$dest" ]; then
  dest="$HOME/.local/bin"
  mkdir -p "$dest"
fi
install -m 0755 "$tmp/claude-unstuck" "$dest/claude-unstuck"
echo "Installed to $dest/claude-unstuck"
case ":$PATH:" in
  *":$dest:"*) ;;
  *) echo "NOTE: add $dest to your PATH" ;;
esac
"$dest/claude-unstuck" version
echo "Try: claude-unstuck doctor"
