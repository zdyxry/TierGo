# Embedded core

`cmd/fetch-core` writes the official EasyTier binaries here, one platform at a
time, compressed. By default it reads the release version, asset name and
SHA-256 for the selected platform from `core-lock.json`, and always verifies
the downloaded archive:

```sh
go run ./cmd/fetch-core                          # locked host platform
go run ./cmd/fetch-core -platform linux/amd64   # locked release platform
```

An explicit `-version` or `latest` without a matching lock entry requires
`-sha256`. Passing `-lock ""` explicitly disables the lock and likewise
requires both `-version` and `-sha256`.

Core and CLI are mandatory; Windows archives must also contain `wintun.dll` and
`WinDivert64.sys`. Windows `.exe` names are normalized to `easytier-core.gz`
and `easytier-cli.gz`. Extraction and provenance writing happen in a staging
directory under this output directory. Only a complete result replaces the
platform directory, so a failed fetch does not leave a partial update.
`provenance.json` records version, asset and archive SHA-256, without a fetch
timestamp.

Npcap and `Packet.dll` are not redistributed. Windows features that require
Npcap need a separate user-installed copy from <https://npcap.com>; bundling
wintun and WinDivert does not install or replace Npcap.

A build made afterwards embeds these files, so the application is one file. A
build made without them still compiles and looks for `easytier-core` on the
machine instead.

This file only exists so the directory is never empty.
