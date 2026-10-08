<script lang="ts">
  import { Download, Upload, ArrowRight, ShieldCheck, CheckCircle2, HardDrive, Cpu, Network } from "@lucide/svelte";

  interface Props {
    onSelectMode: (mode: "receive" | "send") => void;
  }

  let { onSelectMode }: Props = $props();
</script>

<div class="max-w-3xl mx-auto px-5 py-8 flex flex-col justify-center min-h-[calc(100vh-65px)]">
  <!-- Operational Overview Header -->
  <div class="mb-6 border-b border-[#24252e] pb-4">
    <div class="flex items-center gap-2 mb-1">
      <span class="text-[10px] font-mono uppercase tracking-widest text-amber-500 font-semibold bg-[#211a0c] border border-[#483713] px-1.5 py-0.5 rounded-[2px]">
        DIFFERENTIAL ENGINE
      </span>
      <span class="text-xs text-neutral-400 font-mono">v1.1.0</span>
    </div>
    <h2 class="text-xl font-bold tracking-tight text-white">
      P2P File & Modpack Synchronizer
    </h2>
    <p class="text-xs text-neutral-400 mt-1 max-w-xl leading-relaxed">
      Synchronize game instances and directory trees directly between peers. Files with matching cryptographic hashes are skipped, and updates are committed atomically.
    </p>
  </div>

  <!-- Dual Mode Workstation Cards -->
  <div class="grid grid-cols-1 md:grid-cols-2 gap-4">
    <!-- Receive Mode Panel -->
    <div class="bg-[#15161b] border border-[#262731] hover:border-[#383a48] transition-colors p-5 rounded-[4px] flex flex-col justify-between">
      <div>
        <div class="flex items-center justify-between mb-4">
          <div class="w-8 h-8 rounded-[3px] bg-[#1d1f27] border border-[#2e303d] flex items-center justify-center text-sky-400">
            <Download class="w-4 h-4" />
          </div>
          <span class="text-[10px] font-mono uppercase tracking-wider text-neutral-400 border border-[#2b2c37] px-1.5 py-0.5 rounded-[2px] bg-[#1a1b22]">
            CLIENT
          </span>
        </div>

        <h3 class="text-base font-semibold text-white mb-1.5">
          Receive Files
        </h3>
        <p class="text-xs text-neutral-400 leading-relaxed mb-5">
          Connect to a remote host session code or direct IP address. Download modified files into your local instance directory.
        </p>

        <div class="space-y-1.5 font-mono text-[11px] text-neutral-400 border-t border-[#22232c] pt-3 mb-5">
          <div class="flex items-center justify-between">
            <span class="text-neutral-500">Destination:</span>
            <span class="text-neutral-300">Target folder on disk</span>
          </div>
          <div class="flex items-center justify-between">
            <span class="text-neutral-500">Auth:</span>
            <span class="text-neutral-300">6-digit room code / IP</span>
          </div>
          <div class="flex items-center justify-between">
            <span class="text-neutral-500">Integrity:</span>
            <span class="text-emerald-400">SHA-256 verification</span>
          </div>
        </div>
      </div>

      <button
        type="button"
        onclick={() => onSelectMode("receive")}
        class="w-full py-2.5 px-4 rounded-[3px] bg-[#22242d] hover:bg-[#2b2d39] active:translate-y-[1px] border border-[#343644] hover:border-[#484a5c] text-white font-medium text-xs flex items-center justify-center gap-2 transition-all cursor-pointer"
      >
        <span>Open Receive Console</span>
        <ArrowRight class="w-3.5 h-3.5 text-neutral-400" />
      </button>
    </div>

    <!-- Send Mode Panel -->
    <div class="bg-[#15161b] border border-[#262731] hover:border-[#383a48] transition-colors p-5 rounded-[4px] flex flex-col justify-between">
      <div>
        <div class="flex items-center justify-between mb-4">
          <div class="w-8 h-8 rounded-[3px] bg-[#1d1f27] border border-[#2e303d] flex items-center justify-center text-amber-500">
            <Upload class="w-4 h-4" />
          </div>
          <span class="text-[10px] font-mono uppercase tracking-wider text-neutral-400 border border-[#2b2c37] px-1.5 py-0.5 rounded-[2px] bg-[#1a1b22]">
            HOST
          </span>
        </div>

        <h3 class="text-base font-semibold text-white mb-1.5">
          Send Files
        </h3>
        <p class="text-xs text-neutral-400 leading-relaxed mb-5">
          Host a sync session from your instance folder. Inspect directory trees, toggle 1:1 folder mirroring, and share your session code.
        </p>

        <div class="space-y-1.5 font-mono text-[11px] text-neutral-400 border-t border-[#22232c] pt-3 mb-5">
          <div class="flex items-center justify-between">
            <span class="text-neutral-500">Curation:</span>
            <span class="text-neutral-300">Selective tree checkboxes</span>
          </div>
          <div class="flex items-center justify-between">
            <span class="text-neutral-500">Mirroring:</span>
            <span class="text-neutral-300">Optional 1:1 folder pruning</span>
          </div>
          <div class="flex items-center justify-between">
            <span class="text-neutral-500">Streaming:</span>
            <span class="text-amber-400">P2P chunk stream</span>
          </div>
        </div>
      </div>

      <button
        type="button"
        onclick={() => onSelectMode("send")}
        class="w-full py-2.5 px-4 rounded-[3px] bg-amber-600 hover:bg-amber-500 active:translate-y-[1px] border border-amber-500 text-black font-semibold text-xs flex items-center justify-center gap-2 transition-all cursor-pointer shadow-sm"
      >
        <span>Open Host Console</span>
        <ArrowRight class="w-3.5 h-3.5 text-black" />
      </button>
    </div>
  </div>

  <!-- Bottom System Architecture Specs -->
  <div class="mt-6 pt-4 border-t border-[#24252e] flex flex-wrap items-center justify-between gap-3 text-neutral-400 text-[11px] font-mono">
    <div class="flex items-center gap-1.5">
      <HardDrive class="w-3.5 h-3.5 text-neutral-500" />
      <span class="text-neutral-500">ATOMIC SWAP:</span>
      <span class="text-neutral-300">.wormhole.tmp buffer</span>
    </div>
    <div class="flex items-center gap-1.5">
      <ShieldCheck class="w-3.5 h-3.5 text-emerald-500" />
      <span class="text-neutral-500">DIGEST:</span>
      <span class="text-neutral-300">SHA-256 chunk hashes</span>
    </div>
    <div class="flex items-center gap-1.5">
      <Network class="w-3.5 h-3.5 text-sky-400" />
      <span class="text-neutral-500">TRANSPORT:</span>
      <span class="text-neutral-300">WebRTC DataChannel / Direct TCP</span>
    </div>
  </div>
</div>
