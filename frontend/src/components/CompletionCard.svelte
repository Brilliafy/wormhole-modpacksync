<script lang="ts">
  import { onMount } from "svelte";
  import { CheckCircle2, Folder, ArrowRight, Zap, Trash2, HardDriveDownload } from "@lucide/svelte";
  import confetti from "canvas-confetti";
  import { OpenFolder } from "../../wailsjs/go/main/App";
  import type { models } from "../../wailsjs/go/models";
  import { formatBytes } from "../lib/format";

  interface Props {
    role: "sender" | "receiver";
    stats: models.SyncStats;
    targetFolder?: string;
    onDone: () => void;
  }

  let { role, stats, targetFolder = "", onDone }: Props = $props();

  let openError = $state("");

  onMount(() => {
    try {
      confetti({
        particleCount: 65,
        spread: 60,
        origin: { y: 0.65 },
        colors: ["#10b981", "#f59e0b", "#38bdf8", "#a855f7"]
      });
    } catch {
      // Ignore if webview canvas is not ready
    }
  });

  async function handleOpenFolder() {
    if (!targetFolder) return;
    openError = "";
    try {
      await OpenFolder(targetFolder);
    } catch (e: unknown) {
      openError = e instanceof Error ? e.message : String(e);
    }
  }
</script>

<div class="w-full bg-[#15161b] rounded-[6px] border border-[#23422e] p-6 shadow-xl flex flex-col items-center text-center animate-in fade-in zoom-in-95 duration-200">
  <!-- Glowing Success Icon -->
  <div class="w-14 h-14 rounded-full bg-[#102a1c] border border-[#1e5836] flex items-center justify-center text-emerald-400 mb-4 shadow-[0_0_25px_rgba(16,185,129,0.25)]">
    <CheckCircle2 class="w-8 h-8 stroke-[2.2]" />
  </div>

  <h3 class="text-xl font-bold tracking-tight text-white mb-1">
    {role === "sender" ? "Sync Completed!" : "Modpack Ready!"}
  </h3>
  <p class="text-xs text-neutral-400 max-w-sm mb-6 leading-relaxed">
    {role === "sender"
      ? "Your friend's instance has been fully synchronized with your selected files."
      : "All files have been verified, synced, and placed in your instance folder."}
  </p>

  <!-- Metrics Grid -->
  <div class="grid grid-cols-2 gap-3 w-full mb-5">
    <div class="bg-[#101115] p-3 rounded-[4px] border border-[#262731] flex flex-col items-center justify-center">
      <span class="text-[11px] text-neutral-400 mb-0.5">Files Processed</span>
      <span class="font-mono text-base font-bold text-white tabular-nums">
        {stats.totalFiles || stats.completedFiles || 0}
      </span>
    </div>

    <div class="bg-[#101115] p-3 rounded-[4px] border border-[#262731] flex flex-col items-center justify-center">
      <span class="text-[11px] text-neutral-400 mb-0.5">Data Transferred</span>
      <span class="font-mono text-base font-bold text-emerald-400 tabular-nums">
        {formatBytes(stats.transferredBytes || 0)}
      </span>
    </div>
  </div>

  <!-- Delta Sync Bandwidth Savings Banner -->
  {#if stats.savedBytes && stats.savedBytes > 0}
    <div class="w-full mb-4 px-3.5 py-2.5 rounded-[4px] bg-[#142319] border border-[#214d31] flex items-center gap-2.5 text-left text-xs text-emerald-300">
      <Zap class="w-4 h-4 text-emerald-400 flex-shrink-0" />
      <div>
        <span class="font-semibold text-emerald-200">Saved {formatBytes(stats.savedBytes)} bandwidth!</span>
        <span class="text-emerald-400/80 block text-[11px]">Identical files were detected and skipped via delta hashing.</span>
      </div>
    </div>
  {/if}

  <!-- Pruned files notice -->
  {#if stats.prunedFiles && stats.prunedFiles > 0}
    <div class="w-full mb-4 px-3.5 py-2 rounded-[4px] bg-[#1a1b22] border border-[#2d2f3d] flex items-center gap-2 text-left text-xs text-neutral-300">
      <Trash2 class="w-3.5 h-3.5 text-neutral-400 flex-shrink-0" />
      <span>Cleaned up {stats.prunedFiles} outdated file{stats.prunedFiles === 1 ? '' : 's'} in mirrored folders.</span>
    </div>
  {/if}

  {#if openError}
    <p class="text-xs text-rose-400 mb-3">{openError}</p>
  {/if}

  <!-- Action Buttons -->
  <div class="w-full flex flex-col sm:flex-row gap-2.5 pt-2">
    {#if targetFolder}
      <button
        type="button"
        onclick={handleOpenFolder}
        class="flex-1 py-2.5 px-4 rounded-[4px] bg-[#1d2029] hover:bg-[#262935] border border-[#373a4b] text-white text-xs font-medium flex items-center justify-center gap-2 transition-colors cursor-pointer active:translate-y-[1px]"
      >
        <Folder class="w-3.5 h-3.5 text-amber-400" />
        <span>Open Folder</span>
      </button>
    {/if}

    <button
      type="button"
      onclick={onDone}
      class="flex-1 py-2.5 px-4 rounded-[4px] bg-emerald-600 hover:bg-emerald-500 border border-emerald-500 text-white text-xs font-semibold flex items-center justify-center gap-2 transition-colors cursor-pointer active:translate-y-[1px] shadow-sm"
    >
      <span>Finish</span>
      <ArrowRight class="w-3.5 h-3.5" />
    </button>
  </div>
</div>
