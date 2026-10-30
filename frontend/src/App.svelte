<script lang="ts">
  import Header from "./components/Header.svelte";
  import ModeSelection from "./components/ModeSelection.svelte";
  import ReceiveFlow from "./components/ReceiveFlow.svelte";
  import SendFlow from "./components/SendFlow.svelte";
  import { GetNetworkInfo, Reset as WailsReset } from "../wailsjs/go/main/App";
  import { EventsOn } from "../wailsjs/runtime/runtime";
  import type { models } from "../wailsjs/go/models";

  type ViewMode = "mode_select" | "receive" | "send";
  let currentView = $state<ViewMode>("mode_select");
  let networkMode = $state<"webrtc" | "direct">("webrtc");
  let networkInfo = $state<models.NetworkInfo | null>(null);

  let currentStats = $state<models.SyncStats>({
    status: "idle",
    role: "",
    mode: "",
    totalFiles: 0,
    completedFiles: 0,
    prunedFiles: 0,
    totalBytes: 0,
    transferredBytes: 0,
    speedBytesPerSec: 0,
    speedFormatted: "0 B/s",
    percent: 0,
    currentFile: "",
    message: "",
    errorMessage: "",
    code: "",
    peerAddress: ""
  });

  $effect(() => {
    // Detect network environment (Tailscale, local LAN)
    GetNetworkInfo()
      .then((info) => {
        networkInfo = info;
      })
      .catch((err) => {
        console.warn("Could not retrieve network info:", err);
      });

    // Listen to real-time sync progress events emitted from Go
    const unsubscribe = EventsOn("sync:stats", (stats: models.SyncStats) => {
      currentStats = stats;
    });

    return () => {
      if (typeof unsubscribe === "function") {
        unsubscribe();
      }
    };
  });

  function handleSelectMode(mode: "receive" | "send") {
    currentView = mode;
  }

  function handleToggleNetworkMode() {
    networkMode = networkMode === "webrtc" ? "direct" : "webrtc";
  }

  async function handleReset() {
    try {
      await WailsReset();
    } catch (e) {
      console.warn("Reset error:", e);
    }
    currentStats = {
      status: "idle",
      role: "",
      mode: "",
      totalFiles: 0,
      completedFiles: 0,
      prunedFiles: 0,
      totalBytes: 0,
      transferredBytes: 0,
      speedBytesPerSec: 0,
      speedFormatted: "0 B/s",
      percent: 0,
      currentFile: "",
      message: "",
      errorMessage: "",
      code: "",
      peerAddress: ""
    };
    currentView = "mode_select";
  }
</script>

<main class="min-h-screen bg-[#0e0f12] text-neutral-100 flex flex-col font-sans selection:bg-amber-500/30 selection:text-white">
  <!-- Top Header Navigation -->
  <Header
    tailscaleIP={networkInfo?.tailscaleIP}
    networkMode={networkMode}
    onSelectNetworkMode={(mode) => (networkMode = mode)}
    canReset={currentView !== "mode_select"}
    onReset={handleReset}
  />

  <!-- Main View Area -->
  <div class="flex-1 w-full">
    {#if currentView === "mode_select"}
      <ModeSelection onSelectMode={handleSelectMode} />
    {:else if currentView === "receive"}
      <ReceiveFlow
        networkMode={networkMode}
        currentStats={currentStats}
        onClose={handleReset}
      />
    {:else if currentView === "send"}
      <SendFlow
        networkMode={networkMode}
        currentStats={currentStats}
        onClose={handleReset}
      />
    {/if}
  </div>
</main>
