<script lang="ts">
  import { onMount } from "svelte";
  import {
    Folder,
    KeyRound,
    Download,
    X,
    AlertCircle,
    ArrowLeft,
    CheckCircle2,
    Clipboard,
    History,
    Sparkles
  } from "@lucide/svelte";
  import TransferProgress from "./TransferProgress.svelte";
  import CompletionCard from "./CompletionCard.svelte";
  import { SelectFolder, StartReceiver, Stop as WailsStop } from "../../wailsjs/go/main/App";
  import type { models } from "../../wailsjs/go/models";
  import { playCompletionSound, playErrorSound } from "../lib/sound";
  import { getRecentFolders, addRecentFolder } from "../lib/storage";

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
  let recentFolders = $state<string[]>([]);
  let clipboardCode = $state("");

  onMount(async () => {
    recentFolders = getRecentFolders("receive");
    if (recentFolders.length > 0 && !targetFolder) {
      targetFolder = recentFolders[0];
    }

    // Check clipboard for valid room code
    try {
      if (navigator.clipboard && navigator.clipboard.readText) {
        const text = await navigator.clipboard.readText();
        if (text) {
          const trimmed = text.trim();
          if (/^[A-Za-z0-9]{4}-?[A-Za-z0-9]{4}$/.test(trimmed)) {
            let formatted = trimmed.toUpperCase();
            if (!formatted.includes("-")) {
              formatted = `${formatted.slice(0, 4)}-${formatted.slice(4)}`;
            }
            clipboardCode = formatted;
          }
        }
      }
    } catch {
      // Clipboard permission denied or unavailable
    }
  });

  let isComplete = $derived(currentStats.status === "finished" && currentStats.role === "receiver");
  let isError = $derived(currentStats.status === "error" && currentStats.role === "receiver");

  let isLocked = $derived(
    isConnectingOrSyncing ||
    (currentStats.role === "receiver" &&
      ["connecting", "waiting", "scanning", "analyzing", "syncing", "paused", "finished"].includes(currentStats.status))
  );

  let currentStep = $derived.by(() => {
    if (isComplete) return 3;
    if (currentStats.status === "syncing" || currentStats.status === "analyzing") return 3;
    if (isLocked) return 2;
    return 1;
  });

  $effect(() => {
    if (isComplete && !soundPlayed) {
      soundPlayed = true;
      playCompletionSound();
      if (targetFolder) {
        addRecentFolder("receive", targetFolder);
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
        addRecentFolder("receive", selected);
        recentFolders = getRecentFolders("receive");
      }
    } catch (err: unknown) {
      localError = err instanceof Error ? err.message : String(err);
    }
  }

  function handleSelectRecent(folder: string) {
    if (isLocked) return;
    targetFolder = folder;
    localError = "";
  }

  function handleCodeInput(e: Event) {
    const input = e.target as HTMLInputElement;
    let val = input.value.toUpperCase().replace(/[^A-Z0-9-]/g, "");

    // If typing without dash, auto-insert dash after 4 characters
    if (networkMode === "webrtc") {
      val = val.replace(/-/g, "");
      if (val.length > 4) {
        val = `${val.slice(0, 4)}-${val.slice(4, 8)}`;
      } else {
        val = val.slice(0, 4);
      }
    }

    code = val;
    input.value = val;
  }

  async function handlePasteCode() {
    try {
      const text = await navigator.clipboard.readText();
      if (text) {
        let val = text.trim().toUpperCase().replace(/[^A-Z0-9-]/g, "");
        if (networkMode === "webrtc") {
          val = val.replace(/-/g, "");
          if (val.length >= 8) {
            val = `${val.slice(0, 4)}-${val.slice(4, 8)}`;
          }
        }
        code = val;
      }
    } catch {
      // Ignore
    }
  }

  function useClipboardCode() {
    if (clipboardCode) {
      code = clipboardCode;
      clipboardCode = "";
    }
  }

  async function handleStop() {
    try {
      await WailsStop();
    } catch (e) {
      console.warn("Stop error:", e);
    }
    isConnectingOrSyncing = false;
  }

  async function handleStartReceive() {
    if (!targetFolder) {
      localError = "Please select the target folder to sync into.";
      return;
    }
    if (!code.trim()) {
      localError = "Please enter your friend's code.";
      return;
    }

    localError = "";
    isConnectingOrSyncing = true;
    soundPlayed = false;

    try {
      await StartReceiver(targetFolder, networkMode, code.trim());
      addRecentFolder("receive", targetFolder);
    } catch (err: unknown) {
      isConnectingOrSyncing = false;
      localError = err instanceof Error ? err.message : String(err);
      playErrorSound();
    }
  }

  function handleKeyDown(e: KeyboardEvent) {
    if (e.key === "Enter" && !isLocked && targetFolder && code.trim()) {
      handleStartReceive();
    }
  }
</script>

<div class="max-w-lg mx-auto px-5 py-6 flex flex-col items-center justify-center min-h-[calc(100vh-65px)]">
  <!-- Stepper Breadcrumb -->
  <div class="w-full flex items-center justify-between mb-5 px-1">
    <div class="flex items-center gap-2">
      <button
        type="button"
        onclick={onClose}
        disabled={isLocked && !isComplete && !isError}
        class="flex items-center gap-1.5 text-xs text-neutral-400 hover:text-white transition-colors cursor-pointer disabled:opacity-40 disabled:cursor-not-allowed mr-2"
      >
        <ArrowLeft class="w-3.5 h-3.5" />
        <span>Back</span>
      </button>

      <span class="text-xs text-neutral-400 font-medium">Receive Files</span>
    </div>

    <!-- 3-Phase Stepper Pills -->
    <div class="flex items-center gap-1.5 text-[11px] font-mono">
      <span class="px-2 py-0.5 rounded-[2px] {currentStep >= 1 ? 'bg-[#22242d] text-sky-400 font-semibold border border-[#353746]' : 'text-neutral-500'}">
        1. Setup
      </span>
      <span class="text-neutral-600">→</span>
      <span class="px-2 py-0.5 rounded-[2px] {currentStep >= 2 ? 'bg-[#22242d] text-sky-400 font-semibold border border-[#353746]' : 'text-neutral-500'}">
        2. Connect
      </span>
      <span class="text-neutral-600">→</span>
      <span class="px-2 py-0.5 rounded-[2px] {currentStep >= 3 ? 'bg-[#22242d] text-emerald-400 font-semibold border border-[#1e462e]' : 'text-neutral-500'}">
        3. Sync
      </span>
    </div>
  </div>

  {#if isComplete}
    <!-- Completion Card -->
    <CompletionCard
      role="receiver"
      stats={currentStats}
      targetFolder={targetFolder}
      onDone={onClose}
    />
  {:else if isLocked}
    <!-- Active Synchronization & Telemetry -->
    <div class="w-full flex flex-col items-center">
      <TransferProgress
        role="receiver"
        stats={currentStats}
        onStop={handleStop}
      />
    </div>
  {:else}
    <!-- Setup Form -->
    <div class="w-full bg-[#15161b] rounded-[6px] border border-[#262731] p-6 shadow-sm">
      <div class="mb-5">
        <h2 class="text-base font-semibold text-white mb-1">Receive files from friend</h2>
        <p class="text-neutral-400 text-xs leading-relaxed">
          Select your local folder and enter the 8-character code shared by your friend.
        </p>
      </div>

      <!-- Target Folder Selection -->
      <div class="mb-4">
        <label for="target-folder-input" class="block text-xs text-neutral-300 font-medium mb-1">
          Folder to sync into
        </label>
        <div class="flex gap-2">
          <input
            id="target-folder-input"
            type="text"
            readonly
            value={targetFolder}
            placeholder="Select folder..."
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

        <!-- Recent Folders Quick Selection -->
        {#if recentFolders.length > 0}
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

      <!-- Code Entry -->
      <div class="mb-5">
        <div class="flex items-center justify-between mb-1">
          <label for="room-code-input" class="text-xs text-neutral-300 font-medium">
            {networkMode === "direct" ? "Direct IP:Port" : "Share code"}
          </label>
          <button
            type="button"
            onclick={handlePasteCode}
            class="text-[11px] text-sky-400 hover:text-sky-300 flex items-center gap-1 cursor-pointer transition-colors"
          >
            <Clipboard class="w-3 h-3" />
            <span>Paste</span>
          </button>
        </div>

        <div class="relative">
          <div class="absolute inset-y-0 left-0 pl-3 flex items-center pointer-events-none">
            <KeyRound class="w-3.5 h-3.5 text-amber-500/80" />
          </div>
          <input
            id="room-code-input"
            type="text"
            value={code}
            oninput={handleCodeInput}
            onkeydown={handleKeyDown}
            placeholder={networkMode === "direct" ? "e.g. 100.115.92.3:42424" : "e.g. 1EB4-WQMJ"}
            class="w-full bg-[#111216] border border-[#292b36] text-white placeholder-neutral-600 text-sm font-mono tracking-widest rounded-[3px] pl-9 pr-3 py-2.5 focus:outline-none focus:border-amber-500 transition-colors uppercase"
          />
        </div>

        <!-- Clipboard Suggestion Chip -->
        {#if clipboardCode && !code}
          <div class="mt-2">
            <button
              type="button"
              onclick={useClipboardCode}
              class="w-full text-left px-2.5 py-1.5 rounded-[3px] bg-[#102433] hover:bg-[#153147] border border-[#1e4c6e] text-sky-300 text-xs flex items-center justify-between transition-colors cursor-pointer"
            >
              <span class="flex items-center gap-1.5">
                <Sparkles class="w-3.5 h-3.5 text-sky-400" />
                <span>Found code in clipboard: <strong class="font-mono text-white">{clipboardCode}</strong></span>
              </span>
              <span class="text-[11px] text-sky-400 font-medium">Use code →</span>
            </button>
          </div>
        {/if}
      </div>

      <!-- Backend & Local errors -->
      {#if currentStats.errorMessage}
        <div class="mb-4 p-2.5 rounded-[3px] bg-[#291314] border border-[#521c1f] text-rose-300 text-xs flex items-center gap-2">
          <AlertCircle class="w-4 h-4 flex-shrink-0 text-rose-400" />
          <span>{currentStats.errorMessage}</span>
        </div>
      {/if}

      {#if localError}
        <div class="mb-4 p-2.5 rounded-[3px] bg-[#291314] border border-[#521c1f] text-rose-300 text-xs flex items-center gap-2">
          <AlertCircle class="w-4 h-4 flex-shrink-0 text-rose-400" />
          <span>{localError}</span>
        </div>
      {/if}

      <!-- Action Button -->
      <button
        type="button"
        onclick={handleStartReceive}
        disabled={!targetFolder || !code.trim()}
        class="w-full py-2.5 rounded-[3px] bg-sky-600 hover:bg-sky-500 disabled:opacity-40 disabled:cursor-not-allowed active:translate-y-[1px] border border-sky-500 text-white font-semibold text-xs transition-colors flex items-center justify-center gap-2 cursor-pointer shadow-sm"
      >
        <Download class="w-3.5 h-3.5" />
        <span>Connect and sync</span>
      </button>
    </div>
  {/if}
</div>
