<script lang="ts">
  import {
    Folder,
    KeyRound,
    Copy,
    Check,
    Play,
    Pause,
    Square,
    X,
    ArrowLeft,
    AlertCircle,
    CheckCircle2,
    Activity,
    RefreshCw,
    CopyCheck,
    Trash2
  } from "@lucide/svelte";
  import FileTree from "./FileTree.svelte";
  import { SelectFolder, ScanFolder, StartSender, Pause as WailsPause, Resume as WailsResume, Stop as WailsStop } from "../../wailsjs/go/main/App";
  import type { models } from "../../wailsjs/go/models";
  import { playCompletionSound, playErrorSound } from "../lib/sound";
  import { formatBytes } from "../lib/format";

  interface Props {
    networkMode: "webrtc" | "direct";
    currentStats: models.SyncStats;
    onClose: () => void;
  }

  let { networkMode, currentStats, onClose }: Props = $props();

  let instanceFolder = $state("");
  let fileTree = $state<models.FileTreeNode | null>(null);
  let selectedPaths = $state<string[]>([]);
  let absoluteSyncFolders = $state<string[]>([]);
  let isScanningTree = $state(false);

  let roomCode = $state("");
  let directAddresses = $state<string[]>([]);
  let copied = $state(false);
  let localError = $state("");
  let isSessionStarted = $state(false);
  let isPaused = $state(false);
  let soundPlayed = $state(false);

  let isComplete = $derived(currentStats.status === "finished" && currentStats.role === "sender");
  let isError = $derived(currentStats.status === "error" && currentStats.role === "sender");
  let isSyncing = $derived(currentStats.status === "syncing" && currentStats.role === "sender");

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
    if (isSessionStarted) return;
    try {
      const selected = await SelectFolder();
      if (selected) {
        instanceFolder = selected;
        localError = "";
        await loadFileTree(selected);
      }
    } catch (err: unknown) {
      localError = err instanceof Error ? err.message : String(err);
    }
  }

  async function loadFileTree(folderPath: string) {
    isScanningTree = true;
    try {
      const tree = await ScanFolder(folderPath);
      fileTree = tree;
    } catch (err: unknown) {
      localError = err instanceof Error ? err.message : String(err);
    } finally {
      isScanningTree = false;
    }
  }

  function handleSelectionChange(paths: string[], absFolders: string[]) {
    selectedPaths = paths;
    absoluteSyncFolders = absFolders;
  }

  async function handleStartSender() {
    if (!instanceFolder) {
      localError = "Please select an instance folder first.";
      return;
    }
    if (selectedPaths.length === 0) {
      localError = "Please select at least one file or folder to sync.";
      return;
    }

    localError = "";
    isSessionStarted = true;
    soundPlayed = false;

    try {
      const result = await StartSender(instanceFolder, selectedPaths, absoluteSyncFolders, networkMode);
      if (result) {
        roomCode = result.code;
        directAddresses = result.directAddresses || [];
      }
    } catch (err: unknown) {
      isSessionStarted = false;
      localError = err instanceof Error ? err.message : String(err);
      playErrorSound();
    }
  }

  async function handlePauseResume() {
    if (isPaused) {
      await WailsResume();
      isPaused = false;
    } else {
      await WailsPause();
      isPaused = true;
    }
  }

  async function handleStop() {
    await WailsStop();
    isSessionStarted = false;
    isPaused = false;
  }

  function copyCode() {
    const textToCopy = networkMode === "direct" && directAddresses.length > 0 ? directAddresses[0] : roomCode;
    navigator.clipboard.writeText(textToCopy);
    copied = true;
    setTimeout(() => {
      copied = false;
    }, 2000);
  }
</script>

<div class="max-w-5xl mx-auto px-5 py-5 flex flex-col min-h-[calc(100vh-65px)]">
  <!-- Top Navigation -->
  <div class="flex items-center justify-between mb-4 pb-2 border-b border-[#23242e]">
    <button
      type="button"
      onclick={onClose}
      disabled={isSyncing}
      class="flex items-center gap-1.5 text-xs text-neutral-400 hover:text-white transition-colors cursor-pointer disabled:opacity-50"
    >
      <ArrowLeft class="w-3.5 h-3.5" />
      <span>Back</span>
    </button>
  </div>

  <!-- Content Grid -->
  <div class="grid grid-cols-1 lg:grid-cols-12 gap-4 flex-1 items-start">
    <!-- Left Column: Folder Selection & File Tree -->
    <div class="lg:col-span-7 flex flex-col gap-3.5 bg-[#15161b] p-4 rounded-[4px] border border-[#262731] shadow-sm">
      <div>
        <div class="flex items-center justify-between mb-1">
          <label for="instance-folder-input" class="text-xs text-neutral-300 font-medium">
            Folder to send
          </label>
          {#if instanceFolder}
            <button
              type="button"
              onclick={() => loadFileTree(instanceFolder)}
              disabled={isSessionStarted}
              class="text-xs text-amber-400 hover:text-amber-300 flex items-center gap-1 cursor-pointer disabled:opacity-50"
            >
              <RefreshCw class="w-3 h-3" />
              <span>Rescan</span>
            </button>
          {/if}
        </div>
        <div class="flex gap-2">
          <input
            id="instance-folder-input"
            type="text"
            readonly
            value={instanceFolder}
            placeholder="Select folder..."
            class="flex-1 bg-[#111216] border border-[#292b36] text-neutral-200 text-xs rounded-[3px] px-3 py-1.5 focus:outline-none font-mono truncate"
          />
          <button
            type="button"
            onclick={handleBrowseFolder}
            disabled={isSessionStarted}
            class="px-3.5 py-1.5 rounded-[3px] bg-[#22242d] hover:bg-[#2b2d39] border border-[#353746] text-neutral-200 text-xs font-medium flex items-center gap-1.5 transition-colors cursor-pointer flex-shrink-0 disabled:opacity-50 active:translate-y-[1px]"
          >
            <Folder class="w-3.5 h-3.5 text-amber-500" />
            <span>Browse</span>
          </button>
        </div>
      </div>

      <!-- File Tree Component -->
      <div class="flex flex-col flex-1">
        <div class="text-xs text-neutral-300 font-medium mb-1 flex items-center justify-between">
          <span>Files to sync</span>
          <span class="text-[11px] text-neutral-500 font-normal">saves & logs unselected by default</span>
        </div>

        {#if isScanningTree}
          <div class="h-64 rounded-[3px] border border-[#262731] bg-[#111216] flex flex-col items-center justify-center gap-2 text-neutral-400">
            <RefreshCw class="w-4 h-4 animate-spin text-amber-400" />
            <span class="text-xs">Scanning folder...</span>
          </div>
        {:else if fileTree}
          <FileTree rootNode={fileTree} onSelectionChange={handleSelectionChange} />
        {:else}
          <div class="h-64 rounded-[3px] border border-dashed border-[#282a35] bg-[#111216]/50 flex flex-col items-center justify-center gap-2 text-neutral-500 p-6 text-center">
            <Folder class="w-7 h-7 text-neutral-600" />
            <p class="text-xs">Choose your folder above to view and select files.</p>
          </div>
        {/if}
      </div>
    </div>

    <!-- Right Column: Code & Controls -->
    <div class="lg:col-span-5 flex flex-col gap-4 bg-[#15161b] p-4 rounded-[4px] border border-[#262731] shadow-sm">
      <div>
        <h3 class="text-sm font-semibold text-white">Connection & Code</h3>
        <p class="text-xs text-neutral-400 mt-0.5">
          Share this code with your friend to connect.
        </p>
      </div>

      <!-- Code Box -->
      <div class="p-3.5 rounded-[3px] bg-[#111216] border border-[#262731] flex flex-col gap-2">
        <span class="text-xs text-neutral-300 font-medium flex items-center justify-between">
          <span>Code</span>
        </span>

        {#if !isSessionStarted}
          <div class="w-full bg-[#171820] border border-[#2a2c37] text-neutral-400 text-xs rounded-[3px] px-2.5 py-1.5 flex items-center gap-2">
            <KeyRound class="w-3.5 h-3.5 text-amber-500 flex-shrink-0" />
            <span>A unique time-based code is generated automatically when you start.</span>
          </div>
        {:else}
          <div class="flex items-center justify-between bg-[#191a22] border border-[#2e303d] rounded-[3px] px-3.5 py-2.5">
            <div class="flex items-center gap-2.5">
              <KeyRound class="w-4 h-4 text-amber-500" />
              <div class="font-mono text-base font-bold tracking-wide text-white break-all">
                {roomCode || "Connecting..."}
              </div>
            </div>
            <button
              type="button"
              onclick={copyCode}
              class="px-2.5 py-1 rounded-[2px] bg-[#22242d] hover:bg-[#2b2d39] text-neutral-200 text-xs font-medium flex items-center gap-1.5 transition-colors cursor-pointer border border-[#353746] active:translate-y-[1px]"
            >
              {#if copied}
                <Check class="w-3 h-3 text-emerald-400" />
                <span class="text-emerald-400">Copied</span>
              {:else}
                <Copy class="w-3 h-3 text-neutral-400" />
                <span>Copy</span>
              {/if}
            </button>
          </div>

          {#if networkMode === "direct" && directAddresses.length > 0}
            <div class="text-xs text-neutral-400 mt-1">
              Direct IP: <span class="text-neutral-200 select-all font-mono font-medium">{directAddresses[0]}</span>
            </div>
          {/if}
        {/if}

        {#if absoluteSyncFolders.length > 0}
          <div class="p-2 rounded-[2px] bg-[#211a0c] border border-[#483713] text-amber-300 text-xs flex items-center gap-2">
            <CopyCheck class="w-3.5 h-3.5 text-amber-400 flex-shrink-0" />
            <span>Mirroring: <strong class="text-amber-200">{absoluteSyncFolders.join(", ")}</strong> (extra files in these folders will be deleted on peer)</span>
          </div>
        {/if}
      </div>

      <!-- Live Transfer Status & Progress Bar -->
      {#if isSessionStarted}
        <div class="p-3.5 rounded-[3px] bg-[#111216] border border-[#262731] flex flex-col gap-2.5">
          <div class="flex items-center justify-between text-xs">
            <span class="font-medium text-neutral-300 flex items-center gap-1.5">
              {#if isSyncing}
                <span class="w-1.5 h-1.5 rounded-full bg-amber-400"></span>
                <span>Syncing files...</span>
              {:else if isComplete}
                <CheckCircle2 class="w-3.5 h-3.5 text-emerald-400" />
                <span class="text-emerald-400">Sync complete</span>
              {:else if isPaused}
                <span class="text-amber-400">Paused</span>
              {:else}
                <span>{currentStats.status}</span>
              {/if}
            </span>

            <!-- Transfer Speed in MB/s -->
            <span class="font-mono font-bold text-xs text-amber-400 flex items-center gap-1 tabular-nums">
              <Activity class="w-3 h-3 text-amber-500" />
              <span>{currentStats.speedFormatted || "0 B/s"}</span>
            </span>
          </div>

          <!-- Progress Bar -->
          <div class="w-full bg-[#1c1d25] h-2 rounded-[2px] overflow-hidden">
            <div
              class="h-full transition-all duration-200 {isComplete ? 'bg-emerald-500' : 'bg-amber-500'}"
              style="width: {currentStats.percent || 0}%"
            ></div>
          </div>

          <div class="flex items-center justify-between text-xs text-neutral-400 font-mono tabular-nums">
            <span>{Math.round(currentStats.percent || 0)}%</span>
            {#if currentStats.totalBytes > 0}
              <span>{formatBytes(currentStats.transferredBytes || 0)} / {formatBytes(currentStats.totalBytes)}</span>
            {/if}
          </div>

          <!-- Message & Current File -->
          <div class="text-xs text-neutral-300 truncate bg-[#161720] px-2.5 py-1 rounded-[2px] border border-[#242531]">
            {currentStats.message || "Waiting for friend to connect..."}
          </div>

          {#if currentStats.currentFile && !isComplete}
            <div class="text-xs text-neutral-400 font-mono truncate">
              {currentStats.currentFile}
            </div>
          {/if}

          {#if currentStats.prunedFiles > 0}
            <div class="text-xs text-neutral-400 flex items-center gap-1.5 tabular-nums">
              <Trash2 class="w-3 h-3 text-neutral-500" />
              <span>Deleted {currentStats.prunedFiles} extra file(s) on peer</span>
            </div>
          {/if}
        </div>
      {/if}

      <!-- Errors -->
      {#if localError}
        <div class="p-2.5 rounded-[3px] bg-[#291314] border border-[#521c1f] text-rose-300 text-xs flex items-center gap-2">
          <AlertCircle class="w-4 h-4 flex-shrink-0 text-rose-400" />
          <span>{localError}</span>
        </div>
      {/if}

      <!-- Control Buttons -->
      <div class="mt-auto flex flex-col gap-2 pt-2">
        {#if !isSessionStarted}
          <button
            type="button"
            onclick={handleStartSender}
            disabled={!instanceFolder || selectedPaths.length === 0}
            class="w-full py-2.5 rounded-[3px] bg-amber-600 hover:bg-amber-500 disabled:opacity-40 text-black font-semibold text-xs border border-amber-500 transition-colors flex items-center justify-center gap-2 cursor-pointer active:translate-y-[1px] shadow-sm"
          >
            <Play class="w-3.5 h-3.5 text-black" />
            <span>Start sync</span>
          </button>
        {:else if !isComplete && !isError}
          <div class="grid grid-cols-2 gap-2">
            <!-- Pause / Resume (only meaningful during an active transfer) -->
            <button
              type="button"
              onclick={handlePauseResume}
              disabled={!isSyncing}
              class="py-2 rounded-[3px] bg-[#22242d] hover:bg-[#2b2d39] text-neutral-200 text-xs font-medium border border-[#353746] flex items-center justify-center gap-1.5 transition-colors cursor-pointer active:translate-y-[1px] disabled:opacity-40 disabled:cursor-not-allowed"
            >
              {#if isPaused}
                <Play class="w-3.5 h-3.5 text-emerald-400" />
                <span>Resume</span>
              {:else}
                <Pause class="w-3.5 h-3.5 text-amber-400" />
                <span>Pause</span>
              {/if}
            </button>

            <!-- Stop -->
            <button
              type="button"
              onclick={handleStop}
              class="py-2 rounded-[3px] bg-[#2a1315] hover:bg-[#381a1d] text-rose-300 text-xs font-medium border border-[#521c20] flex items-center justify-center gap-1.5 transition-colors cursor-pointer active:translate-y-[1px]"
            >
              <Square class="w-3.5 h-3.5" />
              <span>Stop</span>
            </button>
          </div>
        {:else}
          <!-- Close button once finished or on error -->
          <button
            type="button"
            onclick={onClose}
            class="w-full py-2 rounded-[3px] {isComplete ? 'bg-emerald-600 hover:bg-emerald-500 border border-emerald-500' : 'bg-[#22242d] hover:bg-[#2b2d39] border border-[#353746]'} text-white font-medium text-xs transition-colors flex items-center justify-center gap-1.5 cursor-pointer active:translate-y-[1px]"
          >
            <X class="w-3.5 h-3.5" />
            <span>Close</span>
          </button>
        {/if}
      </div>
    </div>
  </div>
</div>
