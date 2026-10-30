<script lang="ts">
  import { Activity, ShieldCheck, Zap, Pause, Play, Square, HardDrive, FileText, CheckCircle2, Clock } from "@lucide/svelte";
  import type { models } from "../../wailsjs/go/models";
  import { formatBytes, formatSpeed } from "../lib/format";

  interface Props {
    role: "sender" | "receiver";
    stats: models.SyncStats;
    onPause?: () => void;
    onResume?: () => void;
    onStop: () => void;
    isPaused?: boolean;
  }

  let { role, stats, onPause, onResume, onStop, isPaused = false }: Props = $props();

  // ETA Calculation based on current rolling speed
  let etaFormatted = $derived.by(() => {
    if (!stats.totalBytes || !stats.speedBytesPerSec || stats.speedBytesPerSec < 1024) {
      return "";
    }
    const remaining = stats.totalBytes - (stats.transferredBytes || 0);
    if (remaining <= 0) return "";
    const secs = Math.ceil(remaining / stats.speedBytesPerSec);
    if (secs < 60) {
      return `~${secs}s remaining`;
    }
    const mins = Math.floor(secs / 60);
    const remSecs = secs % 60;
    return `~${mins}m ${remSecs}s remaining`;
  });

  // Transport identification pill
  let transportLabel = $derived.by(() => {
    if (stats.mode === "direct") {
      return { text: "Direct IP / LAN", color: "text-purple-400 bg-[#21162e] border-[#452866]" };
    }
    const msg = (stats.message || "").toLowerCase();
    if (msg.includes("transit") || msg.includes("relay")) {
      return { text: "Encrypted Transit Relay", color: "text-sky-400 bg-[#102433] border-[#1d4c6e]" };
    }
    return { text: "Direct P2P (WebRTC)", color: "text-emerald-400 bg-[#102419] border-[#1e462e]" };
  });

  // Shorten current file path for clean display
  let displayFile = $derived.by(() => {
    if (!stats.currentFile) return "";
    const parts = stats.currentFile.split(/[/\\]/);
    if (parts.length <= 2) return stats.currentFile;
    return `${parts[0]}/.../${parts[parts.length - 1]}`;
  });

  let percent = $derived(Math.min(100, Math.max(0, stats.percent || 0)));
  let isSyncing = $derived(stats.status === "syncing");
</script>

<div class="w-full bg-[#15161b] rounded-[6px] border border-[#262731] p-5 shadow-sm flex flex-col gap-4">
  <!-- Top Badges & Status Strip -->
  <div class="flex items-center justify-between gap-2 border-b border-[#23242e] pb-3">
    <div class="flex items-center gap-2">
      <span class="inline-flex items-center gap-1.5 px-2 py-0.5 rounded-[3px] text-xs font-medium border {transportLabel.color}">
        <ShieldCheck class="w-3.5 h-3.5" />
        <span>{transportLabel.text}</span>
      </span>
      {#if isPaused}
        <span class="inline-flex items-center px-2 py-0.5 rounded-[3px] text-xs font-medium bg-[#292212] border border-[#52411e] text-amber-400">
          Paused
        </span>
      {/if}
    </div>

    <!-- Live speed readout with activity pulse -->
    {#if stats.speedFormatted && stats.speedFormatted !== "0 B/s"}
      <div class="flex items-center gap-1.5 text-xs font-mono font-bold text-amber-400 tabular-nums">
        <Activity class="w-3.5 h-3.5 text-amber-500 animate-pulse" />
        <span>{stats.speedFormatted}</span>
      </div>
    {/if}
  </div>

  <!-- Hero Percentage & Main Progress Bar -->
  <div class="flex flex-col gap-2">
    <div class="flex items-baseline justify-between">
      <div class="flex items-baseline gap-2">
        <span class="text-3xl font-mono font-extrabold text-white tabular-nums tracking-tight">
          {Math.round(percent)}<span class="text-lg text-neutral-400 font-normal">%</span>
        </span>
        {#if etaFormatted}
          <span class="text-xs text-neutral-400 flex items-center gap-1">
            <Clock class="w-3 h-3 text-neutral-500" />
            <span>{etaFormatted}</span>
          </span>
        {/if}
      </div>

      <div class="text-xs text-neutral-400 font-mono tabular-nums">
        {#if stats.totalBytes > 0}
          <span>{formatBytes(stats.transferredBytes || 0)} / {formatBytes(stats.totalBytes)}</span>
        {:else if stats.totalFiles > 0}
          <span>{stats.completedFiles} / {stats.totalFiles} files</span>
        {/if}
      </div>
    </div>

    <!-- Progress track -->
    <div class="w-full bg-[#101115] h-2.5 rounded-[3px] overflow-hidden border border-[#23242d] p-[1px]">
      <div
        class="h-full rounded-[2px] bg-gradient-to-r from-amber-500 to-amber-400 transition-all duration-200 ease-out shadow-[0_0_8px_rgba(245,158,11,0.3)]"
        style="width: {percent}%"
      ></div>
    </div>
  </div>

  <!-- Telemetry Readouts -->
  <div class="bg-[#111216] rounded-[4px] border border-[#23242e] p-3 flex flex-col gap-1.5 text-xs">
    <!-- Status message -->
    <div class="flex items-center justify-between text-neutral-300">
      <span class="font-medium text-white truncate">{stats.message || "Working..."}</span>
      {#if stats.totalFiles > 0}
        <span class="font-mono text-neutral-400 text-[11px] tabular-nums flex-shrink-0 ml-2">
          {stats.completedFiles} of {stats.totalFiles} files
        </span>
      {/if}
    </div>

    <!-- Current active file -->
    {#if displayFile}
      <div class="flex items-center gap-1.5 text-neutral-400 font-mono text-[11px] truncate pt-1 border-t border-[#1c1d25]">
        <FileText class="w-3 h-3 text-neutral-500 flex-shrink-0" />
        <span class="truncate">{displayFile}</span>
      </div>
    {/if}
  </div>

  <!-- Control Buttons Strip -->
  <div class="flex items-center justify-end gap-2 pt-1">
    {#if role === "sender" && onPause && onResume}
      <button
        type="button"
        onclick={isPaused ? onResume : onPause}
        disabled={!isSyncing}
        class="px-3.5 py-1.5 rounded-[3px] bg-[#22242d] hover:bg-[#2b2d39] text-neutral-200 text-xs font-medium border border-[#353746] flex items-center gap-1.5 transition-colors cursor-pointer active:translate-y-[1px] disabled:opacity-40 disabled:cursor-not-allowed"
      >
        {#if isPaused}
          <Play class="w-3.5 h-3.5 text-emerald-400" />
          <span>Resume</span>
        {:else}
          <Pause class="w-3.5 h-3.5 text-amber-400" />
          <span>Pause</span>
        {/if}
      </button>
    {/if}

    <button
      type="button"
      onclick={onStop}
      class="px-3.5 py-1.5 rounded-[3px] bg-[#2a1315] hover:bg-[#381a1d] text-rose-300 text-xs font-medium border border-[#521c20] flex items-center gap-1.5 transition-colors cursor-pointer active:translate-y-[1px]"
    >
      <Square class="w-3.5 h-3.5" />
      <span>Cancel</span>
    </button>
  </div>
</div>
