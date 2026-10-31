# Wormhole Modpack Sync

A high-performance, cross-platform peer-to-peer file synchronizer built with **Go**, **Wails v2**, **Svelte 5**, and **Tailwind CSS**. Designed specifically for Minecraft modpacks (and generic directory synchronization) across **Linux** and **Windows** with zero configuration.

---

## 🚀 Key Features

- **Differential & Delta Transfer**: Computes cryptographic SHA-256 hashes in parallel. Unchanged files are automatically skipped, saving gigabytes of bandwidth and cutting sync times to seconds.
- **Universal Zero-Cost NAT Traversal**:
  - **Direct P2P WebRTC**: High-speed, DTLS-encrypted direct DataChannel transfer over STUN.
  - **Encrypted Transit Bridge Fallback**: If strict Symmetric NAT or enterprise firewalls prevent direct hole-punching, transfers automatically and seamlessly fall back to an end-to-end encrypted transit bridge (`transit.magic-wormhole.io:4001`) with directional AES-256-GCM authenticated encryption and 64-bit sequence counters. Zero plaintext ever reaches intermediate relays.
  - **Multi-STUN Characterization**: Evaluates Endpoint-Independent (Cone) vs Endpoint-Dependent (Symmetric) mapping and measures linear port stride ($\Delta$).
  - **Parallel Gateway Probing**: Discovers and requests port leases via RFC 6886 (NAT-PMP), RFC 6887 (PCP), and UPnP IGD.
  - **Birthday Paradox Probe Engine**: High-probability hole punching ($N=256$, $M=512$ randomized probes) designed to traverse asymmetric NAT boundaries without tripping network intrusion alarms.
- **Fast Scanning & Non-Blocking Handshakes**: The directory tree is walked in parallel and the 8-character share code is generated instantly while hashing runs in the background. Peers can connect immediately; manifests are delivered the moment hashing completes.
- **Atomic Safe Writes**: Every incoming file is streamed into a `.wormhole.tmp` file, hash-verified, and atomically renamed into place. Pauses, interruptions, or dropped connections will never corrupt existing game files.
- **Folder Mirroring & Pruning (Absolute Sync)**: Designate folders (such as `mods/` and `config/`) to strictly mirror 1:1. Outdated or removed mods on the sender side are automatically pruned on the receiver side, while player data (such as singleplayer `saves/`) remains untouched.
- **Smart Minecraft Modpack Auto-Presets**:
  - Automatically identifies Minecraft instances containing `mods/`, `config/`, or `scripts/`.
  - 1-Click presets:
    - **Full Modpack (Recommended)**: Auto-selects `mods/`, `config/`, `scripts/`, `defaultconfigs/`, `shaderpacks/`, `resourcepacks/`, and enables 1:1 mirroring on `mods/` and `config/` while safeguarding `saves/`, `logs/`, `screenshots/`, and `options.txt`.
    - **Configs Only**: High-speed sync for balancing and configuration updates without re-transferring large mod jars.
    - **All** / **None** for custom selections.
- **Streamlined UI & Telemetry**:
  - **Structured 3-Phase Stepper**: Clear breadcrumb progression (`Setup` &rarr; `Connect` &rarr; `Sync & Complete`).
  - **Frictionless Code Entry**: Auto-hyphenation (`1EB4-WQMJ`), auto-capitalization, 1-click paste button, and system clipboard auto-detect chip.
  - **Unified Telemetry**: Live transfer speed (`MB/s`), rolling ETA estimate, color-coded transport badge (`🔒 Direct P2P` vs `🛡️ Encrypted Relay` vs `⚡ Local Network`), and clean file path truncation.
  - **Actionable Completion**: Confetti celebration, harmonic audio chime, delta bandwidth savings metrics, and a 1-click **Open Folder in File Manager** shortcut.
  - **Recent Folders History**: Stores the last 3 used paths in `localStorage` for 1-click re-selection.

---

## 🌐 Network Modes

| Mode | How It Works | Ideal For |
|---|---|---|
| **Global Wormhole (Default)** | Exchanges SDP via public MQTT rendezvous (`broker.emqx.io` / `broker.hivemq.com`), attempts direct STUN P2P hole-punching, and automatically falls back to the AES-256-GCM Encrypted Transit Bridge if symmetric NAT blocks direct traversal. | Works **everywhere** worldwide with a simple 8-character code (`XXXX-XXXX`) with zero setup or router configuration. |
| **Direct IP / LAN / Tailscale** | Direct TCP connection over port `42424` using local LAN (`192.168.x.x`) or Tailscale VPN (`100.x.y.z`) IP addresses with zero third-party involvement. | Devices on the same Wi-Fi router, wired Ethernet, or connected via a shared Tailscale tailnet. |
| **Self-Hosted Relay** | Run the bundled signaling relay on a public VPS: `wormhole-sync --relay --port 42425`. Set `WORMHOLE_RELAY` on both machines to use your own infrastructure. | Private enterprise deployments or private group servers. |

---

## 📂 Project Architecture

```
wormhole-modpacksync/
├── main.go                     # Wails application entrypoint & CLI flags
├── app.go                      # Wails IPC bridge & OS file manager integration
├── cmd/
│   └── e2etest/main.go         # Headless CLI end-to-end sync testing tool
├── pkg/
│   ├── models/models.go        # Manifest, FileTreeNode, SyncStats, DeltaPlan
│   ├── atomicfile/writer.go    # Atomic temporary streaming with SHA-256 verification
│   ├── scanner/                # Fast parallel directory scanner and SHA-256 hasher
│   ├── delta/planner.go        # Manifest differential planner and prune analyzer
│   ├── signaling/              # Room code generation and robust MQTT rendezvous
│   ├── nat/                    # Traversal engine
│   │   ├── stun.go             # Multi-STUN characterization & port stride detection
│   │   ├── gateway.go          # RFC 6886 NAT-PMP, RFC 6887 PCP, and UPnP probers
│   │   ├── ipv6.go             # Global unicast IPv6 discovery & routability check
│   │   └── puncher.go          # Birthday Paradox socket pool & stride sweeper
│   └── transport/              # Dual transport & encrypted transit bridge
│       ├── direct.go           # High-speed framed TCP streaming protocol
│       ├── webrtc.go           # Pion WebRTC DataChannel hole-punching engine
│       ├── transit.go          # Magic Wormhole relay client with AES-256-GCM
│       └── manager.go          # Session state coordinator & automatic fallback
└── frontend/                   # Svelte 5 + Tailwind CSS v4 frontend
    └── src/
        ├── App.svelte          # Main view container & settings dialog
        ├── lib/
        │   ├── storage.ts      # Recent folders persistence
        │   ├── sound.ts        # Zero-dependency Web Audio harmonic completion chime
        │   └── format.ts       # Human-readable byte and speed formatters
        └── components/
            ├── Header.svelte           # Top navigation, status pills & settings modal
            ├── ModeSelection.svelte    # Initial Send / Receive selection
            ├── SendFlow.svelte         # 3-Step sender flow & hero code card
            ├── ReceiveFlow.svelte      # 3-Step receiver flow & smart code input
            ├── FileTree.svelte         # Hierarchical file tree, presets & search auto-expand
            ├── TransferProgress.svelte # Unified live progress telemetry card
            └── CompletionCard.svelte   # Celebratory completion card & open folder action
```

---

## 🛠️ Building & Running

### Prerequisites

- **Go**: 1.22+
- **Node.js**: 20+ (with `pnpm` or `npm`)
- **Wails CLI**: `go install github.com/wailsapp/wails/v2/cmd/wails@latest`
- **Linux Dependencies**: `webkit2gtk-4.1` (or `webkit2gtk-4.0`) and `gtk3`

### Development Mode

Run the live-reloading dev server:

```bash
wails dev -tags webkit2_41
```

### Production Builds

#### Linux (AMD64)
```bash
wails build -tags webkit2_41
```
The binary is generated at `./build/bin/wormhole-sync`.

#### Windows (AMD64)
Cross-compile from Linux (requires `x86_64-w64-mingw32-gcc`):
```bash
wails build -platform windows/amd64
```
Or directly on Windows:
```cmd
wails build
```
The executable is generated at `./build/bin/wormhole-sync.exe`.

### Standalone Signaling Relay Server

The binary includes a built-in lightweight signaling relay server:

```bash
./build/bin/wormhole-sync --relay --port 42425
```

---

## 🧪 Running Tests

Run all unit tests across all packages:

```bash
go test -v ./pkg/...
```

Run frontend type checking and production build:

```bash
cd frontend && npm run check && npm run build
```

---

## 📄 License

MIT License. Free and open source.
