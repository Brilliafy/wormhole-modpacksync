package models

import (
	"fmt"
)

// FileInfo contains metadata and SHA-256 hash of a file
type FileInfo struct {
	RelPath string `json:"relPath"`
	Size    int64  `json:"size"`
	ModTime int64  `json:"modTime"`
	Hash    string `json:"hash"`
	IsDir   bool   `json:"isDir"`
	Checked bool   `json:"checked"`
}

// FileTreeNode represents a node in the hierarchical file tree for sender UI
type FileTreeNode struct {
	ID           string          `json:"id"`
	Name         string          `json:"name"`
	RelPath      string          `json:"relPath"`
	IsDir        bool            `json:"isDir"`
	Size         int64           `json:"size"`
	Checked      bool            `json:"checked"`
	AbsoluteSync bool            `json:"absoluteSync"` // If true on a directory, extra files on receiver will be pruned
	Children     []*FileTreeNode `json:"children,omitempty"`
}

// SyncManifest represents the collection of files selected for synchronization
type SyncManifest struct {
	InstanceName        string     `json:"instanceName"`
	CreatedAt           int64      `json:"createdAt"`
	Files               []FileInfo `json:"files"`
	TotalFiles          int        `json:"totalFiles"`
	TotalBytes          int64      `json:"totalBytes"`
	AbsoluteSyncFolders []string   `json:"absoluteSyncFolders,omitempty"` // Folders that must strictly mirror sender (delete extras)
}

// DeltaPlan represents the delta computation result on the receiver
type DeltaPlan struct {
	FilesToSync   []FileInfo `json:"filesToSync"`
	FilesUpToDate []FileInfo `json:"filesUpToDate"`
	FilesToDelete []string   `json:"filesToDelete"` // Extra files in absolute-sync folders that need deletion
	TotalFiles    int        `json:"totalFiles"`
	SyncFiles     int        `json:"syncFiles"`
	TotalBytes    int64      `json:"totalBytes"`
	SyncBytes     int64      `json:"syncBytes"`
	SavedBytes    int64      `json:"savedBytes"`
}

// SyncStats provides live telemetry to the UI for both sender and receiver
type SyncStats struct {
	Status           string   `json:"status"` // "idle", "scanning", "waiting", "connecting", "analyzing", "syncing", "paused", "finished", "stopped", "error"
	Role             string   `json:"role"`   // "sender" or "receiver"
	Mode             string   `json:"mode"`   // "direct" or "webrtc"
	TotalFiles       int      `json:"totalFiles"`
	CompletedFiles   int      `json:"completedFiles"`
	PrunedFiles      int      `json:"prunedFiles"`
	TotalBytes       int64    `json:"totalBytes"`
	TransferredBytes int64    `json:"transferredBytes"`
	SpeedBytesPerSec float64  `json:"speedBytesPerSec"`
	SpeedFormatted   string   `json:"speedFormatted"`
	Percent          float64  `json:"percent"`
	CurrentFile      string   `json:"currentFile"`
	Message          string   `json:"message"`
	ErrorMessage     string   `json:"errorMessage"`
	Code             string   `json:"code"`
	PeerAddress      string   `json:"peerAddress"`
	MirroredFolders  []string `json:"mirroredFolders,omitempty"`
}

// SenderInitResult returns the code and detected network addresses for sender
type SenderInitResult struct {
	Code                string   `json:"code"`
	DirectAddresses     []string `json:"directAddresses"`
	TailscaleIP         string   `json:"tailscaleIP"`
	SelectedFiles       int      `json:"selectedFiles"`
	TotalBytes          int64    `json:"totalBytes"`
	AbsoluteSyncFolders []string `json:"absoluteSyncFolders,omitempty"`
}

// NetworkInfo provides detected local network and Tailscale addresses
type NetworkInfo struct {
	LocalIPs    []string `json:"localIPs"`
	TailscaleIP string   `json:"tailscaleIP"`
	DefaultPort int      `json:"defaultPort"`
}

// FormatSpeed formats bytes per second into human readable MB/s or KB/s
func FormatSpeed(bytesPerSec float64) string {
	if bytesPerSec >= 1024*1024 {
		return fmt.Sprintf("%.2f MB/s", bytesPerSec/(1024*1024))
	} else if bytesPerSec >= 1024 {
		return fmt.Sprintf("%.1f KB/s", bytesPerSec/1024)
	}
	return fmt.Sprintf("%.0f B/s", bytesPerSec)
}

// FormatBytes formats byte count into human readable string
func FormatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.2f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}
