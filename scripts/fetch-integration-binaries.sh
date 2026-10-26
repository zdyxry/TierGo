#!/usr/bin/env bash
set -euo pipefail

root="$(git rev-parse --show-toplevel)"
cd "$root"
mkdir -p build/test-bin/v2.6.4 build/test-bin/main

fetch_locked() {
    local version="$1" asset="$2" sha256="$3"
    local archive="build/test-bin/$asset"
    if [[ ! -f "$archive" ]]; then
        local temporary="$archive.tmp"
        trap 'rm -f "$temporary"' RETURN
        curl -fL --retry 3 --output "$temporary" \
            "https://github.com/EasyTier/EasyTier/releases/download/$version/$asset"
        mv "$temporary" "$archive"
        trap - RETURN
    fi
    printf '%s  %s\n' "$sha256" "$archive" | sha256sum -c -
}

stable_version="$(jq -r .version core-lock.json)"
stable_asset="$(jq -r '.platforms["linux/amd64"].asset' core-lock.json)"
stable_sha256="$(jq -r '.platforms["linux/amd64"].sha256' core-lock.json)"
next_version="$(jq -r .testFixtures.nextCore.version core-lock.json)"
next_asset="$(jq -r .testFixtures.nextCore.asset core-lock.json)"
next_sha256="$(jq -r .testFixtures.nextCore.sha256 core-lock.json)"

fetch_locked "$stable_version" "$stable_asset" "$stable_sha256"
fetch_locked "$next_version" "$next_asset" "$next_sha256"

unzip -j -o "build/test-bin/$stable_asset" '*/easytier-web' -d build/test-bin/v2.6.4 >/dev/null
unzip -j -o "build/test-bin/$next_asset" '*/easytier-core' '*/easytier-cli' -d build/test-bin/main >/dev/null
chmod +x build/test-bin/v2.6.4/easytier-web build/test-bin/main/easytier-core build/test-bin/main/easytier-cli

export EASYTIER_WEB="$root/build/test-bin/v2.6.4/easytier-web"
export EASYTIER_NEXT_CORE="$root/build/test-bin/main/easytier-core"
export EASYTIER_NEXT_CLI="$root/build/test-bin/main/easytier-cli"

"$EASYTIER_WEB" --version
"$EASYTIER_NEXT_CORE" --version
