<script lang="ts">
  import { Network, RotateCcw, FolderSync, ShieldCheck } from "@lucide/svelte";

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

<header class="w-full bg-[#141519] border-b border-[#262730] px-5 py-3 flex items-center justify-between sticky top-0 z-50">
  <div class="flex items-center gap-3">
    <div class="w-8 h-8 rounded-[3px] bg-[#1e2026] border border-[#31333e] flex items-center justify-center text-amber-500 shadow-sm">
      <FolderSync class="w-4 h-4" />
    </div>
    <div class="text-left">
      <h1 class="text-sm font-semibold tracking-tight text-white">
        Wormhole Sync
      </h1>
      <p class="text-[11px] text-neutral-400">Modpack & folder sync</p>
    </div>
  </div>

  <div class="flex items-center gap-2">
    {#if tailscaleIP}
      <div class="flex items-center gap-1.5 px-2.5 py-1 rounded-[3px] bg-[#102419] border border-[#1e462e] text-emerald-400 text-xs font-mono">
        <span class="w-1.5 h-1.5 rounded-full bg-emerald-400"></span>
        <span class="text-neutral-400 font-sans text-xs">Tailscale:</span> {tailscaleIP}
      </div>
    {/if}

    <button
      type="button"
      onclick={onToggleNetworkMode}
      class="flex items-center gap-1.5 px-2.5 py-1 rounded-[3px] bg-[#1e2026] hover:bg-[#272932] border border-[#2e303b] text-neutral-300 text-xs transition-colors cursor-pointer active:translate-y-[1px]"
      title="Click to toggle connection mode"
    >
      <Network class="w-3.5 h-3.5 text-neutral-400" />
      <span class="text-neutral-400 text-xs">Connection:</span>
      <span class="text-neutral-200 font-medium">{networkMode === "webrtc" ? "WebRTC" : "Direct / Tailscale"}</span>
    </button>

    {#if canReset}
      <button
        type="button"
        onclick={onReset}
        class="flex items-center gap-1.5 px-2.5 py-1 rounded-[3px] bg-[#1e2026] hover:bg-[#272932] border border-[#2e303b] text-neutral-300 text-xs transition-colors cursor-pointer active:translate-y-[1px]"
        title="Return to main menu"
      >
        <RotateCcw class="w-3.5 h-3.5 text-neutral-400" />
        <span>Menu</span>
      </button>
    {/if}
  </div>
</header>
