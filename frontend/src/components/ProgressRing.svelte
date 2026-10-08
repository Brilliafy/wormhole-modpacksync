<script lang="ts">
  import { Zap, CheckCircle2, AlertCircle } from "@lucide/svelte";

  interface Props {
    percent: number;
    speedFormatted?: string;
    status: string;
    message?: string;
    currentFile?: string;
    completedFiles?: number;
    totalFiles?: number;
    transferredBytes?: number;
    totalBytes?: number;
    size?: number;
    strokeWidth?: number;
  }

  let {
    percent = 0,
    speedFormatted = "0 B/s",
    status = "idle",
    message = "",
    currentFile = "",
    completedFiles = 0,
    totalFiles = 0,
    transferredBytes = 0,
    totalBytes = 0,
    size = 280,
    strokeWidth = 14
  }: Props = $props();

  let radius = $derived((size - strokeWidth) / 2);
  let circumference = $derived(2 * Math.PI * radius);
  let clampedPercent = $derived(Math.min(100, Math.max(0, percent)));
  let strokeDashoffset = $derived(circumference - (clampedPercent / 100) * circumference);

  let isComplete = $derived(status === "finished");
  let isError = $derived(status === "error");
  let isAnalyzingOrWaiting = $derived(status === "waiting" || status === "connecting" || status === "analyzing");
</script>

<div class="relative flex flex-col items-center justify-center select-none" style="width: {size}px; height: {size}px;">
  <!-- SVG Circular Progress Ring -->
  <svg
    class="w-full h-full -rotate-90 transform"
    viewBox="0 0 {size} {size}"
  >
    <defs>
      <!-- Gradient for progress ring -->
      <linearGradient id="ringGradient" x1="0%" y1="0%" x2="100%" y2="100%">
        <stop offset="0%" stop-color="#6366f1" />
        <stop offset="50%" stop-color="#a855f7" />
        <stop offset="100%" stop-color="#ec4899" />
      </linearGradient>

      <!-- Complete gradient -->
      <linearGradient id="completeGradient" x1="0%" y1="0%" x2="100%" y2="100%">
        <stop offset="0%" stop-color="#10b981" />
        <stop offset="100%" stop-color="#059669" />
      </linearGradient>

      <!-- Error gradient -->
      <linearGradient id="errorGradient" x1="0%" y1="0%" x2="100%" y2="100%">
        <stop offset="0%" stop-color="#ef4444" />
        <stop offset="100%" stop-color="#dc2626" />
      </linearGradient>

      <!-- Glow filter -->
      <filter id="glow" x="-20%" y="-20%" width="140%" height="140%">
        <feGaussianBlur stdDeviation="6" result="blur" />
        <feComposite in="SourceGraphic" in2="blur" operator="over" />
      </filter>
    </defs>

    <!-- Background Track -->
    <circle
      cx="{size / 2}"
      cy="{size / 2}"
      r="{radius}"
      stroke="rgba(30, 41, 59, 0.7)"
      stroke-width="{strokeWidth}"
      fill="none"
    />

    <!-- Animated Foreground Progress Circle -->
    <circle
      cx="{size / 2}"
      cy="{size / 2}"
      r="{radius}"
      stroke={isComplete ? "url(#completeGradient)" : isError ? "url(#errorGradient)" : "url(#ringGradient)"}
      stroke-width="{strokeWidth}"
      stroke-linecap="round"
      stroke-dasharray="{circumference}"
      stroke-dashoffset="{strokeDashoffset}"
      fill="none"
      filter="url(#glow)"
      class="transition-[stroke-dashoffset] duration-300 ease-out {isAnalyzingOrWaiting && percent === 0 ? 'animate-pulse' : ''}"
    />
  </svg>

  <!-- Centered Content Inside the Ring -->
  <div class="absolute inset-0 flex flex-col items-center justify-center p-6 text-center z-10 pointer-events-none">
    {#if isComplete}
      <div class="w-14 h-14 rounded-full bg-emerald-500/20 text-emerald-400 flex items-center justify-center mb-2 animate-bounce">
        <CheckCircle2 class="w-8 h-8" />
      </div>
      <span class="text-2xl font-black text-white">100%</span>
      <span class="text-xs font-semibold text-emerald-400 uppercase tracking-wider mt-1">Synced</span>
    {:else if isError}
      <div class="w-14 h-14 rounded-full bg-rose-500/20 text-rose-400 flex items-center justify-center mb-2">
        <AlertCircle class="w-8 h-8" />
      </div>
      <span class="text-xl font-bold text-rose-400">Failed</span>
      <span class="text-xs text-rose-300/80 mt-1">Check logs</span>
    {:else}
      <!-- Percentage -->
      <div class="text-4xl font-extrabold tracking-tight text-white font-mono">
        {Math.round(clampedPercent)}<span class="text-2xl text-purple-400">%</span>
      </div>

      <!-- Speed Badge -->
      {#if speedFormatted && speedFormatted !== "0 B/s"}
        <div class="mt-2 inline-flex items-center gap-1 px-2.5 py-0.5 rounded-full bg-indigo-500/20 border border-indigo-500/30 text-indigo-300 text-xs font-mono font-semibold">
          <Zap class="w-3 h-3 text-indigo-400" />
          <span>{speedFormatted}</span>
        </div>
      {:else}
        <div class="mt-2 text-xs font-mono text-slate-400 uppercase tracking-wide">
          {status}
        </div>
      {/if}

      <!-- Files count -->
      {#if totalFiles > 0}
        <div class="mt-2 text-[11px] font-mono text-slate-400">
          [{completedFiles} / {totalFiles} files]
        </div>
      {/if}
    {/if}
  </div>
</div>
