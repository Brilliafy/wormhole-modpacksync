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
    Zap,
    RefreshCw
  } from "@lucide/svelte";
  import FileTree from "./FileTree.svelte";
  import { SelectFolder, ScanFolder, StartSender, Pause as WailsPause, Resume as WailsResume, Stop as WailsStop } from "../../wailsjs/go/main/App";
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

  let instanceFolder = $state("");
  let fileTree = $state<models.FileTreeNode | null>(null);
  let selectedPaths = $state<string[]>([]);
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
  let isWaitingOrConnecting = $derived(
    (currentStats.status === "waiting" || currentStats.status === "connecting" || currentStats.status === "analyzing" || currentStats.status === "scanning") &&
    currentStats.role === "sender"
  );

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

  function handleSelectionChange(paths: string[]) {
    selectedPaths = paths;
  }

  async function handleStartSender() {
    if (!instanceFolder) {
      localError = "Please select a Minecraft instance folder first.";
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
      const result = await StartSender(instanceFolder, selectedPaths, networkMode, roomCode.trim());
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

<div class="max-w-4xl mx-auto px-6 py-6 flex flex-col min-h-[calc(100vh-80px)]">
  <!-- Top Navigation -->
  <div class="flex items-center justify-between mb-4">
    <button
      type="button"
      onclick={onClose}
      disabled={isSyncing}
      class="flex items-center gap-1.5 text-xs text-slate-400 hover:text-white transition-colors cursor-pointer disabled:opacity-50"
    >
      <ArrowLeft class="w-4 h-4" />
      <span>Back</span>
    </button>
    <div class="flex items-center gap-2">
      <span class="text-xs font-semibold px-2.5 py-0.5 rounded-full bg-purple-500/10 text-purple-300 border border-purple-500/20">
        Sender Console
      </span>
    </div>
  </div>

  <!-- Content Grid -->
  <div class="grid grid-cols-1 lg:grid-cols-12 gap-6 flex-1 items-start">
    <!-- Left Column: Folder Selection & File Tree -->
    <div class="lg:col-span-7 flex flex-col gap-4 bg-slate-900/80 p-5 rounded-2xl border border-slate-800 shadow-xl">
      <div>
        <div class="flex items-center justify-between mb-1.5">
          <label for="instance-folder-input" class="text-xs font-semibold text-slate-300">
            1. Select Instance Folder
          </label>
          {#if instanceFolder}
            <button
              type="button"
              onclick={() => loadFileTree(instanceFolder)}
              disabled={isSessionStarted}
              class="text-[11px] text-indigo-400 hover:text-indigo-300 flex items-center gap-1 cursor-pointer disabled:opacity-50"
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
            placeholder="Choose your Minecraft instance folder..."
            class="flex-1 bg-slate-950/80 border border-slate-800 text-slate-200 text-xs rounded-xl px-3 py-2 focus:outline-none font-mono truncate"
          />
          <button
            type="button"
            onclick={handleBrowseFolder}
            disabled={isSessionStarted}
            class="px-3.5 py-2 rounded-xl bg-slate-800 hover:bg-slate-700 border border-slate-700 text-slate-200 text-xs font-medium flex items-center gap-1.5 transition-colors cursor-pointer flex-shrink-0 disabled:opacity-50"
          >
            <Folder class="w-3.5 h-3.5 text-amber-400" />
            <span>Choose</span>
          </button>
        </div>
      </div>

      <!-- File Tree Component -->
      <div class="flex flex-col flex-1">
        <div class="text-xs font-semibold text-slate-300 mb-1.5 flex items-center justify-between">
          <span>2. Select Files & Folders to Sync</span>
          <span class="text-[11px] font-normal text-slate-500">Uncheck saves/logs to protect personal worlds</span>
        </div>

        {#if isScanningTree}
          <div class="h-64 rounded-xl border border-slate-800 bg-slate-950/60 flex flex-col items-center justify-center gap-2 text-slate-400">
            <RefreshCw class="w-5 h-5 animate-spin text-purple-400" />
            <span class="text-xs font-mono">Scanning directory structure...</span>
          </div>
        {:else if fileTree}
          <FileTree rootNode={fileTree} onSelectionChange={handleSelectionChange} />
        {:else}
          <div class="h-64 rounded-xl border border-dashed border-slate-800 bg-slate-950/30 flex flex-col items-center justify-center gap-3 text-slate-500 p-6 text-center">
            <Folder class="w-8 h-8 text-slate-600" />
            <p class="text-xs">Choose your Minecraft instance directory above to inspect mods, configs, and resourcepacks.</p>
          </div>
        {/if}
      </div>
    </div>

    <!-- Right Column: Code, Connection & Transfer Controls -->
    <div class="lg:col-span-5 flex flex-col gap-5 bg-slate-900/80 p-5 rounded-2xl border border-slate-800 shadow-xl">
      <div>
        <h3 class="text-base font-bold text-white mb-1">Connection & Code</h3>
        <p class="text-xs text-slate-400">
          Share your room code or direct address with your friend.
        </p>
      </div>

      <!-- Code Box -->
      <div class="p-4 rounded-xl bg-slate-950 border border-slate-800 flex flex-col gap-2.5">
        <label for="sender-code-input" class="text-[11px] font-semibold text-slate-400 uppercase tracking-wider flex items-center justify-between">
          <span>Session Code</span>
          <span class="text-purple-400 font-mono">{networkMode === "direct" ? "Direct Mode" : "WebRTC Hole Punch"}</span>
        </label>

        {#if !isSessionStarted}
          <div class="relative">
            <input
              id="sender-code-input"
              type="text"
              bind:value={roomCode}
              placeholder="Leave empty for auto-generated code"
              class="w-full bg-slate-900 border border-slate-800 text-white placeholder-slate-600 text-xs font-mono rounded-lg px-3 py-2 focus:outline-none focus:border-purple-500"
            />
          </div>
        {:else}
          <div class="flex items-center justify-between bg-purple-950/30 border border-purple-500/30 rounded-xl px-4 py-3">
            <div class="flex items-center gap-3">
              <KeyRound class="w-5 h-5 text-purple-400" />
              <div class="font-mono text-xl font-bold tracking-widest text-white">
                {roomCode || "Generating..."}
              </div>
            </div>
            <button
              type="button"
              onclick={copyCode}
              class="px-2.5 py-1.5 rounded-lg bg-purple-500/20 hover:bg-purple-500/30 text-purple-300 text-xs font-semibold flex items-center gap-1.5 transition-colors cursor-pointer"
            >
              {#if copied}
                <Check class="w-3.5 h-3.5 text-emerald-400" />
                <span class="text-emerald-400">Copied</span>
              {:else}
                <Copy class="w-3.5 h-3.5" />
                <span>Copy</span>
              {/if}
            </button>
          </div>

          {#if networkMode === "direct" && directAddresses.length > 0}
            <div class="text-[11px] text-slate-400 font-mono mt-1">
              Direct IP: <span class="text-indigo-300 select-all">{directAddresses[0]}</span>
            </div>
          {/if}
        {/if}
      </div>

      <!-- Live Transfer Status & Progress Bar -->
      {#if isSessionStarted}
        <div class="p-4 rounded-xl bg-slate-950/80 border border-slate-800 flex flex-col gap-3">
          <div class="flex items-center justify-between text-xs">
            <span class="font-semibold text-slate-300 flex items-center gap-1.5">
              {#if isSyncing}
                <span class="w-2 h-2 rounded-full bg-indigo-400 animate-ping"></span>
                <span>Transferring Delta</span>
              {:else if isComplete}
                <CheckCircle2 class="w-4 h-4 text-emerald-400" />
                <span class="text-emerald-400">Complete</span>
              {:else if isPaused}
                <span class="text-amber-400">Paused</span>
              {:else}
                <span>{currentStats.status}</span>
              {/if}
            </span>

            <!-- Transfer Speed in MB/s -->
            <span class="font-mono font-bold text-xs text-indigo-400 flex items-center gap-1">
              <Zap class="w-3 h-3 text-indigo-400" />
              <span>{currentStats.speedFormatted || "0 B/s"}</span>
            </span>
          </div>

          <!-- Progress Bar -->
          <div class="w-full bg-slate-800 h-2.5 rounded-full overflow-hidden p-0.5">
            <div
              class="h-full rounded-full transition-all duration-300 {isComplete ? 'bg-emerald-500' : 'bg-gradient-to-r from-indigo-500 via-purple-500 to-pink-500'}"
              style="width: {currentStats.percent || 0}%"
            ></div>
          </div>

          <div class="flex items-center justify-between text-[11px] font-mono text-slate-400">
            <span>{Math.round(currentStats.percent || 0)}%</span>
            {#if currentStats.totalBytes > 0}
              <span>{formatBytes(currentStats.transferredBytes || 0)} / {formatBytes(currentStats.totalBytes)}</span>
            {/if}
          </div>

          <!-- Message & Current File -->
          <div class="text-[11px] text-slate-300 font-mono truncate bg-slate-900 px-3 py-1.5 rounded-lg border border-slate-800/80">
            {currentStats.message || "Waiting for friend to connect..."}
          </div>

          {#if currentStats.currentFile && !isComplete}
            <div class="text-[10px] text-slate-400 font-mono truncate">
              File: <span class="text-purple-300">{currentStats.currentFile}</span>
            </div>
          {/if}
        </div>
      {/if}

      <!-- Errors -->
      {#if localError}
        <div class="p-3 rounded-xl bg-rose-500/10 border border-rose-500/30 text-rose-300 text-xs flex items-center gap-2">
          <AlertCircle class="w-4 h-4 flex-shrink-0" />
          <span>{localError}</span>
        </div>
      {/if}

      <!-- Control Buttons: Start, Pause, Resume, Stop, Close -->
      <div class="mt-auto flex flex-col gap-2">
        {#if !isSessionStarted}
          <button
            type="button"
            onclick={handleStartSender}
            disabled={!instanceFolder || selectedPaths.length === 0}
            class="w-full py-3 rounded-xl bg-gradient-to-r from-purple-600 to-indigo-600 hover:from-purple-500 hover:to-indigo-500 disabled:opacity-50 text-white font-semibold text-sm shadow-lg shadow-purple-500/25 transition-all flex items-center justify-center gap-2 cursor-pointer"
          >
            <Play class="w-4 h-4" />
            <span>Start Session & Generate Code</span>
          </button>
        {:else if !isComplete && !isError}
          <div class="grid grid-cols-2 gap-2">
            <!-- Pause / Resume -->
            <button
              type="button"
              onclick={handlePauseResume}
              class="py-2.5 rounded-xl bg-slate-800 hover:bg-slate-700 text-slate-200 text-xs font-semibold border border-slate-700 flex items-center justify-center gap-1.5 transition-colors cursor-pointer"
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
              class="py-2.5 rounded-xl bg-rose-950/40 hover:bg-rose-900/60 text-rose-300 text-xs font-semibold border border-rose-500/30 flex items-center justify-center gap-1.5 transition-colors cursor-pointer"
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
            class="w-full py-3 rounded-xl {isComplete ? 'bg-emerald-600 hover:bg-emerald-500' : 'bg-slate-800 hover:bg-slate-700'} text-white font-semibold text-sm transition-colors flex items-center justify-center gap-2 cursor-pointer shadow-lg"
          >
            <X class="w-4 h-4" />
            <span>Close</span>
          </button>
        {/if}
      </div>
    </div>
  </div>
</div>
