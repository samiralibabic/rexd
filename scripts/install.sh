#!/usr/bin/env bash
set -euo pipefail

REPO="${REXD_REPO:-samiralibabic/rexd}"
VERSION="${REXD_VERSION:-}"
INSTALL_DIR="${REXD_INSTALL_DIR:-/usr/local/bin}"

need_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "Missing required command: $1" >&2
    exit 1
  fi
}

need_cmd curl
need_cmd tar

os_name() {
  case "$(uname -s)" in
    Linux) echo "linux" ;;
    Darwin) echo "darwin" ;;
    *)
      echo "Unsupported OS: $(uname -s)" >&2
      exit 1
      ;;
  esac
}

arch_name() {
  case "$(uname -m)" in
    x86_64|amd64) echo "amd64" ;;
    arm64|aarch64) echo "arm64" ;;
    *)
      echo "Unsupported architecture: $(uname -m)" >&2
      exit 1
      ;;
  esac
}

hash_file() {
  local target="$1"
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$target" | awk '{print $1}'
    return
  fi
  if command -v shasum >/dev/null 2>&1; then
    shasum -a 256 "$target" | awk '{print $1}'
    return
  fi
  echo "Missing checksum tool: sha256sum or shasum" >&2
  exit 1
}

if [[ -z "${VERSION}" ]]; then
  VERSION="$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" | sed -n 's/.*"tag_name":[[:space:]]*"\([^"]*\)".*/\1/p' | head -n1)"
  if [[ -z "${VERSION}" ]]; then
    echo "Failed to resolve latest version from GitHub releases." >&2
    exit 1
  fi
fi

OS="$(os_name)"
ARCH="$(arch_name)"
ASSET="rexd-${OS}-${ARCH}.tar.gz"
BASE_URL="https://github.com/${REPO}/releases/download/${VERSION}"

TMP_DIR="$(mktemp -d)"
trap 'rm -rf "${TMP_DIR}"' EXIT

echo "Installing rexd ${VERSION} (${OS}/${ARCH}) ..."
curl -fsSL "${BASE_URL}/${ASSET}" -o "${TMP_DIR}/${ASSET}"
curl -fsSL "${BASE_URL}/checksums.txt" -o "${TMP_DIR}/checksums.txt"

EXPECTED_SHA="$(awk "/  ${ASSET}\$/{print \$1}" "${TMP_DIR}/checksums.txt")"
if [[ -z "${EXPECTED_SHA}" ]]; then
  echo "Checksum entry for ${ASSET} not found." >&2
  exit 1
fi

ACTUAL_SHA="$(hash_file "${TMP_DIR}/${ASSET}")"
if [[ "${EXPECTED_SHA}" != "${ACTUAL_SHA}" ]]; then
  echo "Checksum mismatch for ${ASSET}." >&2
  exit 1
fi

tar -xzf "${TMP_DIR}/${ASSET}" -C "${TMP_DIR}"
EXTRACTED_BIN="${TMP_DIR}/rexd-${OS}-${ARCH}"
if [[ ! -f "${EXTRACTED_BIN}" ]]; then
  echo "Expected binary ${EXTRACTED_BIN} not found after extraction." >&2
  exit 1
fi

if [[ -w "${INSTALL_DIR}" ]]; then
  install -m 0755 "${EXTRACTED_BIN}" "${INSTALL_DIR}/rexd"
else
  sudo install -m 0755 "${EXTRACTED_BIN}" "${INSTALL_DIR}/rexd"
fi

echo "Installed: ${INSTALL_DIR}/rexd"
echo "Run: rexd --help"
