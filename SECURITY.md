# Security policy

## Reporting a vulnerability

Please report suspected vulnerabilities privately through GitHub Security
Advisories when that feature is available in the project repository. Otherwise,
open an issue without passwords, private keys, tokens, configuration backups or
real network addresses, and ask for a private contact channel. Do not publicly
disclose exploit details until a fix is available.

The project does not currently promise a response-time SLA. Reports should
include the affected TierGo and EasyTier Core versions, platform, reproduction
steps and impact.

## Local data and secrets

- TierGo has no application telemetry, analytics or crash-upload client.
- Network profiles, disabled remote configuration backups and generated Core
  TOML may contain network secrets, SecureMode keys, WireGuard private keys,
  ACL group secrets or Config Server tokens. They are stored locally with
  owner-only file permissions (normally 0600), not encrypted with a system
  keychain.
- Configuration remains on uninstall unless the user deletes the configuration
  directory. Review file-system, backup and administrator access on shared
  machines.
- Activity logs redact registered secrets of at least four characters. Shorter
  values and secrets unknown to the client are not guaranteed to be redacted.
  Review logs before sharing them.
- Copying or downloading WireGuard configuration exposes private keys to the
  destination selected by the user. QR codes are generated locally.

## Network boundaries

EasyTier RPC does not provide authentication. Only connect to an RPC endpoint
through a trusted tunnel such as SSH or an already trusted VPN. Do not expose
it directly to the Internet. Disabling or deleting a remote network changes the
remote host and uses a local full-configuration backup before re-enabling it.

EasyTier Core communicates with the peers, relays, listeners, Config Server and
other endpoints selected by the user. Those parties receive the network traffic
and metadata required by the corresponding EasyTier protocol. TierGo does not
add a separate cloud service.

## Update and supply-chain checks

Release Core archives are pinned by tag and SHA-256 in `core-lock.json`. GUI
releases are built from one Git tag and publish SHA-256 manifests. macOS formal
releases require Developer ID signing and Apple notarization; development or
ad-hoc-signed builds must not be presented as formally notarized releases.

Npcap's `Packet.dll` is not redistributed by this project. Do not add it to a
release without separately confirming redistribution permission.
