<script lang="ts">
  import { Network, RotateCcw, FolderSync, ShieldCheck, Settings2, X, Check, Globe, Cable } from "@lucide/svelte";

  interface Props {
    tailscaleIP?: string;
    networkMode: "webrtc" | "direct";
    onSelectNetworkMode?: (mode: "webrtc" | "direct") => void;
    canReset?: boolean;
    onReset?: () => void;
  }

  let {
    tailscaleIP = "",
    networkMode = "webrtc",
    onSelectNetworkMode,
    canReset = false,
    onReset
  }: Props = $props();

  let showSettingsModal = $state(false);

  function handleSelect(mode: "webrtc" | "direct") {
    onSelectNetworkMode?.(mode);
    showSettingsModal = false;
  }
</script>

<header class="w-full bg-[#141519] border-b border-[#262730] px-5 py-3 flex items-center justify-between sticky top-0 z-50">
  <div class="flex items-center gap-3">
    <div class="w-8 h-8 rounded-[4px] bg-[#1e2026] border border-[#31333e] flex items-center justify-center text-amber-500 shadow-sm">
      <FolderSync class="w-4 h-4" />
    </div>
    <div class="text-left">
      <h1 class="text-sm font-semibold tracking-tight text-white flex items-center gap-1.5">
        <span>Wormhole Sync</span>
        <span class="text-[10px] px-1.5 py-0.2 rounded bg-amber-500/10 text-amber-400 font-mono font-normal border border-amber-500/20">v1.2</span>
      </h1>
      <p class="text-[11px] text-neutral-400">Modpack & folder sync</p>
    </div>
  </div>

  <div class="flex items-center gap-2">
    <!-- Tailscale Badge (if active) -->
    {#if tailscaleIP}
      <div class="flex items-center gap-1.5 px-2.5 py-1 rounded-[3px] bg-[#102419] border border-[#1e462e] text-emerald-400 text-xs font-mono" title="Tailscale VPN active">
        <span class="w-1.5 h-1.5 rounded-full bg-emerald-400"></span>
        <span class="text-neutral-400 font-sans text-xs">Tailscale:</span> {tailscaleIP}
      </div>
    {/if}

    <!-- Connection Mode Status / Settings Pill -->
    <button
      type="button"
      onclick={() => (showSettingsModal = true)}
      class="flex items-center gap-1.5 px-2.5 py-1 rounded-[3px] bg-[#1e2026] hover:bg-[#272932] border border-[#2e303b] text-neutral-300 text-xs transition-colors cursor-pointer active:translate-y-[1px]"
      title="Click to view connection options"
    >
      {#if networkMode === "webrtc"}
        <span class="w-1.5 h-1.5 rounded-full bg-emerald-400"></span>
        <Globe class="w-3.5 h-3.5 text-neutral-400" />
        <span class="text-neutral-200 font-medium">Global P2P</span>
      {:else}
        <Cable class="w-3.5 h-3.5 text-purple-400" />
        <span class="text-neutral-200 font-medium">Direct IP / LAN</span>
      {/if}
      <Settings2 class="w-3 h-3 text-neutral-500 ml-0.5" />
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

<!-- Connection Mode Settings Modal -->
{#if showSettingsModal}
  <div class="fixed inset-0 bg-black/60 backdrop-blur-sm z-50 flex items-center justify-center p-4">
    <div class="bg-[#15161b] rounded-[6px] border border-[#2c2e3a] p-5 max-w-md w-full shadow-2xl animate-in fade-in zoom-in-95 duration-150">
      <div class="flex items-center justify-between pb-3 border-b border-[#23242e] mb-4">
        <h3 class="text-sm font-semibold text-white flex items-center gap-2">
          <Settings2 class="w-4 h-4 text-amber-500" />
          <span>Connection Mode</span>
        </h3>
        <button
          type="button"
          onclick={() => (showSettingsModal = false)}
          class="text-neutral-400 hover:text-white transition-colors cursor-pointer"
        >
          <X class="w-4 h-4" />
        </button>
      </div>

      <div class="space-y-3">
        <!-- Option 1: Global Wormhole (Recommended) -->
        <button
          type="button"
          onclick={() => handleSelect("webrtc")}
          class="w-full text-left p-3.5 rounded-[4px] border transition-all cursor-pointer flex items-start gap-3 {networkMode === 'webrtc' ? 'bg-[#18231c] border-[#225735] text-white' : 'bg-[#111216] border-[#252732] hover:border-[#383a48] text-neutral-300'}"
        >
          <div class="w-8 h-8 rounded-[3px] bg-[#102419] border border-[#1e462e] text-emerald-400 flex items-center justify-center flex-shrink-0 mt-0.5">
            <Globe class="w-4 h-4" />
          </div>
          <div class="flex-1">
            <div class="flex items-center justify-between mb-0.5">
              <span class="text-xs font-semibold {networkMode === 'webrtc' ? 'text-emerald-300' : 'text-white'}">
                Global Wormhole (Recommended)
              </span>
              {#if networkMode === "webrtc"}
                <Check class="w-4 h-4 text-emerald-400" />
              {/if}
            </div>
            <p class="text-[11px] text-neutral-400 leading-relaxed">
              Connect anywhere in the world using a simple 8-character code. Direct P2P with encrypted transit relay fallback through strict firewalls and CGNATs.
            </p>
          </div>
        </button>

        <!-- Option 2: Direct IP / Tailscale -->
        <button
          type="button"
          onclick={() => handleSelect("direct")}
          class="w-full text-left p-3.5 rounded-[4px] border transition-all cursor-pointer flex items-start gap-3 {networkMode === 'direct' ? 'bg-[#22172f] border-[#532e7a] text-white' : 'bg-[#111216] border-[#252732] hover:border-[#383a48] text-neutral-300'}"
        >
          <div class="w-8 h-8 rounded-[3px] bg-[#231535] border border-[#4c2777] text-purple-400 flex items-center justify-center flex-shrink-0 mt-0.5">
            <Cable class="w-4 h-4" />
          </div>
          <div class="flex-1">
            <div class="flex items-center justify-between mb-0.5">
              <span class="text-xs font-semibold {networkMode === 'direct' ? 'text-purple-300' : 'text-white'}">
                Direct IP / Local LAN / Tailscale
              </span>
              {#if networkMode === "direct"}
                <Check class="w-4 h-4 text-purple-400" />
              {/if}
            </div>
            <p class="text-[11px] text-neutral-400 leading-relaxed">
              Target a specific IP:Port directly without rendezvous signaling. Ideal for devices on the same Wi-Fi router or connected via a private Tailscale network.
            </p>
          </div>
        </button>
      </div>

      <div class="mt-5 pt-3 border-t border-[#23242e] flex justify-end">
        <button
          type="button"
          onclick={() => (showSettingsModal = false)}
          class="px-3.5 py-1.5 rounded-[3px] bg-[#22242d] hover:bg-[#2b2d39] text-white text-xs font-medium border border-[#353746] transition-colors cursor-pointer"
        >
          Close
        </button>
      </div>
    </div>
  </div>
{/if}
