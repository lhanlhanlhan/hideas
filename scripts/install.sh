#!/usr/bin/env sh
set -eu

REPO="${HIDEAS_REPO:-lhanlhanlhan/hideas}"
VERSION="${HIDEAS_VERSION:-latest}"
INSTALL_DIR="${HIDEAS_INSTALL_DIR:-}"

usage() {
  cat <<'EOF'
Usage: install.sh [--version VERSION] [--install-dir DIR]

Environment:
  HIDEAS_REPO          GitHub repo, default: lhanlhanlhan/hideas
  HIDEAS_VERSION      Release version or "latest", default: latest
  HIDEAS_INSTALL_DIR  Install directory

Examples:
  curl -fsSL https://raw.githubusercontent.com/lhanlhanlhan/hideas/main/scripts/install.sh | sh
  HIDEAS_VERSION=v0.1 sh install.sh
  sh install.sh --install-dir "$HOME/.local/bin"
EOF
}

while [ "$#" -gt 0 ]; do
  case "$1" in
    --version)
      VERSION="$2"
      shift 2
      ;;
    --install-dir)
      INSTALL_DIR="$2"
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "unknown argument: $1" >&2
      usage >&2
      exit 1
      ;;
  esac
done

need_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "required command not found: $1" >&2
    exit 1
  fi
}

need_cmd uname
need_cmd tar
need_cmd mktemp

if command -v curl >/dev/null 2>&1; then
  FETCH="curl -fsSL"
elif command -v wget >/dev/null 2>&1; then
  FETCH="wget -qO-"
else
  echo "required command not found: curl or wget" >&2
  exit 1
fi

os="$(uname -s)"
arch="$(uname -m)"

case "$os" in
  Darwin)
    os_name="darwin"
    ;;
  Linux)
    os_name="linux"
    ;;
  *)
    echo "unsupported OS: $os" >&2
    exit 1
    ;;
esac

case "$arch" in
  arm64|aarch64)
    arch_name="arm64"
    ;;
  x86_64|amd64)
    arch_name="amd64"
    ;;
  *)
    echo "unsupported architecture: $arch" >&2
    exit 1
    ;;
esac

if [ "$os_name" = "darwin" ] && [ "$arch_name" != "arm64" ]; then
  echo "unsupported target: darwin-$arch_name. Release assets currently include darwin-arm64 only." >&2
  exit 1
fi

asset="hideas-${os_name}-${arch_name}.tar.gz"

if [ "$VERSION" = "latest" ]; then
  base_url="https://github.com/${REPO}/releases/latest/download"
else
  base_url="https://github.com/${REPO}/releases/download/${VERSION}"
fi

if [ -z "$INSTALL_DIR" ]; then
  if [ -n "${HOME:-}" ] && [ -d "$HOME/.local/bin" ]; then
    INSTALL_DIR="$HOME/.local/bin"
  elif [ -n "${HOME:-}" ]; then
    INSTALL_DIR="$HOME/.local/bin"
  else
    INSTALL_DIR="/usr/local/bin"
  fi
fi

tmp_dir="$(mktemp -d)"
cleanup() {
  rm -rf "$tmp_dir"
}
trap cleanup EXIT INT TERM

echo "Downloading ${asset} from ${REPO} ${VERSION}..."
$FETCH "${base_url}/${asset}" > "${tmp_dir}/${asset}"

if $FETCH "${base_url}/SHA256SUMS" > "${tmp_dir}/SHA256SUMS" 2>/dev/null; then
  expected="$(grep " ${asset}\$" "${tmp_dir}/SHA256SUMS" | awk '{print $1}')"
  if [ -n "$expected" ]; then
    if command -v sha256sum >/dev/null 2>&1; then
      actual="$(sha256sum "${tmp_dir}/${asset}" | awk '{print $1}')"
    elif command -v shasum >/dev/null 2>&1; then
      actual="$(shasum -a 256 "${tmp_dir}/${asset}" | awk '{print $1}')"
    else
      actual=""
      echo "sha256sum or shasum not found; skipping checksum verification" >&2
    fi
    if [ -n "$actual" ] && [ "$actual" != "$expected" ]; then
      echo "checksum mismatch for ${asset}" >&2
      exit 1
    fi
  fi
fi

tar -xzf "${tmp_dir}/${asset}" -C "$tmp_dir"
chmod +x "${tmp_dir}/hideas"
mkdir -p "$INSTALL_DIR"

target="${INSTALL_DIR}/hideas"
if [ -w "$INSTALL_DIR" ]; then
  mv "${tmp_dir}/hideas" "$target"
else
  need_cmd sudo
  sudo mv "${tmp_dir}/hideas" "$target"
fi

echo "Installed hideas to ${target}"
if ! command -v hideas >/dev/null 2>&1; then
  echo "Note: ${INSTALL_DIR} is not currently on PATH." >&2
fi

