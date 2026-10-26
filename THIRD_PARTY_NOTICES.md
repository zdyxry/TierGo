# Third-party notices

TierGo's own source code is licensed under the MIT License in `LICENSE`.
That license does not replace the licenses of the components below. The
corresponding license texts are included under `third_party/` and in packaged
applications.

## EasyTier Core and protobuf schemas

- Project: EasyTier, https://github.com/EasyTier/EasyTier
- Version used by this release: v2.6.4
- Commit: `8428a89d2dabc94c97d370ec607c6ca142473626`
- License: GNU Lesser General Public License v3; see `third_party/easytier/LICENSE`.
- Corresponding source: https://github.com/EasyTier/EasyTier/tree/8428a89d2dabc94c97d370ec607c6ca142473626

Packaged applications execute the unmodified official `easytier-core` and
`easytier-cli` binaries. The protobuf files and generated descriptor under
`internal/easytier/schema/` were copied without modification from that commit.
EasyTier's own third-party components and their build inputs remain available
from the corresponding-source repository, including its `Cargo.lock`.

## Go libraries

The application includes these Go modules. Their copyright and license texts
are in the named directories under `third_party/go/`.

| Module | Version | License material |
|---|---|---|
| github.com/egoist/mygo | v0.6.2 | `mygo/LICENSE` (MIT) |
| github.com/makiuchi-d/gozxing | v0.1.1 | `gozxing/LICENSE` (MIT; test/QR support) |
| github.com/pelletier/go-toml/v2 | v2.2.4 | `go-toml/LICENSE` (MIT) |
| github.com/skip2/go-qrcode | 2020-06-17 revision da1b6568686e | `go-qrcode/LICENSE` (MIT) |
| github.com/ebitengine/purego | v0.11.1 | `purego/LICENSE` (Apache-2.0) |
| github.com/go-text/typesetting | v0.3.5 | `typesetting/LICENSE` (Unlicense OR BSD-3-Clause) |
| github.com/go-text/typesetting/harfbuzz | v0.3.5 | `typesetting/harfbuzz-LICENSE` (MIT) |
| golang.org/x/image | v0.46.0 | `x-image/LICENSE` (BSD-3-Clause) |
| golang.org/x/sys | v0.49.0 | `x-sys/LICENSE` (BSD-3-Clause) |
| golang.org/x/text | v0.43.0 | `x-text/LICENSE` (BSD-3-Clause) |
| golang.org/x/xerrors | 2020-08-04 revision 5ec99f83aff1 | `xerrors/LICENSE` (BSD-3-Clause) |
| google.golang.org/protobuf | v1.36.6 | `protobuf/LICENSE` (BSD-3-Clause) |

## Icons

The inline SVG shapes in `internal/app/style.go` are derived from Lucide
(https://github.com/lucide-icons/lucide). Lucide is ISC-licensed and some
shapes are derived from Feather under MIT; see `third_party/lucide/LICENSE`.

## Windows components

- Wintun: the prebuilt binary license permits distribution alongside software
  that uses only its published API. See `third_party/wintun/prebuilt-binaries-license.txt`.
  The upstream `COPYING` is also included. Source: https://github.com/WireGuard/wintun
- WinDivert: dual-licensed under LGPLv3 or GPLv2 at the recipient's choice;
  see `third_party/windivert/LICENSE`. Source: https://github.com/basil00/Divert
- Npcap `Packet.dll`: **not distributed by TierGo**. The Npcap license states
  that redistribution requires special permission. Windows users who need the
  Npcap-backed EasyTier features must install Npcap themselves from
  https://npcap.com. The reference license is preserved at
  `third_party/npcap/LICENSE`; its presence does not grant redistribution rights.
