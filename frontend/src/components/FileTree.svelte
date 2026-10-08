<script lang="ts">
  import { Folder, File, ChevronRight, ChevronDown, Check, Search, Filter } from "@lucide/svelte";
  import { formatBytes } from "../lib/format";
  import type { models } from "../../wailsjs/go/models";

  interface Props {
    rootNode: models.FileTreeNode;
    onSelectionChange?: (selectedPaths: string[]) => void;
  }

  let { rootNode, onSelectionChange }: Props = $props();

  let searchQuery = $state("");
  let expandedFolders = $state<Record<string, boolean>>({ "": true });

  // Map of path to checked state
  let checkedMap = $state<Record<string, boolean>>({});

  // Initialize checkedMap from rootNode
  function initChecked(node: models.FileTreeNode) {
    checkedMap[node.relPath] = node.checked;
    if (node.children) {
      for (const child of node.children) {
        initChecked(child);
      }
    }
  }

  $effect(() => {
    if (rootNode) {
      initChecked(rootNode);
      // Auto-expand top level folders
      if (rootNode.children) {
        for (const child of rootNode.children) {
          if (child.isDir) {
            expandedFolders[child.relPath] = true;
          }
        }
      }
      notifySelection();
    }
  });

  function toggleExpand(relPath: string) {
    expandedFolders[relPath] = !expandedFolders[relPath];
  }

  // Toggle node and all its descendants
  function toggleCheck(node: models.FileTreeNode) {
    const nextState = !checkedMap[node.relPath];
    setNodeCheckRecursive(node, nextState);
    notifySelection();
  }

  function setNodeCheckRecursive(node: models.FileTreeNode, state: boolean) {
    checkedMap[node.relPath] = state;
    if (node.children) {
      for (const child of node.children) {
        setNodeCheckRecursive(child, state);
      }
    }
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
      if (node.children) {
        for (const child of node.children) {
          applyRecursive(child);
        }
      }
    }

    if (rootNode) {
      applyRecursive(rootNode);
      notifySelection();
    }
  }

  function selectAll(state: boolean) {
    if (rootNode) {
      setNodeCheckRecursive(rootNode, state);
      notifySelection();
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

  function notifySelection() {
    if (onSelectionChange) {
      onSelectionChange(getSelectedFilePaths());
    }
  }

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

<div class="flex flex-col h-full bg-slate-950/60 rounded-xl border border-slate-800 overflow-hidden">
  <!-- Toolbar -->
  <div class="p-3 bg-slate-900/80 border-b border-slate-800 flex flex-wrap items-center justify-between gap-2">
    <!-- Presets -->
    <div class="flex items-center gap-1.5">
      <button
        type="button"
        onclick={applyMinecraftPreset}
        class="px-2.5 py-1 rounded-lg bg-indigo-500/10 hover:bg-indigo-500/20 border border-indigo-500/30 text-indigo-300 text-xs font-semibold transition-colors cursor-pointer flex items-center gap-1"
      >
        <Filter class="w-3 h-3" />
        <span>Modpack Preset</span>
      </button>
      <button
        type="button"
        onclick={() => selectAll(true)}
        class="px-2 py-1 rounded-lg bg-slate-800 hover:bg-slate-700 text-slate-300 text-xs transition-colors cursor-pointer"
      >
        All
      </button>
      <button
        type="button"
        onclick={() => selectAll(false)}
        class="px-2 py-1 rounded-lg bg-slate-800 hover:bg-slate-700 text-slate-300 text-xs transition-colors cursor-pointer"
      >
        None
      </button>
    </div>

    <!-- Search box -->
    <div class="relative w-44 md:w-56">
      <Search class="w-3.5 h-3.5 text-slate-500 absolute left-2.5 top-1/2 -translate-y-1/2" />
      <input
        type="text"
        bind:value={searchQuery}
        placeholder="Filter mods, configs..."
        class="w-full bg-slate-900 text-slate-200 text-xs rounded-lg pl-8 pr-2.5 py-1.5 border border-slate-800 focus:outline-none focus:border-indigo-500/50"
      />
    </div>
  </div>

  <!-- Tree List View -->
  <div class="flex-1 overflow-y-auto p-2 font-mono text-xs max-h-[360px] divide-y divide-slate-900/50">
    {#if rootNode && rootNode.children}
      {#each rootNode.children as child (child.relPath)}
        {#if nodeMatchesSearch(child, searchQuery)}
          {@render TreeNode(child, 0)}
        {/if}
      {/each}
    {:else}
      <div class="p-6 text-center text-slate-500">No files found in directory</div>
    {/if}
  </div>

  <!-- Footer Stats Bar -->
  <div class="p-3 bg-slate-900/90 border-t border-slate-800 flex items-center justify-between text-xs">
    <div class="text-slate-400">
      Selected: <strong class="text-indigo-400">{selectionStats.count} files</strong>
    </div>
    <div class="text-slate-400 font-mono">
      Total Size: <strong class="text-purple-400">{formatBytes(selectionStats.bytes)}</strong>
    </div>
  </div>
</div>

{#snippet TreeNode(node: models.FileTreeNode, depth: number)}
  <div class="group select-none">
    <div
      class="flex items-center gap-2 py-1 px-1.5 rounded hover:bg-slate-800/60 transition-colors"
      style="padding-left: {depth * 18 + 6}px;"
    >
      <!-- Expand / Collapse chevron for folders -->
      {#if node.isDir}
        <button
          type="button"
          onclick={() => toggleExpand(node.relPath)}
          class="w-4 h-4 flex items-center justify-center text-slate-400 hover:text-white cursor-pointer"
        >
          {#if expandedFolders[node.relPath]}
            <ChevronDown class="w-3.5 h-3.5" />
          {:else}
            <ChevronRight class="w-3.5 h-3.5" />
          {/if}
        </button>
      {:else}
        <span class="w-4"></span>
      {/if}

      <!-- Custom Checkbox -->
      <button
        type="button"
        onclick={() => toggleCheck(node)}
        class="w-4 h-4 rounded border flex items-center justify-center cursor-pointer transition-all {checkedMap[node.relPath] ? 'bg-indigo-600 border-indigo-500 text-white' : 'border-slate-700 bg-slate-900 hover:border-slate-500'}"
      >
        {#if checkedMap[node.relPath]}
          <Check class="w-3 h-3 stroke-[3]" />
        {/if}
      </button>

      <!-- Icon & Name -->
      <button
        type="button"
        class="flex items-center gap-1.5 flex-1 min-w-0 text-left bg-transparent border-0 p-0 cursor-pointer"
        onclick={() => node.isDir && toggleExpand(node.relPath)}
      >
        {#if node.isDir}
          <Folder class="w-3.5 h-3.5 text-amber-400 flex-shrink-0" />
          <span class="font-semibold text-slate-200 truncate">{node.name}</span>
        {:else}
          <File class="w-3.5 h-3.5 text-slate-400 flex-shrink-0" />
          <span class="text-slate-300 truncate">{node.name}</span>
        {/if}
      </button>

      <!-- Size -->
      {#if node.size > 0}
        <span class="text-[11px] text-slate-500 font-mono ml-2 group-hover:text-slate-400">
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
