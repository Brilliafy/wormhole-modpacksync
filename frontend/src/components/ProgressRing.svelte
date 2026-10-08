<script lang="ts">
  import { CheckCircle2, AlertCircle, Activity } from "@lucide/svelte";

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
    size = 260,
    strokeWidth = 10
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
  <!-- SVG Circular Gauge -->
  <svg
    class="w-full h-full -rotate-90 transform"
    viewBox="0 0 {size} {size}"
  >
    <!-- Background Track -->
    <circle
      cx="{size / 2}"
      cy="{size / 2}"
      r="{radius}"
      stroke="#1f2027"
      stroke-width="{strokeWidth}"
      fill="none"
    />

    <!-- Active Progress Stroke (Solid precision color, no diffuse blur) -->
    <circle
      cx="{size / 2}"
      cy="{size / 2}"
      r="{radius}"
      stroke={isComplete ? "#10b981" : isError ? "#ef4444" : "#f59e0b"}
      stroke-width="{strokeWidth}"
      stroke-linecap="butt"
      stroke-dasharray="{circumference}"
      stroke-dashoffset="{strokeDashoffset}"
      fill="none"
      class="transition-[stroke-dashoffset] duration-200 ease-out"
    />
  </svg>

  <!-- Technical Readout Inside the Gauge -->
  <div class="absolute inset-0 flex flex-col items-center justify-center p-4 text-center z-10 pointer-events-none">
    {#if isComplete}
      <div class="w-10 h-10 rounded-[3px] bg-[#102419] border border-[#1e462e] text-emerald-400 flex items-center justify-center mb-1.5">
        <CheckCircle2 class="w-6 h-6" />
      </div>
      <span class="text-2xl font-mono font-bold text-white tabular-nums">100%</span>
      <span class="text-[10px] font-mono uppercase tracking-wider text-emerald-400 mt-0.5">COMPLETED</span>
    {:else if isError}
      <div class="w-10 h-10 rounded-[3px] bg-[#2a1314] border border-[#521c1f] text-rose-400 flex items-center justify-center mb-1.5">
        <AlertCircle class="w-6 h-6" />
      </div>
      <span class="text-lg font-mono font-bold text-rose-400">FAILED</span>
      <span class="text-[10px] font-mono text-rose-400/80 mt-0.5">CHECK LOGS</span>
    {:else}
      <!-- Tabular Percentage -->
      <div class="text-3xl font-bold tracking-tight text-white font-mono tabular-nums">
        {Math.round(clampedPercent)}<span class="text-xl text-neutral-400">%</span>
      </div>

      <!-- Speed readout badge -->
      {#if speedFormatted && speedFormatted !== "0 B/s"}
        <div class="mt-2 inline-flex items-center gap-1 px-2 py-0.5 rounded-[2px] bg-[#211a0c] border border-[#483713] text-amber-400 text-xs font-mono tabular-nums font-semibold">
          <Activity class="w-3 h-3 text-amber-500" />
          <span>{speedFormatted}</span>
        </div>
      {:else}
        <div class="mt-2 text-[10px] font-mono text-neutral-400 uppercase tracking-wider">
          {status}
        </div>
      {/if}

      <!-- Files count -->
      {#if totalFiles > 0}
        <div class="mt-1.5 text-[11px] font-mono text-neutral-400 tabular-nums">
          {completedFiles} / {totalFiles} files
        </div>
      {/if}
    {/if}
  </div>
</div>
