#!/bin/sh
set -eu

repo="honch-io/sandbox"
install=1
install_dir="${HOME}/.local/bin"
sandbox_dir="${HONCH_SANDBOX_ROOT:-${HOME}/.local/share/honch/sandbox}"

usage() {
  cat <<'USAGE'
Usage: install.sh [--no-install] [--install-dir DIR] [--sandbox-dir DIR]

Downloads the latest Honch CLI release, prepares a sandbox checkout, and starts `honch onboarding`.
Default install location: ~/.local/bin/honch.
Default sandbox checkout: ~/.local/share/honch/sandbox.
USAGE
}

while [ "$#" -gt 0 ]; do
  case "$1" in
    --no-install)
      install=0
      shift
      ;;
    --install-dir)
      if [ "$#" -lt 2 ]; then
        echo "missing value for --install-dir" >&2
        exit 2
      fi
      install_dir="$2"
      shift 2
      ;;
    --sandbox-dir)
      if [ "$#" -lt 2 ]; then
        echo "missing value for --sandbox-dir" >&2
        exit 2
      fi
      sandbox_dir="$2"
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "unknown option: $1" >&2
      usage >&2
      exit 2
      ;;
  esac
done

os_name="$(uname -s | tr '[:upper:]' '[:lower:]')"
arch_name="$(uname -m)"

case "$os_name" in
  darwin|linux) ;;
  *)
    echo "unsupported OS: $os_name" >&2
    exit 1
    ;;
esac

case "$arch_name" in
  x86_64|amd64) arch_name="amd64" ;;
  arm64|aarch64) arch_name="arm64" ;;
  *)
    echo "unsupported architecture: $arch_name" >&2
    exit 1
    ;;
esac

tmp_dir="$(mktemp -d)"
cleanup() {
  rm -rf "$tmp_dir"
}
trap cleanup EXIT INT TERM

url="https://github.com/${repo}/releases/latest/download/honch-${os_name}-${arch_name}"
tmp_honch="${tmp_dir}/honch"

echo "Downloading Honch CLI from ${url}"
curl -fL "$url" -o "$tmp_honch"
chmod +x "$tmp_honch"

honch_cmd="$tmp_honch"
if [ "$install" -eq 1 ]; then
  target="${install_dir}/honch"
  printf "Install honch to %s? [y/N] " "$target"
  read answer || answer=""
  case "$answer" in
    y|Y|yes|YES)
      mkdir -p "$install_dir"
      cp "$tmp_honch" "$target"
      chmod +x "$target"
      honch_cmd="$target"
      echo "Installed honch to ${target}"
      ;;
    *)
      echo "Running honch from temporary download"
      ;;
  esac
fi

is_sandbox_checkout() {
  test -f "$1/go.mod" && test -d "$1/adapters" && test -d "$1/harnesses"
}

if is_sandbox_checkout "$PWD"; then
  sandbox_dir="$PWD"
elif is_sandbox_checkout "$sandbox_dir"; then
  :
elif [ -e "$sandbox_dir" ]; then
  echo "sandbox checkout path exists but is not a sandbox repo: $sandbox_dir" >&2
  exit 1
else
  if ! command -v git >/dev/null 2>&1; then
    echo "git is required to prepare the sandbox checkout" >&2
    exit 1
  fi
  mkdir -p "$(dirname "$sandbox_dir")"
  git clone "https://github.com/${repo}.git" "$sandbox_dir"
fi

cd "$sandbox_dir"
"$honch_cmd" onboarding
