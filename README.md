# Wormhole Modpack Sync 🌌

A modern, high-performance, cross-platform delta synchronizer built with **Go**, **Wails**, **Svelte 5**, and **Tailwind CSS**. Designed specifically for Minecraft modpacks (and generic folder synchronization) across **Linux** and **Windows**.

---

## ⚡ Features

- **Delta Synchronization**: Only transfers new or modified files. Files with matching SHA-256 hashes are automatically skipped, saving gigabytes of bandwidth and minutes of wait time.
- **Atomic Safe Writes**: Every incoming file is streamed into a `.wormhole.tmp` file, hash-verified, and then atomically renamed into place. Connection drops or pauses will never corrupt existing instance files.
- **Two Network Modes**:
  1. **P2P NAT Hole Punch (WebRTC)**: Direct peer-to-peer data channels punched through NAT using STUN servers. Share a simple 6-digit code with your friend.
  2. **Direct Network (Tailscale / LAN)**: Direct high-speed TCP transfer over Tailscale (`100.x.y.z`) or local network, automatically detecting host IP addresses with zero third-party dependencies.
- **Interactive File Tree & Modpack Presets**:
  - Filter and check/uncheck files and directories to sync.
  - Built-in **Modpack Preset** that enables `mods/`, `config/`, `resourcepacks/`, `shaderpacks/`, `kubejs/`, `scripts/` while safeguarding personal player files (`saves/`, `logs/`, `screenshots/`, `options.txt`).
- **Intuitive UI Flow**:
  - **Receive Files**: Pick destination directory &rarr; Enter code &rarr; Form locks into a glowing circular progress ring with centered telemetry &rarr; Completion chime plays &rarr; Close button.
  - **Send Files**: Choose instance &rarr; Customize file tree &rarr; Share code/address &rarr; Start, Pause, Resume, Stop &rarr; Live progress bar &amp; speed indicator (MB/s).
- **Universal Audio Notification**: Zero-dependency Web Audio API harmonic chime triggers when synchronization completes.

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
