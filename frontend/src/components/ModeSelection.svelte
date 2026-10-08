<script lang="ts">
  import { Download, Upload, ShieldCheck, Zap, Layers, Network } from "@lucide/svelte";

  interface Props {
    onSelectMode: (mode: "receive" | "send") => void;
  }

  let { onSelectMode }: Props = $props();
</script>

<div class="max-w-4xl mx-auto px-6 py-12 flex flex-col items-center justify-center min-h-[calc(100vh-80px)]">
  <!-- Hero Section -->
  <div class="text-center mb-12 max-w-xl">
    <div class="inline-flex items-center gap-2 px-3 py-1 rounded-full bg-indigo-500/10 border border-indigo-500/20 text-indigo-400 text-xs font-semibold mb-4">
      <Zap class="w-3.5 h-3.5" />
      <span>Delta Sync Technology</span>
    </div>
    <h2 class="text-3xl font-extrabold tracking-tight text-white mb-3">
      Sync Minecraft Instances in Seconds
    </h2>
    <p class="text-slate-400 text-sm leading-relaxed">
      Exchange a simple code with your friend. Only modified or missing files are transferred, verified with SHA-256 and committed with atomic writes.
    </p>
  </div>

  <!-- Selection Cards -->
  <div class="grid grid-cols-1 md:grid-cols-2 gap-6 w-full max-w-3xl">
    <!-- Receive Files Card -->
    <button
      type="button"
      onclick={() => onSelectMode("receive")}
      class="group relative text-left p-8 rounded-2xl bg-slate-900/60 hover:bg-slate-800/80 border border-slate-800 hover:border-indigo-500/50 shadow-xl shadow-black/40 hover:shadow-indigo-500/10 transition-all duration-300 cursor-pointer overflow-hidden flex flex-col justify-between"
    >
      <div class="absolute -right-12 -top-12 w-40 h-40 bg-indigo-500/10 rounded-full blur-2xl group-hover:bg-indigo-500/20 transition-all duration-500"></div>

      <div>
        <div class="w-14 h-14 rounded-2xl bg-indigo-500/10 border border-indigo-500/30 flex items-center justify-center mb-6 text-indigo-400 group-hover:scale-110 group-hover:bg-indigo-500 group-hover:text-white transition-all duration-300 shadow-md">
          <Download class="w-7 h-7" />
        </div>
        <h3 class="text-xl font-bold text-white mb-2 group-hover:text-indigo-300 transition-colors">
          Receive Files
        </h3>
        <p class="text-slate-400 text-sm leading-relaxed mb-6">
          Connect to a friend's session using their code. Choose your local Minecraft folder to selectively download only updated delta files.
        </p>
      </div>

      <div class="flex items-center justify-between pt-4 border-t border-slate-800/80">
        <span class="text-xs font-medium text-slate-500 group-hover:text-slate-400">Target Folder & Code</span>
        <div class="flex items-center gap-1 text-indigo-400 font-semibold text-sm group-hover:translate-x-1 transition-transform">
          <span>Join</span>
          <span>&rarr;</span>
        </div>
      </div>
    </button>

    <!-- Send Files Card -->
    <button
      type="button"
      onclick={() => onSelectMode("send")}
      class="group relative text-left p-8 rounded-2xl bg-slate-900/60 hover:bg-slate-800/80 border border-slate-800 hover:border-purple-500/50 shadow-xl shadow-black/40 hover:shadow-purple-500/10 transition-all duration-300 cursor-pointer overflow-hidden flex flex-col justify-between"
    >
      <div class="absolute -right-12 -top-12 w-40 h-40 bg-purple-500/10 rounded-full blur-2xl group-hover:bg-purple-500/20 transition-all duration-500"></div>

      <div>
        <div class="w-14 h-14 rounded-2xl bg-purple-500/10 border border-purple-500/30 flex items-center justify-center mb-6 text-purple-400 group-hover:scale-110 group-hover:bg-purple-500 group-hover:text-white transition-all duration-300 shadow-md">
          <Upload class="w-7 h-7" />
        </div>
        <h3 class="text-xl font-bold text-white mb-2 group-hover:text-purple-300 transition-colors">
          Send Files
        </h3>
        <p class="text-slate-400 text-sm leading-relaxed mb-6">
          Select your instance folder, pick which mods/configs to sync via an interactive file tree, and share your session code with your friend.
        </p>
      </div>

      <div class="flex items-center justify-between pt-4 border-t border-slate-800/80">
        <span class="text-xs font-medium text-slate-500 group-hover:text-slate-400">File Tree & Code Share</span>
        <div class="flex items-center gap-1 text-purple-400 font-semibold text-sm group-hover:translate-x-1 transition-transform">
          <span>Host</span>
          <span>&rarr;</span>
        </div>
      </div>
    </button>
  </div>

  <!-- Feature Highlights -->
  <div class="grid grid-cols-3 gap-6 mt-12 w-full max-w-3xl pt-8 border-t border-slate-800/60 text-slate-400 text-xs text-center">
    <div class="flex flex-col items-center gap-1.5">
      <Layers class="w-4 h-4 text-indigo-400" />
      <span class="font-medium text-slate-300">Delta Synchronization</span>
      <span class="text-slate-500 text-[11px]">Transfers only changed files</span>
    </div>
    <div class="flex flex-col items-center gap-1.5">
      <ShieldCheck class="w-4 h-4 text-emerald-400" />
      <span class="font-medium text-slate-300">Atomic Safe Writes</span>
      <span class="text-slate-500 text-[11px]">Zero corrupted files on abort</span>
    </div>
    <div class="flex flex-col items-center gap-1.5">
      <Network class="w-4 h-4 text-purple-400" />
      <span class="font-medium text-slate-300">P2P NAT Hole Punch</span>
      <span class="text-slate-500 text-[11px]">Direct WebRTC or Tailscale</span>
    </div>
  </div>
</div>
