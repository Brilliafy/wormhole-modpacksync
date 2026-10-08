package transport

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"

	"wormhole-sync/pkg/models"
	"wormhole-sync/pkg/scanner"
	"wormhole-sync/pkg/signaling"
)

type SessionManager struct {
	mu            sync.Mutex
	cancelFunc    context.CancelFunc
	currentStats  models.SyncStats
	onStatsUpdate func(models.SyncStats)

	// Senders
	directSender *DirectSender
	webrtcSender *WebRTCSender

	// Receivers
	directReceiver *DirectReceiver
	webrtcReceiver *WebRTCReceiver

	// Embedded relay
	embeddedRelay *signaling.EmbeddedRelayServer
	relayServer   *http.Server
	relayPort     int
}

func NewSessionManager(onStatsUpdate func(models.SyncStats)) *SessionManager {
	sm := &SessionManager{
		onStatsUpdate: onStatsUpdate,
		currentStats: models.SyncStats{
			Status: "idle",
		},
		relayPort: 42425,
	}
	sm.startEmbeddedRelay()
	return sm
}

func (sm *SessionManager) startEmbeddedRelay() {
	sm.embeddedRelay = signaling.NewEmbeddedRelayServer()
	sm.relayServer = &http.Server{
		Addr:    fmt.Sprintf("0.0.0.0:%d", sm.relayPort),
		Handler: sm.embeddedRelay.Handler(),
	}

	go func() {
		_ = sm.relayServer.ListenAndServe()
	}()
}

// GetNetworkInfo detects available local and Tailscale IPs
func (sm *SessionManager) GetNetworkInfo() *models.NetworkInfo {
	info := &models.NetworkInfo{
		LocalIPs:    make([]string, 0),
		DefaultPort: 42424,
	}

	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return info
	}

	for _, addr := range addrs {
		ipNet, ok := addr.(*net.IPNet)
		if !ok || ipNet.IP.IsLoopback() {
			continue
		}
		ip4 := ipNet.IP.To4()
		if ip4 == nil {
			continue
		}

		ipStr := ip4.String()
		// Detect Tailscale (100.64.0.0/10)
		if strings.HasPrefix(ipStr, "100.") {
			info.TailscaleIP = ipStr
		} else {
			info.LocalIPs = append(info.LocalIPs, ipStr)
		}
	}

	return info
}

func (sm *SessionManager) UpdateStats(stats models.SyncStats) {
	sm.mu.Lock()
	sm.currentStats = stats
	sm.mu.Unlock()

	if sm.onStatsUpdate != nil {
		sm.onStatsUpdate(stats)
	}
}

func (sm *SessionManager) GetStats() models.SyncStats {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	return sm.currentStats
}

// StartSenderSession prepares the sender, scans files, and launches direct or webrtc listener
func (sm *SessionManager) StartSenderSession(rootDir string, selectedPaths []string, absoluteSyncFolders []string, mode string, customCode string) (*models.SenderInitResult, error) {
	sm.Reset()

	ctx, cancel := context.WithCancel(context.Background())
	sm.cancelFunc = cancel

	sm.UpdateStats(models.SyncStats{
		Status:          "scanning",
		Role:            "sender",
		Mode:            mode,
		Message:         "Scanning instance directory and hashing selected files...",
		MirroredFolders: absoluteSyncFolders,
	})

	// Scan and hash files
	manifest, err := scanner.ScanSelectedFiles(rootDir, selectedPaths, absoluteSyncFolders, func(done, total int, current string) {
		sm.UpdateStats(models.SyncStats{
			Status:          "scanning",
			Role:            "sender",
			Mode:            mode,
			TotalFiles:      total,
			CompletedFiles:  done,
			CurrentFile:     current,
			Percent:         float64(done) / float64(total) * 100,
			Message:         fmt.Sprintf("Hashing files: [%d/%d] %s", done, total, current),
			MirroredFolders: absoluteSyncFolders,
		})
	})
	if err != nil {
		sm.UpdateStats(models.SyncStats{
			Status:          "error",
			Role:            "sender",
			Mode:            mode,
			ErrorMessage:    fmt.Sprintf("Failed to scan directory: %v", err),
			MirroredFolders: absoluteSyncFolders,
		})
		return nil, err
	}

	code := customCode
	if code == "" {
		code = signaling.GenerateRoomCode()
	}

	netInfo := sm.GetNetworkInfo()
	directAddrs := make([]string, 0)
	if netInfo.TailscaleIP != "" {
		directAddrs = append(directAddrs, fmt.Sprintf("%s:%d", netInfo.TailscaleIP, netInfo.DefaultPort))
	}
	for _, ip := range netInfo.LocalIPs {
		directAddrs = append(directAddrs, fmt.Sprintf("%s:%d", ip, netInfo.DefaultPort))
	}

	result := &models.SenderInitResult{
		Code:                code,
		DirectAddresses:     directAddrs,
		TailscaleIP:         netInfo.TailscaleIP,
		SelectedFiles:       manifest.TotalFiles,
		TotalBytes:          manifest.TotalBytes,
		AbsoluteSyncFolders: manifest.AbsoluteSyncFolders,
	}

	if mode == "direct" || mode == "tailscale" {
		// Launch Direct TCP Server
		sm.directSender = NewDirectSender(rootDir, manifest, netInfo.DefaultPort, func(stats models.SyncStats) {
			stats.Code = code
			stats.MirroredFolders = manifest.AbsoluteSyncFolders
			sm.UpdateStats(stats)
		})

		go func() {
			err := sm.directSender.Start(ctx)
			if err != nil && ctx.Err() == nil {
				sm.UpdateStats(models.SyncStats{
					Status:          "error",
					Role:            "sender",
					Mode:            "direct",
					ErrorMessage:    err.Error(),
					MirroredFolders: manifest.AbsoluteSyncFolders,
				})
			}
		}()
	} else {
		// Launch WebRTC Hole Punch Session
		sm.webrtcSender = NewWebRTCSender(rootDir, manifest, func(stats models.SyncStats) {
			stats.Code = code
			stats.MirroredFolders = manifest.AbsoluteSyncFolders
			sm.UpdateStats(stats)
		})

		go func() {
			offerSDP, err := sm.webrtcSender.CreateOffer()
			if err != nil {
				sm.UpdateStats(models.SyncStats{
					Status:       "error",
					Role:         "sender",
					Mode:         "webrtc",
					ErrorMessage: fmt.Sprintf("WebRTC offer failed: %v", err),
				})
				return
			}

			// Publish to embedded signaling relay
			sigClient := signaling.NewSignalingClient("")
			directHint := ""
			if len(directAddrs) > 0 {
				directHint = directAddrs[0]
			}
			_ = sigClient.PublishOffer(ctx, code, offerSDP, directHint)

			sm.UpdateStats(models.SyncStats{
				Status:  "waiting",
				Role:    "sender",
				Mode:    "webrtc",
				Code:    code,
				Message: fmt.Sprintf("Ready! Share code [%s] with your friend to connect.", code),
			})

			// Wait for receiver's answer
			answerSDP, err := sigClient.WaitForAnswer(ctx, code)
			if err != nil {
				if ctx.Err() == nil {
					sm.UpdateStats(models.SyncStats{
						Status:       "error",
						Role:         "sender",
						Mode:         "webrtc",
						ErrorMessage: fmt.Sprintf("Failed waiting for answer: %v", err),
					})
				}
				return
			}

			if err := sm.webrtcSender.SetAnswer(answerSDP); err != nil {
				sm.UpdateStats(models.SyncStats{
					Status:       "error",
					Role:         "sender",
					Mode:         "webrtc",
					ErrorMessage: fmt.Sprintf("Failed applying answer: %v", err),
				})
				return
			}

			if err := sm.webrtcSender.StartSession(ctx); err != nil && ctx.Err() == nil {
				sm.UpdateStats(models.SyncStats{
					Status:       "error",
					Role:         "sender",
					Mode:         "webrtc",
					ErrorMessage: err.Error(),
				})
			}
		}()
	}

	return result, nil
}

// StartReceiverSession connects to the sender using code or direct address
func (sm *SessionManager) StartReceiverSession(destDir string, mode string, codeOrAddr string) error {
	sm.Reset()

	ctx, cancel := context.WithCancel(context.Background())
	sm.cancelFunc = cancel

	cleanInput := strings.TrimSpace(codeOrAddr)

	// Check if user input is a direct IP:Port or if mode is direct
	isDirectAddr := strings.Contains(cleanInput, ":") || mode == "direct" || mode == "tailscale"

	if isDirectAddr {
		// Target address
		addr := cleanInput
		if !strings.Contains(addr, ":") {
			addr = fmt.Sprintf("%s:42424", addr)
		}

		sm.directReceiver = NewDirectReceiver(destDir, addr, func(stats models.SyncStats) {
			stats.Code = cleanInput
			sm.UpdateStats(stats)
		})

		go func() {
			err := sm.directReceiver.Start(ctx)
			if err != nil && ctx.Err() == nil {
				sm.UpdateStats(models.SyncStats{
					Status:       "error",
					Role:         "receiver",
					Mode:         "direct",
					ErrorMessage: err.Error(),
				})
			}
		}()
	} else {
		// WebRTC Code Mode
		sm.webrtcReceiver = NewWebRTCReceiver(destDir, func(stats models.SyncStats) {
			stats.Code = cleanInput
			sm.UpdateStats(stats)
		})

		go func() {
			sm.UpdateStats(models.SyncStats{
				Status:  "connecting",
				Role:    "receiver",
				Mode:    "webrtc",
				Code:    cleanInput,
				Message: fmt.Sprintf("Connecting using code [%s]...", cleanInput),
			})

			sigClient := signaling.NewSignalingClient("")
			offerSDP, directAddr, err := sigClient.FetchOffer(ctx, cleanInput)
			if err != nil {
				sm.UpdateStats(models.SyncStats{
					Status:       "error",
					Role:         "receiver",
					Mode:         "webrtc",
					ErrorMessage: fmt.Sprintf("Failed finding session for code [%s]: %v", cleanInput, err),
				})
				return
			}

			// If offerSDP is available, use WebRTC hole punch
			if offerSDP != "" {
				answerSDP, err := sm.webrtcReceiver.CreateAnswer(offerSDP)
				if err != nil {
					sm.UpdateStats(models.SyncStats{
						Status:       "error",
						Role:         "receiver",
						Mode:         "webrtc",
						ErrorMessage: fmt.Sprintf("Failed creating answer: %v", err),
					})
					return
				}

				if err := sigClient.PublishAnswer(ctx, cleanInput, answerSDP); err != nil {
					sm.UpdateStats(models.SyncStats{
						Status:       "error",
						Role:         "receiver",
						Mode:         "webrtc",
						ErrorMessage: fmt.Sprintf("Failed publishing answer: %v", err),
					})
					return
				}

				if err := sm.webrtcReceiver.StartSession(ctx); err != nil && ctx.Err() == nil {
					sm.UpdateStats(models.SyncStats{
						Status:       "error",
						Role:         "receiver",
						Mode:         "webrtc",
						ErrorMessage: err.Error(),
					})
				}
			} else if directAddr != "" {
				// Fallback to direct address discovered via signaling
				sm.directReceiver = NewDirectReceiver(destDir, directAddr, func(stats models.SyncStats) {
					stats.Code = cleanInput
					sm.UpdateStats(stats)
				})
				_ = sm.directReceiver.Start(ctx)
			}
		}()
	}

	return nil
}

func (sm *SessionManager) Pause() {
	if sm.directSender != nil {
		sm.directSender.Pause()
	}
	if sm.webrtcSender != nil {
		sm.webrtcSender.Pause()
	}
}

func (sm *SessionManager) Resume() {
	if sm.directSender != nil {
		sm.directSender.Resume()
	}
	if sm.webrtcSender != nil {
		sm.webrtcSender.Resume()
	}
}

func (sm *SessionManager) Stop() {
	if sm.cancelFunc != nil {
		sm.cancelFunc()
	}
	if sm.directSender != nil {
		sm.directSender.Stop()
	}
	if sm.webrtcSender != nil {
		sm.webrtcSender.Stop()
	}
	if sm.directReceiver != nil {
		sm.directReceiver.Stop()
	}
	if sm.webrtcReceiver != nil {
		sm.webrtcReceiver.Stop()
	}
	sm.UpdateStats(models.SyncStats{
		Status:  "stopped",
		Message: "Transfer stopped.",
	})
}

func (sm *SessionManager) Reset() {
	sm.Stop()
	sm.mu.Lock()
	sm.directSender = nil
	sm.webrtcSender = nil
	sm.directReceiver = nil
	sm.webrtcReceiver = nil
	sm.currentStats = models.SyncStats{
		Status: "idle",
	}
	sm.mu.Unlock()
}
