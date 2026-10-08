<script lang="ts">
  import { Folder, KeyRound, Download, X, AlertCircle, ArrowLeft, CheckCircle2, CopyCheck, Trash2 } from "@lucide/svelte";
  import ProgressRing from "./ProgressRing.svelte";
  import { SelectFolder, StartReceiver } from "../../wailsjs/go/main/App";
  import type { models } from "../../wailsjs/go/models";
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
      localError = "Please enter the session code or IP from host.";
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

<div class="max-w-lg mx-auto px-5 py-8 flex flex-col items-center justify-center min-h-[calc(100vh-65px)]">
  {#if !isLocked}
    <!-- Step 1 & 2: Setup Form -->
    <div class="w-full bg-[#15161b] rounded-[4px] border border-[#262731] p-6 shadow-sm">
      <!-- Top header -->
      <div class="flex items-center justify-between mb-5 pb-3 border-b border-[#23242e]">
        <button
          type="button"
          onclick={onClose}
          class="flex items-center gap-1.5 text-xs text-neutral-400 hover:text-white transition-colors cursor-pointer"
        >
          <ArrowLeft class="w-3.5 h-3.5" />
          <span>Back</span>
        </button>
        <span class="text-[10px] font-mono uppercase tracking-wider text-neutral-400 border border-[#2c2e39] px-1.5 py-0.5 rounded-[2px] bg-[#1a1b22]">
          CLIENT SETUP
        </span>
      </div>

      <h2 class="text-base font-semibold text-white mb-1">Receive Files</h2>
      <p class="text-neutral-400 text-xs mb-5 leading-relaxed">
        Specify destination folder and enter host session code or direct IP:Port to pull differential updates.
      </p>

      <!-- Target Folder Selection -->
      <div class="mb-4">
        <label for="target-folder-input" class="block text-[11px] font-mono uppercase tracking-wider text-neutral-400 mb-1">
          Destination Folder
        </label>
        <div class="flex gap-2">
          <input
            id="target-folder-input"
            type="text"
            readonly
            value={targetFolder}
            placeholder="Select target folder on disk..."
            class="flex-1 bg-[#111216] border border-[#292b36] text-neutral-200 text-xs rounded-[3px] px-3 py-2 focus:outline-none font-mono truncate"
          />
          <button
            type="button"
            onclick={handleBrowseFolder}
            class="px-3.5 py-2 rounded-[3px] bg-[#22242d] hover:bg-[#2b2d39] border border-[#353746] text-neutral-200 text-xs font-medium flex items-center gap-1.5 transition-colors cursor-pointer flex-shrink-0 active:translate-y-[1px]"
          >
            <Folder class="w-3.5 h-3.5 text-amber-500" />
            <span>Browse</span>
          </button>
        </div>
      </div>

      <!-- Code Entry -->
      <div class="mb-5">
        <label for="room-code-input" class="block text-[11px] font-mono uppercase tracking-wider text-neutral-400 mb-1">
          Host Code {networkMode === "direct" ? "or Direct IP:Port" : ""}
        </label>
        <div class="relative">
          <div class="absolute inset-y-0 left-0 pl-3 flex items-center pointer-events-none">
            <KeyRound class="w-3.5 h-3.5 text-neutral-500" />
          </div>
          <input
            id="room-code-input"
            type="text"
            bind:value={code}
            placeholder={networkMode === "direct" ? "e.g. 100.115.92.3:42424" : "e.g. 849201"}
            class="w-full bg-[#111216] border border-[#292b36] text-white placeholder-neutral-600 text-xs font-mono tracking-wider rounded-[3px] pl-9 pr-3 py-2 focus:outline-none focus:border-amber-500 transition-colors"
          />
        </div>
      </div>

      <!-- Errors -->
      {#if localError}
        <div class="mb-4 p-2.5 rounded-[3px] bg-[#291314] border border-[#521c1f] text-rose-300 text-xs flex items-center gap-2 font-mono">
          <AlertCircle class="w-4 h-4 flex-shrink-0 text-rose-400" />
          <span>{localError}</span>
        </div>
      {/if}

      <!-- Action Button -->
      <button
        type="button"
        onclick={handleStartReceive}
        class="w-full py-2.5 rounded-[3px] bg-sky-600 hover:bg-sky-500 active:translate-y-[1px] border border-sky-500 text-white font-semibold text-xs transition-colors flex items-center justify-center gap-2 cursor-pointer shadow-sm"
      >
        <Download class="w-3.5 h-3.5" />
        <span>Connect & Start Sync</span>
      </button>
    </div>
  {:else}
    <!-- Active Synchronization: UI is disabled, Progress Ring in the center -->
    <div class="w-full flex flex-col items-center justify-center">
      <div class="bg-[#15161b] rounded-[4px] border border-[#262731] p-6 shadow-sm flex flex-col items-center w-full max-w-sm">
        <!-- Circular Gauge Component -->
        <div class="mb-4">
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
            size={240}
            strokeWidth={10}
          />
        </div>

        <!-- Telemetry Readout -->
        <div class="text-center w-full mb-5 space-y-2">
          <h3 class="text-xs font-mono uppercase tracking-wider font-semibold text-white flex items-center justify-center gap-2">
            {#if isComplete}
              <CheckCircle2 class="w-4 h-4 text-emerald-400" />
              <span class="text-emerald-400">SYNC COMPLETE</span>
            {:else if isError}
              <AlertCircle class="w-4 h-4 text-rose-400" />
              <span class="text-rose-400">TRANSFER FAILED</span>
            {:else}
              <span>TRANSFERRING FILES</span>
            {/if}
          </h3>

          <p class="text-[11px] text-neutral-300 font-mono truncate px-3 py-1 bg-[#111216] rounded-[2px] border border-[#23242e]">
            {currentStats.message || "Connecting to host..."}
          </p>

          {#if currentStats.currentFile && !isComplete}
            <div class="text-[10px] text-neutral-400 font-mono truncate px-1">
              FILE: <span class="text-neutral-200">{currentStats.currentFile}</span>
            </div>
          {/if}

          {#if currentStats.totalBytes > 0}
            <div class="text-[10px] text-neutral-400 font-mono tabular-nums">
              {formatBytes(currentStats.transferredBytes || 0)} / {formatBytes(currentStats.totalBytes)}
            </div>
          {/if}

          {#if currentStats.mirroredFolders && currentStats.mirroredFolders.length > 0}
            <div class="text-[10px] text-amber-300 font-mono flex items-center justify-center gap-1.5 px-2 py-0.5 bg-[#211a0c] rounded-[2px] border border-[#483713]">
              <CopyCheck class="w-3 h-3 text-amber-400" />
              <span>1:1 Mirror: <strong>{currentStats.mirroredFolders.join(", ")}</strong></span>
            </div>
          {/if}

          {#if currentStats.prunedFiles > 0}
            <div class="text-[10px] text-neutral-400 font-mono flex items-center justify-center gap-1.5 tabular-nums">
              <Trash2 class="w-3 h-3 text-neutral-500" />
              <span>Pruned {currentStats.prunedFiles} unlisted remote file{currentStats.prunedFiles === 1 ? '' : 's'}</span>
            </div>
          {/if}
        </div>

        <!-- Close Button -->
        {#if isComplete || isError}
          <button
            type="button"
            onclick={onClose}
            class="w-full py-2 rounded-[3px] {isComplete ? 'bg-emerald-600 hover:bg-emerald-500 border border-emerald-500' : 'bg-[#22242d] hover:bg-[#2b2d39] border border-[#353746]'} text-white font-medium text-xs transition-colors flex items-center justify-center gap-1.5 cursor-pointer active:translate-y-[1px]"
          >
            <X class="w-3.5 h-3.5" />
            <span>Close Console</span>
          </button>
        {/if}
      </div>
    </div>
  {/if}
</div>
