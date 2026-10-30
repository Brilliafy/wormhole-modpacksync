<script lang="ts">
  import { onMount } from "svelte";
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
    Trash2,
    History,
    Sparkles,
    ShieldCheck
  } from "@lucide/svelte";
  import FileTree from "./FileTree.svelte";
  import TransferProgress from "./TransferProgress.svelte";
  import CompletionCard from "./CompletionCard.svelte";
  import {
    SelectFolder,
    ScanFolder,
    StartSender,
    Pause as WailsPause,
    Resume as WailsResume,
    Stop as WailsStop
  } from "../../wailsjs/go/main/App";
  import type { models } from "../../wailsjs/go/models";
  import { playCompletionSound, playErrorSound } from "../lib/sound";
  import { getRecentFolders, addRecentFolder } from "../lib/storage";

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
  let recentFolders = $state<string[]>([]);

  let roomCode = $state("");
  let directAddresses = $state<string[]>([]);
  let relayScope = $state("");
  let relayAddr = $state("");
  let copied = $state(false);
  let localError = $state("");
  let isSessionStarted = $state(false);
  let isPaused = $state(false);
  let soundPlayed = $state(false);

  onMount(() => {
    recentFolders = getRecentFolders("send");
    if (recentFolders.length > 0 && !instanceFolder) {
      handleSelectRecent(recentFolders[0]);
    }
  });

  let isComplete = $derived(currentStats.status === "finished" && currentStats.role === "sender");
  let isError = $derived(currentStats.status === "error" && currentStats.role === "sender");
  let isSyncing = $derived(currentStats.status === "syncing" && currentStats.role === "sender");

  let currentStep = $derived.by(() => {
    if (isComplete) return 3;
    if (isSyncing || currentStats.status === "analyzing") return 3;
    if (isSessionStarted) return 2;
    return 1;
  });

  $effect(() => {
    if (isComplete && !soundPlayed) {
      soundPlayed = true;
      playCompletionSound();
      if (instanceFolder) {
        addRecentFolder("send", instanceFolder);
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
        addRecentFolder("send", selected);
        recentFolders = getRecentFolders("send");
        await loadFileTree(selected);
      }
    } catch (err: unknown) {
      localError = err instanceof Error ? err.message : String(err);
    }
  }

  async function handleSelectRecent(folder: string) {
    if (isSessionStarted) return;
    instanceFolder = folder;
    localError = "";
    await loadFileTree(folder);
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
        relayScope = result.relayScope || "";
        relayAddr = result.relayAddr || "";
        addRecentFolder("send", instanceFolder);
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
    if (!textToCopy) return;
    navigator.clipboard.writeText(textToCopy);
    copied = true;
    setTimeout(() => {
      copied = false;
    }, 2000);
  }
</script>

<div class="max-w-5xl mx-auto px-5 py-5 flex flex-col min-h-[calc(100vh-65px)]">
  <!-- Top Navigation & Stepper -->
  <div class="flex items-center justify-between mb-4 pb-2 border-b border-[#23242e]">
    <div class="flex items-center gap-2">
      <button
        type="button"
        onclick={onClose}
        disabled={isSyncing}
        class="flex items-center gap-1.5 text-xs text-neutral-400 hover:text-white transition-colors cursor-pointer disabled:opacity-40 disabled:cursor-not-allowed mr-2"
      >
        <ArrowLeft class="w-3.5 h-3.5" />
        <span>Back</span>
      </button>

      <span class="text-xs text-neutral-400 font-medium">Send Modpack Files</span>
    </div>

    <!-- 3-Phase Stepper Pills -->
    <div class="flex items-center gap-1.5 text-[11px] font-mono">
      <span class="px-2 py-0.5 rounded-[2px] {currentStep >= 1 ? 'bg-[#22242d] text-amber-400 font-semibold border border-[#353746]' : 'text-neutral-500'}">
        1. Select Files
      </span>
      <span class="text-neutral-600">→</span>
      <span class="px-2 py-0.5 rounded-[2px] {currentStep >= 2 ? 'bg-[#22242d] text-amber-400 font-semibold border border-[#353746]' : 'text-neutral-500'}">
        2. Share Code
      </span>
      <span class="text-neutral-600">→</span>
      <span class="px-2 py-0.5 rounded-[2px] {currentStep >= 3 ? 'bg-[#22242d] text-emerald-400 font-semibold border border-[#1e462e]' : 'text-neutral-500'}">
        3. Sync
      </span>
    </div>
  </div>

  {#if isComplete}
    <!-- Completion View -->
    <div class="max-w-md mx-auto w-full my-auto">
      <CompletionCard
        role="sender"
        stats={currentStats}
        targetFolder={instanceFolder}
        onDone={onClose}
      />
    </div>
  {:else}
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

          <!-- Recent Folders Quick Selection -->
          {#if recentFolders.length > 0 && !isSessionStarted}
            <div class="mt-2 flex flex-wrap items-center gap-1.5 text-[11px] text-neutral-400">
              <span class="flex items-center gap-1 text-neutral-500">
                <History class="w-3 h-3" />
                <span>Recent:</span>
              </span>
              {#each recentFolders as folder}
                <button
                  type="button"
                  onclick={() => handleSelectRecent(folder)}
                  class="px-2 py-0.5 rounded-[2px] bg-[#1a1b22] hover:bg-[#242631] border border-[#2d2f3d] text-neutral-300 hover:text-white transition-colors cursor-pointer truncate max-w-[180px]"
                  title={folder}
                >
                  {folder.split(/[/\\]/).pop() || folder}
                </button>
              {/each}
            </div>
          {/if}
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

      <!-- Right Column: Code & Telemetry -->
      <div class="lg:col-span-5 flex flex-col gap-4">
        <!-- Hero Code Display Card -->
        <div class="bg-[#15161b] p-4 rounded-[4px] border border-[#262731] shadow-sm flex flex-col gap-3">
          <div>
            <h3 class="text-sm font-semibold text-white">Share Code</h3>
            <p class="text-xs text-neutral-400 mt-0.5">
              Your friend pastes this code in Wormhole Sync to connect.
            </p>
          </div>

          {#if !isSessionStarted}
            <div class="p-4 rounded-[4px] bg-[#111216] border border-[#262731] text-center flex flex-col items-center justify-center gap-2 text-neutral-400">
              <div class="w-8 h-8 rounded-full bg-[#1c1d25] flex items-center justify-center text-amber-500">
                <KeyRound class="w-4 h-4" />
              </div>
              <p class="text-xs text-neutral-400 max-w-[220px]">
                Click <strong class="text-amber-400">Start Sync</strong> below to generate your share code and wait for friend.
              </p>
            </div>
          {:else}
            <!-- Active Code Display with 1-click copy -->
            <button
              type="button"
              onclick={copyCode}
              class="w-full text-left p-4 rounded-[4px] bg-[#111216] hover:bg-[#161820] border border-[#2e303d] hover:border-amber-500/50 flex flex-col gap-2 transition-all cursor-pointer group relative"
              title="Click anywhere to copy code"
            >
              <div class="flex items-center justify-between">
                <span class="text-[11px] text-neutral-400 uppercase tracking-wider font-semibold">Share Code</span>
                <span class="text-xs flex items-center gap-1 {copied ? 'text-emerald-400' : 'text-neutral-400 group-hover:text-amber-400'}">
                  {#if copied}
                    <Check class="w-3.5 h-3.5 text-emerald-400" />
                    <span class="font-medium">Copied!</span>
                  {:else}
                    <Copy class="w-3.5 h-3.5" />
                    <span>Click to copy</span>
                  {/if}
                </span>
              </div>

              <div class="font-mono text-2xl font-bold tracking-widest text-white py-1">
                {roomCode || "Generating..."}
              </div>

              {#if networkMode === "direct" && directAddresses.length > 0}
                <div class="text-[11px] text-neutral-400 font-mono">
                  Direct IP: <span class="text-neutral-200 select-all font-semibold">{directAddresses[0]}</span>
                </div>
              {/if}
            </button>

            <!-- Network details note -->
            <div class="text-[11px] text-neutral-500 flex items-start gap-1.5">
              <ShieldCheck class="w-3.5 h-3.5 text-emerald-500/80 flex-shrink-0 mt-0.5" />
              <span>Works worldwide. Direct P2P with encrypted transit bridge fallback.</span>
            </div>
          {/if}
        </div>

        <!-- Telemetry Card when session is active -->
        {#if isSessionStarted}
          <TransferProgress
            role="sender"
            stats={currentStats}
            onPause={handlePauseResume}
            onResume={handlePauseResume}
            onStop={handleStop}
            isPaused={isPaused}
          />
        {/if}

        <!-- Error display -->
        {#if localError}
          <div class="p-2.5 rounded-[3px] bg-[#291314] border border-[#521c1f] text-rose-300 text-xs flex items-center gap-2">
            <AlertCircle class="w-4 h-4 flex-shrink-0 text-rose-400" />
            <span>{localError}</span>
          </div>
        {/if}

        <!-- Start Button when not yet running -->
        {#if !isSessionStarted}
          <button
            type="button"
            onclick={handleStartSender}
            disabled={!instanceFolder || selectedPaths.length === 0}
            class="w-full py-3 rounded-[4px] bg-amber-600 hover:bg-amber-500 disabled:opacity-40 disabled:cursor-not-allowed text-black font-semibold text-xs border border-amber-500 transition-colors flex items-center justify-center gap-2 cursor-pointer active:translate-y-[1px] shadow-sm mt-auto"
          >
            <Play class="w-4 h-4 fill-black" />
            <span>Start Sync & Generate Code</span>
          </button>
        {/if}
      </div>
    </div>
  {/if}
</div>
