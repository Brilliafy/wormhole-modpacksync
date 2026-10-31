<div align="center">

# 🌀 Wormhole Modpack Sync

<p align="center">
  <strong>Lightning-fast, zero-configuration peer-to-peer directory and modpack synchronizer for Linux & Windows.</strong>
</p>

[![Release](https://img.shields.io/github/v/release/Brilliafy/wormhole-modpacksync?style=for-the-badge&color=8b5cf6)](https://github.com/Brilliafy/wormhole-modpacksync/releases/latest)
[![Go Version](https://img.shields.io/badge/Go-1.22+-00ADD8?style=for-the-badge&logo=go&logoColor=white)](https://golang.org)
[![Wails](https://img.shields.io/badge/Wails-v2.9.2-DF1A2A?style=for-the-badge&logo=wails&logoColor=white)](https://wails.io)
[![Svelte](https://img.shields.io/badge/Svelte-v5-FF3E00?style=for-the-badge&logo=svelte&logoColor=white)](https://svelte.dev)
[![Tailwind CSS](https://img.shields.io/badge/Tailwind_CSS-v4-38B2AC?style=for-the-badge&logo=tailwind-css&logoColor=white)](https://tailwindcss.com)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg?style=for-the-badge)](https://opensource.org/licenses/MIT)

<br/>

<p align="center">
  <a href="#-key-features">Key Features</a> •
  <a href="#-how-networking--signaling-works">Networking & Signaling</a> •
  <a href="#-network-modes">Network Modes</a> •
  <a href="#-minecraft-modpack-presets">Modpack Presets</a> •
  <a href="#-building--running">Building & Running</a> •
  <a href="#-environment-variables">Configuration</a>
</p>

</div>

---

## 🚀 Key Features

<table>
<tr>
<td width="50%" valign="top">

### ⚡ Differential Delta Sync
- **Cryptographic SHA-256 Hashing**: Parallel directory scanning and hashing skip identical files automatically.
- **Save Gigabytes of Bandwidth**: Sync large 5 GB+ modpack updates in seconds by only transferring newly added or updated `.jar` and `.toml` files.
- **Bandwidth Savings Telemetry**: Visual readout showing exactly how many megabytes were saved during synchronization.

</td>
<td width="50%" valign="top">

### 🌐 Universal Zero-Cost NAT Traversal
- **Direct WebRTC P2P DataChannels**: DTLS-encrypted direct hole-punched transfer via STUN (`stun.l.google.com`, `stun.cloudflare.com`).
- **Encrypted Transit Bridge Fallback**: If strict Symmetric NAT or enterprise firewalls prevent direct hole-punching, transfers automatically fall back to an end-to-end encrypted transit bridge (`transit.magic-wormhole.io:4001`) with directional **AES-256-GCM** encryption.
- **Multi-STUN Characterization**: Evaluates Endpoint-Independent (Cone) vs Endpoint-Dependent (Symmetric) mapping and measures port stride ($\Delta$).
- **Parallel Gateway Probing**: Automatically requests port leases via **RFC 6886 (NAT-PMP)**, **RFC 6887 (PCP)**, and **UPnP IGD**.

</td>
</tr>
<tr>
<td width="50%" valign="top">

### 🛡️ Atomic Streaming & Pruning
- **Zero In-Place Corruption**: Incoming files stream into `.wormhole.tmp` containers, verify SHA-256 checksums, and atomically rename into place. Connection drops or pauses will never break existing files.
- **Folder-Specific Absolute Sync (Pruning)**: Optionally designate specific folders (such as `mods/` and `config/`) to strictly mirror 1:1. Obsolete or deleted mods on the sender side are automatically pruned on the receiver side, while player data (`saves/`, `options.txt`) remains untouched.

</td>
<td width="50%" valign="top">

### 🎮 Tailored Modpack Experience
- **Minecraft Instance Auto-Detection**: Recognizes folders containing `mods/`, `config/`, or `scripts/`.
- **1-Click Presets**:
  - **Full Modpack (Recommended)**: Pre-selects game content and activates 1:1 pruning on `mods/` and `config/` while safeguarding `saves/`, `logs/`, `screenshots/`, and `options.txt`.
  - **Configs Only**: Blazing-fast sync for balancing updates without re-transferring mod binaries.
- **Smart Search & Filter**: Interactive file tree with auto-expanding directory paths on search match.

</td>
</tr>
<tr>
<td width="50%" valign="top">

### 📱 Streamlined 3-Step UI
- **Instant Room Codes**: 8-character codes (`XXXX-XXXX`) generated immediately while background hashing proceeds in parallel.
- **Smart Code Input**: Automatic hyphenation formatting, automatic uppercase conversion, 1-click clipboard paste, and live clipboard auto-detection chip.
- **Recent Folders History**: Stores the last 3 selected directory paths in `localStorage` for fast re-selection.

</td>
<td width="50%" valign="top">

### 📊 Precision Telemetry & Polish
- **Live Transfer Metrics**: Current speed (`MB/s`), rolling ETA calculation, color-coded transport badges (`🔒 Direct P2P`, `🛡️ Encrypted Relay`, `⚡ Local Network`), and clean file path truncation.
- **Celebratory Completion Card**: Confetti celebration, synthesized zero-dependency Web Audio harmonic completion chime, and native 1-click **Open Folder in File Manager** shortcut.

</td>
</tr>
</table>

---

## 📡 How Networking & Signaling Works

Wormhole Modpack Sync is engineered to establish connections across any network topology—including residential routers, CGNAT, enterprise firewalls, and mobile hotspots—without requiring paid infrastructure or manual port forwarding.

```
+-----------------------------------------------------------------------------------+
|                               SIGNALING PHASE                                     |
|                                                                                   |
|  [Peer A: Sender]                                              [Peer B: Receiver] |
|         |                                                             |           |
|         |--- 1. Connect TLS (Port 8883) --------------------------->  |           |
|         |    Publish SDP Offer to MQTT:                               |           |
|         |    'wormhole-sync/v1/<code-hash>/offer'                     |           |
|         |                               <-- 2. Subscribe & Fetch Offer|           |
|         |                                                             |           |
|         |  <-- 3. Publish SDP Answer to MQTT: ------------------------|           |
|         |         'wormhole-sync/v1/<code-hash>/answer'               |           |
+-----------------------------------------------------------------------------------+
                                          |
                                          v
+-----------------------------------------------------------------------------------+
|                              DATA TRANSFER PHASE                                  |
|                                                                                   |
|  [Primary: Direct WebRTC]                                                         |
|  Both peers attempt simultaneous UDP hole punching via STUN candidates.          |
|                                                                                   |
|  +----------------+      Direct DTLS DataChannel (0 Relays)     +---------------+ |
|  | Peer A: Sender | <========================================>  | Peer B: Recv  | |
|  +----------------+                                             +---------------+ |
|                                                                                   |
|  [Fallback: Encrypted Transit Bridge] (Triggered if Symmetric NAT / CGNAT blocks) |
|                                                                                   |
|  +----------------+   TCP / AES-256-GCM   +---------------+   TCP / AES-256-GCM   |
|  | Peer A: Sender | --------------------> | Transit Relay | --------------------> |
|  +----------------+  (Port 4001 Outbound) | (Magic W-Hole)|  (Port 4001 Outbound) |
|                                           +---------------+                       |
+-----------------------------------------------------------------------------------+
```

### 1. The Signaling Rendezvous (Public MQTT)
To connect two peers across the internet with an 8-character code (like `K7X2-M9QF`), a signaling channel is required to exchange SDP (Session Description Protocol) offers and network candidates:
- By default, both peers connect over secure TLS (`ssl://broker.hivemq.com:8883` or `ssl://broker.emqx.io:8883`).
- The sender hashes the room code into a secure topic: `wormhole-sync/v1/<code-hash>/offer`.
- The receiver fetches the offer, publishes its answer to `wormhole-sync/v1/<code-hash>/answer`, and disconnects once connected.
- **Privacy & Security Guarantee**: The MQTT broker **only** carries small JSON signaling text (~2 KB of base64 SDP candidates). **Zero file contents or file names ever touch the MQTT broker.**

> [!NOTE]
> **Can you bypass public MQTT brokers?**
> Yes! You can point both peers to your own private MQTT broker via `WORMHOLE_MQTT`, run the built-in HTTP relay (`wormhole-sync --relay`), set `WORMHOLE_RELAY`, or transfer directly over LAN / Tailscale IP (`Direct Network` mode).

### 2. Multi-Tier NAT Traversal Engine
When the signaling handshake completes, the transport manager executes:
1. **Parallel Gateway Discovery**: Queries local gateways via **NAT-PMP (RFC 6886)**, **PCP (RFC 6887)**, and **UPnP IGD** to request temporary external port mappings.
2. **Multi-STUN Characterization**: Queries `stun.l.google.com` and `stun.cloudflare.com` from a single socket to detect whether the network uses **Endpoint-Independent (Cone)** or **Endpoint-Dependent (Symmetric)** NAT, and measures linear port stride ($\Delta$).
3. **Birthday Paradox UDP Probing**: On asymmetric topologies (e.g. Symmetric CGNAT to Cone NAT), it deploys a pool of $N=256$ sockets and sprays $M=512$ non-sequential UDP probes paced at 60 packets/sec to punch a hole through restrictive firewalls.
4. **Seamless Transit Fallback**: If direct traversal is mathematically blocked (such as double Symmetric CGNAT on both ends), the transport seamlessly falls back to the **Magic Wormhole Encrypted Transit Bridge** (`transit.magic-wormhole.io:4001`) with directional **AES-256-GCM** encryption and 64-bit sequence counters.

---

## 🌐 Network Modes

| Mode | Mechanism | Ideal Use Case | Zero Config? |
|---|---|---|:---:|
| **Global Wormhole** *(Default)* | Exchanges SDP via public MQTT (`hivemq` / `emqx`), executes direct STUN hole-punching, and automatically falls back to the AES-256-GCM Transit Bridge if symmetric NAT blocks direct traversal. | Anywhere in the world across different ISPs, cellular data, or strict symmetric networks. Simple 8-char code (`XXXX-XXXX`). | ✅ **Yes** |
| **Direct IP / LAN / Tailscale** | Direct framed TCP streaming over port `42424` using local LAN (`192.168.x.x`) or Tailscale VPN (`100.x.y.z`) IP addresses with zero third-party brokers. | Computers on the same home Wi-Fi/Ethernet switch, or peers connected via Tailscale. | ✅ **Yes** |
| **Self-Hosted Signaling Relay** | Built-in HTTP signaling server: `wormhole-sync --relay --port 42425`. Set `WORMHOLE_RELAY` on both machines to route signaling through your private VPS. | Private enterprises, homelabs, or closed communities. | ⚙️ Config Required |

---

## 🎯 Minecraft Modpack Presets

Wormhole Modpack Sync includes built-in logic tailored for Minecraft instances (CurseForge, Prism Launcher, Modrinth, ATLauncher):

```
Minecraft Instance/
├── mods/                 <-- [1:1 Mirror / Prune Enabled] Extra/old mods on receiver are deleted
├── config/               <-- [1:1 Mirror / Prune Enabled] Replaces configs with sender's exact files
├── scripts/              <-- Included in Full Modpack
├── defaultconfigs/       <-- Included in Full Modpack
├── shaderpacks/          <-- Included in Full Modpack
├── resourcepacks/        <-- Included in Full Modpack
├── saves/                <-- [PROTECTED] Singleplayer worlds are NEVER overwritten or pruned
├── logs/                 <-- [PROTECTED] Excluded from sync
├── screenshots/          <-- [PROTECTED] Excluded from sync
└── options.txt           <-- [PROTECTED] Personal player keybinds and video settings are preserved
```

### Preset Breakdown

- **📦 Full Modpack (Recommended)**: Synchronizes all mods, configs, scripts, shaderpacks, and resourcepacks. Automatically designates `mods/` and `config/` for **1:1 Mirroring (Absolute Sync)**, ensuring that outdated mod `.jar` files on the receiver's machine are cleanly deleted.
- **⚙️ Configs Only**: Selects only `config/`, `defaultconfigs/`, and `scripts/`. Delivers instant game balance and recipe fixes across all players in under 3 seconds without re-verifying gigabytes of mod archives.
- **📋 All / None**: 1-click bulk toggles for customized selection of arbitrary game or application directories.

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
│   ├── atomicfile/writer.go    # Atomic temporary streaming (.wormhole.tmp) with SHA-256 verification
│   ├── scanner/                # Fast parallel directory scanner and SHA-256 hasher
│   ├── delta/planner.go        # Manifest differential planner and prune analyzer
│   ├── signaling/              # Room code generation and robust MQTT rendezvous
│   │   ├── mqtt.go             # Public MQTT rendezvous (HiveMQ & EMQX) with TLS
│   │   ├── client.go           # HTTP signaling client & environment URL resolution
│   │   └── relay.go            # Embedded in-memory HTTP signaling relay
│   ├── nat/                    # Universal NAT traversal engine
│   │   ├── stun.go             # Multi-STUN characterization & port stride detection
│   │   ├── gateway.go          # RFC 6886 NAT-PMP, RFC 6887 PCP, and UPnP IGD probers
│   │   ├── ipv6.go             # Global unicast IPv6 discovery & routability checks
│   │   └── puncher.go          # Birthday Paradox socket pool & randomized stride sweeper
│   └── transport/              # Transport orchestration & encrypted transit bridge
│       ├── direct.go           # High-speed framed TCP streaming protocol
│       ├── webrtc.go           # Pion WebRTC DataChannel hole-punching engine
│       ├── transit.go          # Magic Wormhole transit client with AES-256-GCM
│       └── manager.go          # Session state coordinator & automatic fallback
└── frontend/                   # Svelte 5 + Tailwind CSS v4 frontend
    └── src/
        ├── App.svelte          # Main view container & settings dialog
        ├── lib/
        │   ├── storage.ts      # Recent folders persistence (localStorage)
        │   ├── sound.ts        # Zero-dependency Web Audio harmonic completion chime
        │   └── format.ts       # Human-readable byte and speed formatters
        └── components/
            ├── Header.svelte           # Top navigation, status pills & settings modal
            ├── ModeSelection.svelte    # Initial Send / Receive mode selection
            ├── SendFlow.svelte         # 3-Step sender flow & hero code card
            ├── ReceiveFlow.svelte      # 3-Step receiver flow & smart code input
            ├── FileTree.svelte         # Hierarchical file tree, presets & search auto-expand
            ├── TransferProgress.svelte # Unified live progress telemetry card
            └── CompletionCard.svelte   # Celebratory completion card & open folder action
```

---

## 🛠️ Building & Running

### Prerequisites

| Tool | Version | Description |
|---|---|---|
| **Go** | `1.22+` | Backend runtime & compiler |
| **Node.js** | `20+` | Frontend toolchain (supports `pnpm` or `npm`) |
| **Wails CLI** | `v2.9.2+` | Desktop bridge (`go install github.com/wailsapp/wails/v2/cmd/wails@latest`) |
| **Linux Deps** | - | `webkit2gtk-4.1` (or `webkit2gtk-4.0`) and `gtk3` development headers |

### Development Mode

Run the live-reloading dev environment:

```bash
wails dev -tags webkit2_41
```

### Production Builds

#### Linux (AMD64)
```bash
wails build -tags webkit2_41
```
The executable is generated at `./build/bin/wormhole-sync`.

#### Windows (AMD64)
Cross-compile from Linux (requires `mingw-w64`):
```bash
wails build -platform windows/amd64
```
Or directly in Windows PowerShell / Command Prompt:
```cmd
wails build
```
The executable is generated at `./build/bin/wormhole-sync.exe`.

### Standalone Signaling Relay Server

The compiled binary includes a built-in lightweight HTTP signaling relay server for private deployments:

```bash
./build/bin/wormhole-sync --relay --port 42425
```

---

## ⚙️ Environment Variables

Customize signaling and transit endpoints without changing any code:

| Variable | Default | Description |
|---|---|---|
| `WORMHOLE_MQTT` | `ssl://broker.hivemq.com:8883, ssl://broker.emqx.io:8883` | Comma-separated list of MQTT broker URLs for signaling. |
| `WORMHOLE_TRANSIT_RELAY` | `transit.magic-wormhole.io:4001` | TCP address for the fallback Magic Wormhole transit bridge. |
| `WORMHOLE_RELAY` | *(none / localhost)* | Comma-separated list of HTTP signaling relays (e.g. `http://relay.example.com:42425`). |
| `WORMHOLE_SIGNAL` | *(none)* | Set to `local` to restrict signaling to local LAN/Tailscale embedded relay. |
| `WORMHOLE_TURN_URL` | *(none)* | Custom TURN server URL (e.g. `turn:turn.example.com:3478`). |
| `WORMHOLE_TURN_USERNAME`| *(none)* | Username for authenticated TURN servers. |
| `WORMHOLE_TURN_CREDENTIAL`| *(none)*| Credential / password for authenticated TURN servers. |

---

## 💻 Headless CLI & Testing

The repository provides a headless test client at `cmd/e2etest/main.go` for scripting, CI pipelines, and automated synchronization:

```bash
# Sender on machine A:
go run cmd/e2etest/main.go -role sender -mode webrtc -dir /path/to/modpack

# Receiver on machine B:
go run cmd/e2etest/main.go -role receiver -mode webrtc -dir /path/to/destination -code <CODE>
```

Run backend unit and integration tests:

```bash
go test -v ./pkg/...
```

Run frontend type checking and production build:

```bash
cd frontend && npm run check && npm run build
```

---

## 📄 License

Distributed under the **MIT License**. Free, open-source, and privacy-respecting.
