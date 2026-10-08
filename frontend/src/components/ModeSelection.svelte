<script lang="ts">
  import { Download, Upload, ArrowRight } from "@lucide/svelte";

  interface Props {
    onSelectMode: (mode: "receive" | "send") => void;
  }

  let { onSelectMode }: Props = $props();
</script>

<div class="max-w-2xl mx-auto px-5 py-10 flex flex-col justify-center min-h-[calc(100vh-65px)]">
  <!-- Clean Header -->
  <div class="mb-8 text-center">
    <h2 class="text-2xl font-bold tracking-tight text-white mb-2">
      Wormhole Sync
    </h2>
    <p class="text-sm text-neutral-400 max-w-md mx-auto">
      Easily sync Minecraft modpacks, configs, or any folders directly with your friend.
    </p>
  </div>

  <!-- Two Simple Options -->
  <div class="grid grid-cols-1 sm:grid-cols-2 gap-4">
    <!-- Receive Files Card -->
    <button
      type="button"
      onclick={() => onSelectMode("receive")}
      class="bg-[#15161b] hover:bg-[#1b1c23] border border-[#262731] hover:border-[#383a48] transition-all p-5 rounded-[4px] text-left cursor-pointer flex flex-col justify-between group active:translate-y-[1px]"
    >
      <div>
        <div class="w-10 h-10 rounded-[3px] bg-[#1d1f27] border border-[#2e303d] flex items-center justify-center text-sky-400 mb-4">
          <Download class="w-5 h-5" />
        </div>

        <h3 class="text-base font-semibold text-white mb-1.5 group-hover:text-sky-300 transition-colors">
          Receive files
        </h3>
        <p class="text-xs text-neutral-400 leading-relaxed">
          Choose where to save the files, put your friend's code in, and download the updates.
        </p>
      </div>

      <div class="mt-6 pt-3 border-t border-[#22232c] flex items-center justify-between text-xs text-sky-400 font-medium">
        <span>Get started</span>
        <ArrowRight class="w-3.5 h-3.5 group-hover:translate-x-1 transition-transform" />
      </div>
    </button>

    <!-- Send Files Card -->
    <button
      type="button"
      onclick={() => onSelectMode("send")}
      class="bg-[#15161b] hover:bg-[#1b1c23] border border-[#262731] hover:border-[#383a48] transition-all p-5 rounded-[4px] text-left cursor-pointer flex flex-col justify-between group active:translate-y-[1px]"
    >
      <div>
        <div class="w-10 h-10 rounded-[3px] bg-[#1d1f27] border border-[#2e303d] flex items-center justify-center text-amber-500 mb-4">
          <Upload class="w-5 h-5" />
        </div>

        <h3 class="text-base font-semibold text-white mb-1.5 group-hover:text-amber-400 transition-colors">
          Send files
        </h3>
        <p class="text-xs text-neutral-400 leading-relaxed">
          Pick your folder, choose which files to sync, and get a code to share with your friend.
        </p>
      </div>

      <div class="mt-6 pt-3 border-t border-[#22232c] flex items-center justify-between text-xs text-amber-400 font-medium">
        <span>Get started</span>
        <ArrowRight class="w-3.5 h-3.5 group-hover:translate-x-1 transition-transform" />
      </div>
    </button>
  </div>
</div>
