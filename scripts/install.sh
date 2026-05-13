#!/bin/sh
set -eu

repo="https://github.com/jwa91/prehandover"
version="${PREHANDOVER_VERSION:-latest}"
install_dir="${PREHANDOVER_INSTALL_DIR:-$HOME/.local/bin}"

os="$(uname -s)"
case "$os" in
Darwin|Linux) ;;
*)
	echo "unsupported OS: $os" >&2
	exit 1
	;;
esac

machine="$(uname -m)"
case "$machine" in
x86_64|amd64) arch="x86_64" ;;
arm64|aarch64) arch="arm64" ;;
*)
	echo "unsupported architecture: $machine" >&2
	exit 1
	;;
esac

if [ "$version" = "latest" ]; then
	latest_url="$(curl -fsSIL -o /dev/null -w '%{url_effective}' "$repo/releases/latest")"
	tag="${latest_url##*/}"
else
	case "$version" in
	v*) tag="$version" ;;
	*) tag="v$version" ;;
	esac
fi

artifact="prehandover_${os}_${arch}.tar.gz"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT INT TERM

base="$repo/releases/download/$tag"
curl -fsSL "$base/$artifact" -o "$tmp/$artifact"
curl -fsSL "$base/checksums.txt" -o "$tmp/checksums.txt"

(
	cd "$tmp"
	grep "  $artifact$" checksums.txt > "$artifact.sha256"
	shasum -a 256 -c "$artifact.sha256"
	tar -xzf "$artifact"
)

mkdir -p "$install_dir"
cp "$tmp/prehandover" "$install_dir/prehandover"
chmod +x "$install_dir/prehandover"

echo "installed prehandover ${tag#v} to $install_dir/prehandover"

case ":$PATH:" in
*":$install_dir:"*) ;;
*)
	shell_name="$(basename "${SHELL:-sh}")"
	case "$shell_name" in
	zsh) echo "add it to PATH: echo 'export PATH=\"$install_dir:\$PATH\"' >> ~/.zshrc" ;;
	bash) echo "add it to PATH: echo 'export PATH=\"$install_dir:\$PATH\"' >> ~/.bashrc" ;;
	fish) echo "add it to PATH: fish_add_path $install_dir" ;;
	*) echo "add $install_dir to PATH" ;;
	esac
	;;
esac
