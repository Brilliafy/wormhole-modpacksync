<script lang="ts">
  import { Folder, KeyRound, Download, X, AlertCircle, ArrowLeft, CheckCircle2, Sparkles } from "@lucide/svelte";
  import ProgressRing from "./ProgressRing.svelte";
  import { SelectFolder, StartReceiver } from "../../wailsjs/go/main/App";
  import type { models } from "../../wailsjs/go/models";
  import confetti from "canvas-confetti";
  import { playCompletionSound, playErrorSound } from "../lib/sound";
  import { formatBytes } from "../lib/format";

  interface Props {
    networkMode: "webrtc" | "direct";
    currentStats: models.SyncStats;
    onClose: () => void;
  }

  let { networkMode, currentStats, onClose }: Props = $props();

  let targetFolder = $state("");
  let code = $state("");
  let isConnectingOrSyncing = $state(false);
  let localError = $state("");
  let soundPlayed = $state(false);

  // Check if transfer has started
  let isLocked = $derived(
    isConnectingOrSyncing ||
    (currentStats.status !== "idle" && currentStats.role === "receiver")
  );

  let isComplete = $derived(currentStats.status === "finished" && currentStats.role === "receiver");
  let isError = $derived(currentStats.status === "error" && currentStats.role === "receiver");

  $effect(() => {
    if (isComplete && !soundPlayed) {
      soundPlayed = true;
      playCompletionSound();
      try {
        confetti({
          particleCount: 100,
          spread: 70,
          origin: { y: 0.6 }
        });
      } catch (e) {
        // ignore
      }
    } else if (isError && !soundPlayed) {
      soundPlayed = true;
      playErrorSound();
    }
  });

  async function handleBrowseFolder() {
    if (isLocked) return;
    try {
      const selected = await SelectFolder();
      if (selected) {
        targetFolder = selected;
        localError = "";
      }
    } catch (err: unknown) {
      localError = err instanceof Error ? err.message : String(err);
    }
  }

  async function handleStartReceive() {
    if (!targetFolder) {
      localError = "Please select the target folder to sync into.";
      return;
    }
    if (!code.trim()) {
      localError = "Please enter the code from your friend.";
      return;
    }

    localError = "";
    isConnectingOrSyncing = true;
    soundPlayed = false;

    try {
      await StartReceiver(targetFolder, networkMode, code.trim());
    } catch (err: unknown) {
      isConnectingOrSyncing = false;
      localError = err instanceof Error ? err.message : String(err);
      playErrorSound();
    }
  }
</script>

<div class="max-w-xl mx-auto px-6 py-8 flex flex-col items-center justify-center min-h-[calc(100vh-80px)]">
  {#if !isLocked}
    <!-- Step 1 & 2: Setup Form -->
    <div class="w-full bg-slate-900/80 rounded-2xl border border-slate-800 p-8 shadow-2xl backdrop-blur-md">
      <!-- Top header -->
      <div class="flex items-center justify-between mb-6">
        <button
          type="button"
          onclick={onClose}
          class="flex items-center gap-1.5 text-xs text-slate-400 hover:text-white transition-colors cursor-pointer"
        >
          <ArrowLeft class="w-4 h-4" />
          <span>Back</span>
        </button>
        <span class="text-xs font-semibold px-2.5 py-0.5 rounded-full bg-indigo-500/10 text-indigo-300 border border-indigo-500/20">
          Receiver Setup
        </span>
      </div>

      <h2 class="text-2xl font-bold text-white mb-2 tracking-tight">Receive Files</h2>
      <p class="text-slate-400 text-xs mb-6">
        Specify your Minecraft instance folder and enter your friend's code to begin delta synchronization.
      </p>

      <!-- Target Folder Selection -->
      <div class="mb-5">
        <label for="target-folder-input" class="block text-xs font-medium text-slate-300 mb-1.5">
          1. Destination Folder
        </label>
        <div class="flex gap-2">
          <input
            id="target-folder-input"
            type="text"
            readonly
            value={targetFolder}
            placeholder="e.g. ~/.minecraft or PrismLauncher/instances/..."
            class="flex-1 bg-slate-950/80 border border-slate-800 text-slate-200 text-xs rounded-xl px-3.5 py-2.5 focus:outline-none font-mono truncate"
          />
          <button
            type="button"
            onclick={handleBrowseFolder}
            class="px-4 py-2.5 rounded-xl bg-slate-800 hover:bg-slate-700 border border-slate-700 text-slate-200 text-xs font-medium flex items-center gap-1.5 transition-colors cursor-pointer flex-shrink-0"
          >
            <Folder class="w-4 h-4 text-amber-400" />
            <span>Browse</span>
          </button>
        </div>
      </div>

      <!-- Code Entry -->
      <div class="mb-6">
        <label for="room-code-input" class="block text-xs font-medium text-slate-300 mb-1.5">
          2. Enter Friend's Code {networkMode === "direct" ? "or Direct IP:Port" : ""}
        </label>
        <div class="relative">
          <div class="absolute inset-y-0 left-0 pl-3.5 flex items-center pointer-events-none">
            <KeyRound class="w-4 h-4 text-purple-400" />
          </div>
          <input
            id="room-code-input"
            type="text"
            bind:value={code}
            placeholder={networkMode === "direct" ? "e.g. 100.115.92.3:42424" : "e.g. 849201"}
            class="w-full bg-slate-950/80 border border-slate-800 text-white placeholder-slate-500 text-sm font-mono tracking-wider rounded-xl pl-10 pr-4 py-2.5 focus:outline-none focus:border-indigo-500 transition-colors"
          />
        </div>
      </div>

      <!-- Errors -->
      {#if localError}
        <div class="mb-4 p-3 rounded-xl bg-rose-500/10 border border-rose-500/30 text-rose-300 text-xs flex items-center gap-2">
          <AlertCircle class="w-4 h-4 flex-shrink-0" />
          <span>{localError}</span>
        </div>
      {/if}

      <!-- Action Button -->
      <button
        type="button"
        onclick={handleStartReceive}
        class="w-full py-3 rounded-xl bg-gradient-to-r from-indigo-600 via-purple-600 to-pink-600 hover:from-indigo-500 hover:to-pink-500 text-white font-semibold text-sm shadow-lg shadow-purple-500/25 transition-all duration-300 flex items-center justify-center gap-2 cursor-pointer"
      >
        <Download class="w-4 h-4" />
        <span>Connect & Sync Delta</span>
      </button>
    </div>
  {:else}
    <!-- Active Synchronization: UI is disabled, Progress Ring in the center -->
    <div class="w-full flex flex-col items-center justify-center">
      <div class="bg-slate-900/90 rounded-3xl border border-slate-800/80 p-10 shadow-2xl backdrop-blur-xl flex flex-col items-center w-full max-w-md">
        <!-- Circular Progress Ring Component -->
        <div class="mb-6">
          <ProgressRing
            percent={currentStats.percent || 0}
            speedFormatted={currentStats.speedFormatted || "0 B/s"}
            status={currentStats.status}
            message={currentStats.message}
            currentFile={currentStats.currentFile}
            completedFiles={currentStats.completedFiles || 0}
            totalFiles={currentStats.totalFiles || 0}
            transferredBytes={currentStats.transferredBytes || 0}
            totalBytes={currentStats.totalBytes || 0}
            size={270}
            strokeWidth={14}
          />
        </div>

        <!-- Text details inside/under ring -->
        <div class="text-center w-full mb-6">
          <h3 class="text-base font-bold text-white mb-1.5 flex items-center justify-center gap-2">
            {#if isComplete}
              <CheckCircle2 class="w-5 h-5 text-emerald-400" />
              <span>Synchronization Complete</span>
            {:else if isError}
              <AlertCircle class="w-5 h-5 text-rose-400" />
              <span>Transfer Failed</span>
            {:else}
              <span>Receiving Delta Updates</span>
            {/if}
          </h3>

          <p class="text-xs text-slate-300 font-mono truncate px-4 py-1.5 bg-slate-950/60 rounded-lg border border-slate-800/80 mb-2">
            {currentStats.message || "Connecting to sender..."}
          </p>

          {#if currentStats.currentFile && !isComplete}
            <div class="text-[11px] text-slate-400 font-mono truncate px-2">
              File: <span class="text-indigo-300">{currentStats.currentFile}</span>
            </div>
          {/if}

          {#if currentStats.totalBytes > 0}
            <div class="text-[11px] text-slate-500 font-mono mt-1">
              {formatBytes(currentStats.transferredBytes || 0)} / {formatBytes(currentStats.totalBytes)}
            </div>
          {/if}

          {#if currentStats.mirroredFolders && currentStats.mirroredFolders.length > 0}
            <div class="mt-3 text-[11px] text-amber-300 font-mono flex items-center justify-center gap-1.5 px-3 py-1 bg-amber-950/40 rounded-lg border border-amber-500/20">
              <Sparkles class="w-3.5 h-3.5 text-amber-400" />
              <span>Absolute Mirror: <strong>{currentStats.mirroredFolders.join(", ")}</strong></span>
            </div>
          {/if}

          {#if currentStats.prunedFiles > 0}
            <div class="mt-1.5 text-[11px] text-amber-300 font-mono flex items-center justify-center gap-1.5">
              <span>🧹 Pruned {currentStats.prunedFiles} obsolete file{currentStats.prunedFiles === 1 ? '' : 's'}</span>
            </div>
          {/if}
        </div>

        <!-- Close Button (appears when finished or on error) -->
        {#if isComplete || isError}
          <button
            type="button"
            onclick={onClose}
            class="w-full py-3 rounded-xl {isComplete ? 'bg-emerald-600 hover:bg-emerald-500' : 'bg-slate-800 hover:bg-slate-700'} text-white font-semibold text-sm transition-colors flex items-center justify-center gap-2 cursor-pointer shadow-lg"
          >
            <X class="w-4 h-4" />
            <span>Close</span>
          </button>
        {/if}
      </div>
    </div>
  {/if}
</div>
