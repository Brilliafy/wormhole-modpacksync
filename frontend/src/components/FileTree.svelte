<script lang="ts">
  import { Folder, File, ChevronRight, ChevronDown, Check, Search, Filter, CopyCheck } from "@lucide/svelte";
  import { formatBytes } from "../lib/format";
  import type { models } from "../../wailsjs/go/models";

  interface Props {
    rootNode: models.FileTreeNode;
    onSelectionChange?: (selectedPaths: string[], absoluteSyncFolders: string[]) => void;
  }

  let { rootNode, onSelectionChange }: Props = $props();

  let searchQuery = $state("");
  let expandedFolders = $state<Record<string, boolean>>({ "": true });

  // Map of path to checked state
  let checkedMap = $state<Record<string, boolean>>({});
  // Map of folder path to Absolute Sync (mirror / delete extra files) state
  let absoluteSyncMap = $state<Record<string, boolean>>({});

  // Initialize maps from rootNode
  function initMaps(node: models.FileTreeNode) {
    checkedMap[node.relPath] = node.checked;
    if (node.isDir && node.relPath !== "") {
      absoluteSyncMap[node.relPath] = node.absoluteSync;
    }
    if (node.children) {
      for (const child of node.children) {
        initMaps(child);
      }
    }
  }

  // Initialise maps ONLY when a new tree arrives. This effect must never read
  // checkedMap or absoluteSyncMap: if it did, every user toggle would re-run
  // it and wipe the selection back to the tree defaults.
  $effect(() => {
    if (rootNode) {
      initMaps(rootNode);
      // Auto-expand top level folders
      if (rootNode.children) {
        for (const child of rootNode.children) {
          if (child.isDir) {
            expandedFolders[child.relPath] = true;
          }
        }
      }
    }
  });

  function toggleExpand(relPath: string) {
    expandedFolders[relPath] = !expandedFolders[relPath];
  }

  // Toggle node and all its descendants
  function toggleCheck(node: models.FileTreeNode) {
    const nextState = !checkedMap[node.relPath];
    setNodeCheckRecursive(node, nextState);
  }

  function setNodeCheckRecursive(node: models.FileTreeNode, state: boolean) {
    checkedMap[node.relPath] = state;
    if (node.children) {
      for (const child of node.children) {
        setNodeCheckRecursive(child, state);
      }
    }
  }

  function toggleAbsoluteSync(relPath: string) {
    absoluteSyncMap[relPath] = !absoluteSyncMap[relPath];
  }

  function applyMinecraftPreset() {
    const excludes = ["saves", "screenshots", "logs", "crash-reports", "options.txt", "servers.dat", "usercache.json"];
    
    function applyRecursive(node: models.FileTreeNode) {
      const lower = node.name.toLowerCase();
      let shouldCheck = true;
      for (const exc of excludes) {
        if (lower === exc || node.relPath.toLowerCase().startsWith(exc + "/")) {
          shouldCheck = false;
          break;
        }
      }
      checkedMap[node.relPath] = shouldCheck;

      // Automatically enable Absolute Sync on mods and config!
      if (node.isDir && node.relPath !== "") {
        if (lower === "mods" || lower === "config" || lower === "defaultconfigs") {
          absoluteSyncMap[node.relPath] = true;
        }
      }

      if (node.children) {
        for (const child of node.children) {
          applyRecursive(child);
        }
      }
    }

    if (rootNode) {
      applyRecursive(rootNode);
    }
  }

  function selectAll(state: boolean) {
    if (rootNode) {
      setNodeCheckRecursive(rootNode, state);
    }
  }

  function getSelectedFilePaths(): string[] {
    const list: string[] = [];
    function collect(node: models.FileTreeNode) {
      if (!node.isDir && checkedMap[node.relPath]) {
        list.push(node.relPath);
      }
      if (node.children) {
        for (const child of node.children) {
          collect(child);
        }
      }
    }
    if (rootNode) {
      collect(rootNode);
    }
    return list;
  }

  function getAbsoluteSyncFolders(): string[] {
    const list: string[] = [];
    for (const [folderPath, enabled] of Object.entries(absoluteSyncMap)) {
      if (enabled && folderPath !== "") {
        list.push(folderPath);
      }
    }
    return list;
  }

  // Notify the parent whenever the selection changes. Only this derived value
  // is read by the effect below, so checkbox toggles can never re-trigger the
  // tree initialisation effect above (which was resetting every selection).
  let selection = $derived.by(() => ({
    files: getSelectedFilePaths(),
    folders: getAbsoluteSyncFolders(),
  }));

  $effect(() => {
    onSelectionChange?.(selection.files, selection.folders);
  });

  // Calculate selected files and size stats
  let selectionStats = $derived.by(() => {
    let count = 0;
    let bytes = 0;
    function countStats(node: models.FileTreeNode) {
      if (!node.isDir && checkedMap[node.relPath]) {
        count++;
        bytes += node.size;
      }
      if (node.children) {
        for (const child of node.children) {
          countStats(child);
        }
      }
    }
    if (rootNode) {
      countStats(rootNode);
    }
    return { count, bytes };
  });

  let activeMirroredFolders = $derived.by(() => {
    return Object.entries(absoluteSyncMap)
      .filter(([path, enabled]) => enabled && path !== "")
      .map(([path]) => path);
  });

  // Filter helper
  function nodeMatchesSearch(node: models.FileTreeNode, query: string): boolean {
    if (!query) return true;
    const q = query.toLowerCase();
    if (node.name.toLowerCase().includes(q)) return true;
    if (node.children) {
      return node.children.some(c => nodeMatchesSearch(c, query));
    }
    return false;
  }
</script>

<div class="flex flex-col h-full bg-[#121317] rounded-[3px] border border-[#262731] overflow-hidden">
  <!-- Toolbar -->
  <div class="p-2.5 bg-[#17181f] border-b border-[#262731] flex flex-wrap items-center justify-between gap-2">
    <!-- Presets & Bulk Controls -->
    <div class="flex items-center gap-1.5">
      <button
        type="button"
        onclick={applyMinecraftPreset}
        class="px-2.5 py-1 rounded-[3px] bg-[#22232c] hover:bg-[#2b2d38] active:translate-y-[1px] border border-[#353746] text-amber-400 text-xs font-medium transition-colors cursor-pointer flex items-center gap-1.5"
        title="Automatically checks mods, config, scripts while deselecting saves, logs, and options"
      >
        <Filter class="w-3 h-3 text-amber-500" />
        <span>Modpack Preset</span>
      </button>
      <button
        type="button"
        onclick={() => selectAll(true)}
        class="px-2 py-1 rounded-[3px] bg-[#1a1b22] hover:bg-[#23252f] active:translate-y-[1px] border border-[#2b2d38] text-neutral-300 text-xs transition-colors cursor-pointer"
      >
        All
      </button>
      <button
        type="button"
        onclick={() => selectAll(false)}
        class="px-2 py-1 rounded-[3px] bg-[#1a1b22] hover:bg-[#23252f] active:translate-y-[1px] border border-[#2b2d38] text-neutral-300 text-xs transition-colors cursor-pointer"
      >
        None
      </button>
    </div>

    <!-- Search box -->
    <div class="relative w-44 md:w-52">
      <Search class="w-3.5 h-3.5 text-neutral-500 absolute left-2.5 top-1/2 -translate-y-1/2" />
      <input
        type="text"
        bind:value={searchQuery}
        placeholder="Filter path..."
        class="w-full bg-[#111216] text-neutral-200 text-xs rounded-[3px] pl-7 pr-2.5 py-1 border border-[#2a2c37] focus:outline-none focus:border-amber-500 font-mono"
      />
    </div>
  </div>

  <!-- Mirror Notice Banner -->
  {#if activeMirroredFolders.length > 0}
    <div class="px-3 py-1 bg-[#1c1810] border-b border-[#3d3119] flex items-center justify-between text-xs text-amber-300/90">
      <span class="flex items-center gap-1.5">
        <CopyCheck class="w-3.5 h-3.5 text-amber-400 flex-shrink-0" />
        <span>Mirroring enabled for: <strong class="text-amber-200">{activeMirroredFolders.join(", ")}</strong> (extra files in these folders will be deleted)</span>
      </span>
    </div>
  {/if}

  <!-- Tree List View -->
  <div class="flex-1 overflow-y-auto p-1.5 font-mono text-xs max-h-[360px] divide-y divide-[#181920]">
    {#if rootNode && rootNode.children}
      {#each rootNode.children as child (child.relPath)}
        {#if nodeMatchesSearch(child, searchQuery)}
          {@render TreeNode(child, 0)}
        {/if}
      {/each}
    {:else}
      <div class="p-6 text-center text-neutral-500 font-sans text-xs">No files found in directory</div>
    {/if}
  </div>

  <!-- Footer Stats Bar -->
  <div class="px-3 py-1.5 bg-[#17181f] border-t border-[#262731] flex items-center justify-between text-xs text-neutral-400">
    <div>
      <strong class="text-amber-400 font-medium">{selectionStats.count}</strong> files selected
    </div>
    <div class="font-mono tabular-nums text-neutral-300">
      {formatBytes(selectionStats.bytes)}
    </div>
  </div>
</div>

{#snippet TreeNode(node: models.FileTreeNode, depth: number)}
  <div class="group select-none">
    <div
      class="flex items-center gap-2 py-0.5 px-1 rounded-[2px] hover:bg-[#1a1b22] transition-colors h-7"
      style="padding-left: {depth * 16 + 4}px;"
    >
      <!-- Expand / Collapse chevron for folders -->
      {#if node.isDir}
        <button
          type="button"
          onclick={(e) => { e.stopPropagation(); toggleExpand(node.relPath); }}
          title={expandedFolders[node.relPath] ? "Collapse" : "Expand"}
          class="w-5 h-5 flex-shrink-0 flex items-center justify-center text-neutral-400 hover:text-white hover:bg-[#22242d] rounded-[3px] cursor-pointer transition-colors"
        >
          {#if expandedFolders[node.relPath]}
            <ChevronDown class="w-3 h-3" />
          {:else}
            <ChevronRight class="w-3 h-3" />
          {/if}
        </button>
      {:else}
        <span class="w-5 flex-shrink-0"></span>
      {/if}

      <!-- Explicit checkbox: single click only toggles the check state -->
      <button
        type="button"
        onclick={(e) => { e.stopPropagation(); toggleCheck(node); }}
        title={checkedMap[node.relPath] ? "Uncheck" : "Check"}
        aria-label={checkedMap[node.relPath] ? "Uncheck" : "Check"}
        class="w-5 h-5 flex-shrink-0 flex items-center justify-center rounded-[3px] cursor-pointer hover:bg-[#22242d] transition-colors"
      >
        <span class="w-3.5 h-3.5 rounded-[2px] border flex items-center justify-center transition-colors {checkedMap[node.relPath] ? 'bg-amber-600 border-amber-500 text-black' : 'border-[#383a48] bg-[#101115]'}">
          {#if checkedMap[node.relPath]}
            <Check class="w-3 h-3 stroke-[3]" />
          {/if}
        </span>
      </button>

      <!-- Icon & Name: double-click a folder to expand/collapse -->
      <button
        type="button"
        class="flex items-center gap-1.5 flex-1 min-w-0 text-left bg-transparent border-0 p-0 cursor-pointer"
        title={node.isDir ? "Double-click to expand or collapse" : ""}
        ondblclick={() => node.isDir && toggleExpand(node.relPath)}
      >
        {#if node.isDir}
          <Folder class="w-3.5 h-3.5 text-amber-500/90 flex-shrink-0" />
          <span class="font-medium text-neutral-200 truncate">{node.name}</span>
        {:else}
          <File class="w-3.5 h-3.5 text-neutral-400 flex-shrink-0" />
          <span class="text-neutral-300 truncate">{node.name}</span>
        {/if}
      </button>

      <!-- Mirror Toggle Badge (Only for folders with relPath != "") -->
      {#if node.isDir && node.relPath}
        <button
          type="button"
          onclick={(e) => { e.stopPropagation(); toggleAbsoluteSync(node.relPath); }}
          class="px-1.5 py-0.2 rounded-[2px] text-xs flex items-center gap-1 transition-colors cursor-pointer {absoluteSyncMap[node.relPath] ? 'bg-[#291f0c] text-amber-300 border border-[#5c4415]' : 'bg-[#181920] text-neutral-500 hover:text-neutral-300 border border-[#2b2d38]'}"
          title="Mirror this folder: deletes any extra files on your friend's side that you don't have."
        >
          <CopyCheck class="w-3 h-3 {absoluteSyncMap[node.relPath] ? 'text-amber-400' : 'text-neutral-500'}" />
          <span>{absoluteSyncMap[node.relPath] ? 'Mirror: on' : 'Mirror'}</span>
        </button>
      {/if}

      <!-- Size with tabular numbers -->
      {#if node.size > 0}
        <span class="text-[10px] text-neutral-500 font-mono ml-2 group-hover:text-neutral-400 tabular-nums">
          {formatBytes(node.size)}
        </span>
      {/if}
    </div>

    <!-- Recursive Children -->
    {#if node.isDir && expandedFolders[node.relPath] && node.children}
      <div class="children">
        {#each node.children as child (child.relPath)}
          {#if nodeMatchesSearch(child, searchQuery)}
            {@render TreeNode(child, depth + 1)}
          {/if}
        {/each}
      </div>
    {/if}
  </div>
{/snippet}
