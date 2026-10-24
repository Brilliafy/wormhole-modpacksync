# Wormhole Modpack Sync

A high-performance, cross-platform file synchronizer built with **Go**, **Wails**, **Svelte 5**, and **Tailwind CSS**. Designed specifically for Minecraft modpacks (and generic folder synchronization) across **Linux** and **Windows**.

---

## Technical Features

- **Differential Transfer**: Only transfers new or modified files. Files with matching SHA-256 hashes are automatically skipped, saving bandwidth and transfer time.
- **Fast Scanning & Instant Codes**: The directory tree is walked in parallel and the share code is generated the moment you press Start, while hashing continues in the background. A peer can connect while hashing is still running and the manifest is delivered as soon as it is ready.
- **Atomic Safe Writes**: Every incoming file is streamed into a `.wormhole.tmp` file, hash-verified, and then atomically renamed into place. Connection drops or pauses will never corrupt existing instance files.
- **Two Network Modes**:
  1. **P2P NAT Hole Punch (WebRTC)**: Direct peer-to-peer data channels over STUN. An auto-generated, time-based session code (e.g. `K7X2-M9QF`) is created for you to share with your peer. By default the SDP handshake goes through a public MQTT rendezvous, so plain codes work between different networks with zero setup. On a Tailnet the code instead carries the host's own embedded relay address (`CODE@100.x.y.z:42425`). Self-host the relay and set `WORMHOLE_RELAY` on both peers to avoid third parties entirely.
  2. **Direct Network (Tailscale / LAN)**: Direct high-speed TCP socket transfer over Tailscale (`100.x.y.z`) or local network, automatically detecting host IP addresses with zero third-party dependencies.
- **Folder Mirroring & Pruning (Absolute Sync)**: Optionally designate specific folders (e.g. `mods/` and `config/`) to strictly mirror 1:1. Any extra or obsolete files on the receiver's side within those specific folders are automatically pruned, while non-mirrored directories and root files (such as individual files like `options.txt`) remain untouched.
- **Hierarchical Directory Tree & Presets**:
  - Filter and toggle files and directories to sync.
  - Per-folder **1:1 Mirror (Prune)** toggle button with visual status indicators.
  - Built-in **Modpack Preset** that automatically enables `mods/`, `config/`, `resourcepacks/`, `shaderpacks/`, `kubejs/`, `scripts/` (with 1:1 mirror on `mods/` and `config/`) while safeguarding personal player files (`saves/`, `logs/`, `screenshots/`, `options.txt`).
- **Tactile UI Console**:
  - **Receive Mode**: Pick destination directory &rarr; Enter session code &rarr; Console locks into a precision circular gauge with centered telemetry &rarr; Completion audio notification &rarr; Close button.
  - **Send Mode**: Choose instance &rarr; Curate directory tree (single-click a checkbox to toggle it, double-click a folder name to expand) &rarr; An auto-generated code appears when you start &rarr; Start, Pause, Resume, Stop &rarr; Live progress bar &amp; speed indicator (MB/s).
- **Universal Audio Notification**: Zero-dependency Web Audio API harmonic chime triggers when synchronization completes.

---

## Network Modes

- **Direct / Tailscale (recommended, works out of the box)**: The sender listens on TCP `42424` (falling back to a random port if it is busy) and advertises its Tailscale or LAN address. The receiver connects straight to it. No server required.
- **WebRTC code (works across networks by default)**: The sender shares a short code (`K7X2-M9QF`). Both peers connect outbound to a public MQTT rendezvous (`broker.hivemq.com`, falling back to `test.mosquitto.org`), exchange SDP, then stream over a peer-to-peer DTLS-encrypted DataChannel. Only signalling touches the broker; file bytes never do. Use `WORMHOLE_MQTT=ssl://your-broker:8883` to pick your own broker.
- **WebRTC code on a LAN/Tailnet only**: If a Tailscale address is detected (or `WORMHOLE_SIGNAL=local` is set), the code becomes `K7X2-M9QF@192.168.1.20:42425` and the sender's built-in relay is used instead, with no third party involved.
- **Self-hosted relay (optional)**: Run the bundled relay on any public host and set `WORMHOLE_RELAY` on both machines:
  ```
  wormhole-sync -relay -port 42425
  ```
  The generated code is then just the short code (no `@address`).
- **Strict NATs**: if hole punching fails on both sides, set `WORMHOLE_TURN_URL` (plus `WORMHOLE_TURN_USERNAME` and `WORMHOLE_TURN_CREDENTIAL`) on both machines to relay WebRTC through a TURN server. The app reports an ICE failure with this hint instead of hanging.

---

## 📂 Project Architecture

```
wormhole-modpacksync/
├── main.go                     # Wails application entrypoint & standalone relay runner
├── app.go                      # Wails IPC bridge and OS dialog handlers
├── pkg/
│   ├── models/models.go        # Shared data structures (Manifest, FileTreeNode, SyncStats)
│   ├── atomicfile/writer.go    # Atomic write engine with SHA-256 verification
│   ├── scanner/                # Fast recursive directory scanner & parallel hasher
│   │   ├── tree.go             # File tree hierarchy with modpack default filters
│   │   ├── hasher.go           # Worker-pool parallel SHA-256 hashing
│   │   └── scanner.go          # Manifest generator
│   ├── delta/planner.go        # Delta planner (compares local vs remote manifests)
│   ├── signaling/              # Room code generation and embedded HTTP relay
│   └── transport/              # Dual networking layer
│       ├── direct.go           # Direct TCP streaming for Tailscale and LAN
│       ├── webrtc.go           # Pion WebRTC DataChannel NAT hole punching
│       └── manager.go          # Session state coordinator & speed tracker
└── frontend/                   # Svelte 5 + Tailwind CSS v4 frontend
    └── src/
        ├── App.svelte          # Main routing & state container
        ├── lib/
        │   ├── sound.ts        # Synthesized audio chime & error notifications
        │   └── format.ts       # Formatters for bytes and transfer speeds (MB/s)
        └── components/
            ├── Header.svelte         # Top navigation & network status badges
            ├── ModeSelection.svelte  # Welcome screen with Receive / Send cards
            ├── ProgressRing.svelte   # SVG circular progress ring with center telemetry
            ├── FileTree.svelte       # Interactive hierarchical file tree with checkboxes
            ├── ReceiveFlow.svelte    # Receiver view with disabled lock state
            └── SendFlow.svelte       # Sender view with controls & progress bar
```

---

## 🚀 Building & Running

### Prerequisites

- **Go**: 1.22+
- **Node.js**: 20+ (with `pnpm`)
- **Wails CLI**: `go install github.com/wailsapp/wails/v2/cmd/wails@latest`
- **Linux Packages**: `webkit2gtk-4.1` (or `webkit2gtk-4.0`) and `gtk3`

### Development Mode

Run the live-reloading development server:

```bash
wails dev -tags webkit2_41
```

### Building Production Binaries

#### Linux
```bash
wails build -tags webkit2_41
```
The executable is generated at:
```
./build/bin/wormhole-sync
```

#### Windows
To compile on Windows:
```cmd
wails build
```
Or cross-compile from Linux using MinGW (`x86_64-w64-mingw32-gcc`):
```bash
wails build -platform windows/amd64
```

### Standalone Signaling Relay Server

The binary includes a built-in lightweight signaling relay server:

```bash
./build/bin/wormhole-sync --relay --port 42425
```

---

## 🧪 Running Tests

Run all unit and integration tests (atomic writing, delta planning, and end-to-end loopback TCP transfer):

```bash
go test -v ./...
```
