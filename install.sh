#!/bin/sh
set -eu

REPOSITORY="neptaco/uniforge"
VERSION="${UNIFORGE_VERSION:-latest}"
INSTALL_DIR="${UNIFORGE_INSTALL_DIR:-$HOME/.local/bin}"

usage() {
  cat <<'EOF'
Install UniForge from GitHub Releases.

Usage: install.sh [--version vX.Y.Z] [--install-dir PATH]

Environment variables:
  UNIFORGE_VERSION       Release version or latest
  UNIFORGE_INSTALL_DIR   Destination directory (default: ~/.local/bin)
EOF
}

while [ "$#" -gt 0 ]; do
  case "$1" in
    --version) VERSION="${2:?missing value for --version}"; shift 2 ;;
    --install-dir) INSTALL_DIR="${2:?missing value for --install-dir}"; shift 2 ;;
    -h|--help) usage; exit 0 ;;
    *) echo "Unknown argument: $1" >&2; usage >&2; exit 2 ;;
  esac
done

if [ "$VERSION" != latest ] && ! printf '%s\n' "$VERSION" | grep -Eq '^v[0-9]+\.[0-9]+\.[0-9]+$'; then
  echo "Invalid version: $VERSION (expected latest or vX.Y.Z)" >&2
  exit 2
fi

command -v curl >/dev/null 2>&1 || { echo "curl is required" >&2; exit 1; }
command -v tar >/dev/null 2>&1 || { echo "tar is required" >&2; exit 1; }

case "$(uname -s)" in
  Darwin) os=darwin ;;
  Linux) os=linux ;;
  *) echo "Unsupported operating system: $(uname -s)" >&2; exit 1 ;;
esac
case "$(uname -m)" in
  x86_64|amd64) arch=amd64 ;;
  arm64|aarch64) arch=arm64 ;;
  *) echo "Unsupported architecture: $(uname -m)" >&2; exit 1 ;;
esac

if [ "$VERSION" = latest ]; then
  base_url="https://github.com/$REPOSITORY/releases/latest/download"
else
  base_url="https://github.com/$REPOSITORY/releases/download/$VERSION"
fi
archive="uniforge_${os}_${arch}.tar.gz"
tmp_dir="$(mktemp -d)"
trap 'rm -rf "$tmp_dir"' EXIT INT TERM

echo "Downloading UniForge ${VERSION} for ${os}/${arch}..."
curl -fsSL "$base_url/$archive" -o "$tmp_dir/$archive"
curl -fsSL "$base_url/checksums.txt" -o "$tmp_dir/checksums.txt"
expected="$(awk -v name="$archive" '$2 == name || $2 == "*" name { print $1; exit }' "$tmp_dir/checksums.txt")"
[ -n "$expected" ] || { echo "Checksum not found for $archive" >&2; exit 1; }
if command -v sha256sum >/dev/null 2>&1; then
  actual="$(sha256sum "$tmp_dir/$archive" | awk '{print $1}')"
else
  actual="$(shasum -a 256 "$tmp_dir/$archive" | awk '{print $1}')"
fi
[ "$expected" = "$actual" ] || { echo "Checksum mismatch for $archive" >&2; exit 1; }

tar -xzf "$tmp_dir/$archive" -C "$tmp_dir" uniforge
chmod 755 "$tmp_dir/uniforge"
mkdir -p "$INSTALL_DIR"
target="$INSTALL_DIR/uniforge"
staged="$INSTALL_DIR/.uniforge-install-$$"
cp "$tmp_dir/uniforge" "$staged"
chmod 755 "$staged"
mv -f "$staged" "$target"

echo "Installed UniForge to $target"
"$target" --version
case ":$PATH:" in
  *":$INSTALL_DIR:"*) ;;
  *) echo "Add $INSTALL_DIR to PATH to run uniforge." ;;
esac
