package main

import (
	"context"
	"fmt"

	"github.com/wailsapp/wails/v2/pkg/runtime"
	"wormhole-sync/pkg/models"
	"wormhole-sync/pkg/scanner"
	"wormhole-sync/pkg/transport"
)

// App struct
type App struct {
	ctx            context.Context
	sessionManager *transport.SessionManager
}

// NewApp creates a new App application struct
func NewApp() *App {
	return &App{}
}

// startup is called when the app starts. The context is saved
// so we can call the runtime methods
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	a.sessionManager = transport.NewSessionManager(func(stats models.SyncStats) {
		runtime.EventsEmit(a.ctx, "sync:stats", stats)
	})
}

// SelectFolder opens a native directory picker dialog
func (a *App) SelectFolder() (string, error) {
	selected, err := runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "Select Folder to Sync",
	})
	if err != nil {
		return "", err
	}
	return selected, nil
}

// ScanFolder produces a hierarchical FileTreeNode for the sender UI
func (a *App) ScanFolder(folderPath string) (*models.FileTreeNode, error) {
	if folderPath == "" {
		return nil, fmt.Errorf("folder path cannot be empty")
	}
	return scanner.BuildFileTree(folderPath)
}

// GetNetworkInfo returns detected local IPs and Tailscale IP
func (a *App) GetNetworkInfo() *models.NetworkInfo {
	if a.sessionManager == nil {
		return &models.NetworkInfo{DefaultPort: 42424}
	}
	return a.sessionManager.GetNetworkInfo()
}

// StartSender starts the sender session with selected file paths and optional absolute sync folders
func (a *App) StartSender(folderPath string, selectedPaths []string, absoluteSyncFolders []string, mode string, customCode string) (*models.SenderInitResult, error) {
	if a.sessionManager == nil {
		return nil, fmt.Errorf("session manager not initialized")
	}
	return a.sessionManager.StartSenderSession(folderPath, selectedPaths, absoluteSyncFolders, mode, customCode)
}

// StartReceiver starts the receiver session
func (a *App) StartReceiver(destFolder string, mode string, codeOrAddr string) error {
	if a.sessionManager == nil {
		return fmt.Errorf("session manager not initialized")
	}
	return a.sessionManager.StartReceiverSession(destFolder, mode, codeOrAddr)
}

// Pause pauses the active transfer
func (a *App) Pause() {
	if a.sessionManager != nil {
		a.sessionManager.Pause()
	}
}

// Resume resumes the active transfer
func (a *App) Resume() {
	if a.sessionManager != nil {
		a.sessionManager.Resume()
	}
}

// Stop stops the active transfer
func (a *App) Stop() {
	if a.sessionManager != nil {
		a.sessionManager.Stop()
	}
}

// Reset clears the active session back to idle
func (a *App) Reset() {
	if a.sessionManager != nil {
		a.sessionManager.Reset()
	}
}

// GetStats returns the current sync status and progress
func (a *App) GetStats() models.SyncStats {
	if a.sessionManager == nil {
		return models.SyncStats{Status: "idle"}
	}
	return a.sessionManager.GetStats()
}
