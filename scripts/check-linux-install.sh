#!/usr/bin/env bash
set -euo pipefail

root="$(git rev-parse --show-toplevel)"
cd "$root"
version="$(jq -r .version mygo.json)"
name="$(jq -r .name mygo.json | tr '[:upper:]' '[:lower:]')"
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
export HOME="$work/home"
export XDG_DATA_HOME="$HOME/.local/share"
mkdir -p "$HOME" "$work/bundle"
cp "build/linux-amd64/$name-$version-linux-amd64.tar.gz" \
    "build/linux-amd64/install.sh" "$work/bundle/"

(
    cd "$work/bundle"
    sh ./install.sh
)

app_dir="$HOME/.local/$name.app"
desktop="$XDG_DATA_HOME/applications/$name.desktop"
test -x "$app_dir/$name"
test -f "$app_dir/$name.png"
test -f "$app_dir/LICENSE"
test -f "$app_dir/SECURITY.md"
test -f "$app_dir/THIRD_PARTY_NOTICES.md"
test -f "$app_dir/third_party/easytier/LICENSE"
test -f "$desktop"
grep -F "Exec=\"$app_dir/$name\"" "$desktop" >/dev/null
grep -F "Icon=$app_dir/$name.png" "$desktop" >/dev/null

sh build/linux-amd64/install.sh --uninstall
test ! -e "$app_dir"
test ! -e "$desktop"
