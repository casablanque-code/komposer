#!/usr/bin/env bash
set -euo pipefail

REPO="casablanque-code/komposer"
BINARY="komposer"
INSTALL_DIR="/usr/local/bin"

OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)
case "$ARCH" in
  x86_64)  ARCH="amd64" ;;
  aarch64) ARCH="arm64" ;;
  arm64)   ARCH="arm64" ;;
  *)       echo "Unsupported arch: $ARCH" >&2; exit 1 ;;
esac
case "$OS" in
  linux|darwin) ;;
  *)
    echo "This script installs Linux/macOS builds only." >&2
    echo "For Windows, grab komposer_*_windows_*.zip from:" >&2
    echo "  https://github.com/${REPO}/releases/latest" >&2
    exit 1
    ;;
esac

# Resolve release tag. KOMPOSER_VERSION lets a caller pin to a specific
# release instead of always tracking main's idea of "latest" — without
# it, the same `curl install.sh | bash` line can silently install a
# different komposer build tomorrow than it did today.
if [[ -n "${KOMPOSER_VERSION:-}" ]]; then
  TAG="$KOMPOSER_VERSION"
else
  TAG=$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" \
        | grep '"tag_name"' | sed 's/.*"tag_name": "\(.*\)".*/\1/')
fi

if [[ -z "$TAG" ]]; then
  echo "error: could not resolve latest release tag" >&2
  exit 1
fi

# .goreleaser.yaml's name_template uses GoReleaser's {{ .Version }},
# which is the tag with a leading "v" stripped (e.g. tag v0.3.0 ->
# archive komposer_0.3.0_linux_amd64.tar.gz) — not {{ .Tag }}, which
# would keep it. TAG itself (with the "v") is what the download URL
# needs, since release URLs are keyed by the tag as-is.
VERSION="${TAG#v}"
ARCHIVE="${BINARY}_${VERSION}_${OS}_${ARCH}.tar.gz"

BASE_URL="https://github.com/${REPO}/releases/download/${TAG}"
ARCHIVE_URL="${BASE_URL}/${ARCHIVE}"
CHECKSUMS_URL="${BASE_URL}/checksums.txt"

echo "Installing komposer ${TAG} (${OS}/${ARCH})..."

WORKDIR=$(mktemp -d)
trap 'rm -rf "$WORKDIR"' EXIT

curl -fsSL "$ARCHIVE_URL" -o "${WORKDIR}/${ARCHIVE}"
curl -fsSL "$CHECKSUMS_URL" -o "${WORKDIR}/checksums.txt"

echo "Verifying checksum..."
EXPECTED=$(grep " ${ARCHIVE}\$" "${WORKDIR}/checksums.txt" | awk '{print $1}')
if [[ -z "$EXPECTED" ]]; then
  echo "error: ${ARCHIVE} not listed in checksums.txt" >&2
  exit 1
fi
if command -v sha256sum &>/dev/null; then
  ACTUAL=$(sha256sum "${WORKDIR}/${ARCHIVE}" | awk '{print $1}')
elif command -v shasum &>/dev/null; then
  ACTUAL=$(shasum -a 256 "${WORKDIR}/${ARCHIVE}" | awk '{print $1}')
else
  echo "warning: no sha256sum or shasum found — skipping checksum verification" >&2
  ACTUAL="$EXPECTED"
fi

if [[ "$ACTUAL" != "$EXPECTED" ]]; then
  echo "error: checksum mismatch" >&2
  echo "  expected: $EXPECTED" >&2
  echo "  got:      $ACTUAL" >&2
  exit 1
fi
echo "Checksum OK"

tar -xzf "${WORKDIR}/${ARCHIVE}" -C "$WORKDIR" "$BINARY"
chmod +x "${WORKDIR}/${BINARY}"

if [[ -w "$INSTALL_DIR" ]]; then
  mv "${WORKDIR}/${BINARY}" "${INSTALL_DIR}/${BINARY}"
else
  sudo mv "${WORKDIR}/${BINARY}" "${INSTALL_DIR}/${BINARY}"
fi

echo ""
echo "✓ komposer ${TAG} installed to ${INSTALL_DIR}/${BINARY}"
echo ""
echo "Run 'komposer' to get started."
