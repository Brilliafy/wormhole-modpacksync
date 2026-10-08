<script lang="ts">
  import { Zap, Network, RefreshCw } from "@lucide/svelte";

  interface Props {
    tailscaleIP?: string;
    networkMode: "webrtc" | "direct";
    onToggleNetworkMode?: () => void;
    canReset?: boolean;
    onReset?: () => void;
  }

  let {
    tailscaleIP = "",
    networkMode = "webrtc",
    onToggleNetworkMode,
    canReset = false,
    onReset
  }: Props = $props();
</script>

<header class="w-full bg-slate-900/80 backdrop-blur-md border-b border-slate-800/80 px-6 py-4 flex items-center justify-between sticky top-0 z-50">
  <div class="flex items-center gap-3">
    <div class="relative w-10 h-10 rounded-xl bg-gradient-to-br from-indigo-500 via-purple-600 to-pink-500 flex items-center justify-center shadow-lg shadow-purple-500/20 ring-1 ring-white/20">
      <Zap class="w-5 h-5 text-white animate-pulse" />
      <div class="absolute inset-0 rounded-xl bg-purple-400/20 blur-sm -z-10"></div>
    </div>
    <div class="text-left">
      <div class="flex items-center gap-2">
        <h1 class="text-base font-bold tracking-tight text-white flex items-center gap-2">
          Wormhole Sync
          <span class="text-[10px] uppercase tracking-wider font-semibold px-2 py-0.5 rounded-full bg-purple-500/20 text-purple-300 border border-purple-500/30">
            Delta
          </span>
        </h1>
      </div>
      <p class="text-xs text-slate-400">High-speed atomic P2P synchronizer</p>
    </div>
  </div>

  <div class="flex items-center gap-3">
    {#if tailscaleIP}
      <div class="flex items-center gap-1.5 px-3 py-1.5 rounded-lg bg-emerald-950/60 border border-emerald-500/30 text-emerald-400 text-xs font-mono">
        <span class="w-2 h-2 rounded-full bg-emerald-400 animate-ping"></span>
        <span class="font-semibold">Tailscale:</span> {tailscaleIP}
      </div>
    {/if}

    <button
      type="button"
      onclick={onToggleNetworkMode}
      class="flex items-center gap-1.5 px-3 py-1.5 rounded-lg bg-slate-800 hover:bg-slate-700/80 border border-slate-700 text-slate-300 text-xs transition-colors cursor-pointer"
      title="Click to toggle between WebRTC hole-punching and direct Tailscale/LAN connection"
    >
      <Network class="w-3.5 h-3.5 text-indigo-400" />
      <span>Mode: <strong class="text-white">{networkMode === "webrtc" ? "P2P Hole-Punch" : "Direct / Tailscale"}</strong></span>
    </button>

    {#if canReset}
      <button
        type="button"
        onclick={onReset}
        class="flex items-center gap-1.5 px-3 py-1.5 rounded-lg bg-slate-800 hover:bg-rose-900/40 border border-slate-700 hover:border-rose-500/50 text-slate-300 hover:text-rose-300 text-xs transition-colors cursor-pointer"
        title="Reset and choose another action"
      >
        <RefreshCw class="w-3.5 h-3.5" />
        <span>Home</span>
      </button>
    {/if}
  </div>
</header>
