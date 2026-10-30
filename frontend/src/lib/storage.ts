/**
 * Persistent localStorage utility for storing recently used instance paths
 */

const RECENT_SEND_KEY = "wormhole_recent_send_folders";
const RECENT_RECEIVE_KEY = "wormhole_recent_receive_folders";
const MAX_RECENT = 3;

export function getRecentFolders(type: "send" | "receive"): string[] {
  try {
    const key = type === "send" ? RECENT_SEND_KEY : RECENT_RECEIVE_KEY;
    const raw = localStorage.getItem(key);
    if (!raw) return [];
    const list = JSON.parse(raw);
    return Array.isArray(list) ? list.filter((p) => typeof p === "string" && p.trim()) : [];
  } catch {
    return [];
  }
}

export function addRecentFolder(type: "send" | "receive", folderPath: string): void {
  if (!folderPath || !folderPath.trim()) return;
  try {
    const key = type === "send" ? RECENT_SEND_KEY : RECENT_RECEIVE_KEY;
    const current = getRecentFolders(type);
    const normalized = folderPath.trim();
    // Remove if already exists so it jumps to top
    const filtered = current.filter((p) => p !== normalized);
    filtered.unshift(normalized);
    const trimmed = filtered.slice(0, MAX_RECENT);
    localStorage.setItem(key, JSON.stringify(trimmed));
  } catch {
    // Ignore storage quota errors
  }
}
