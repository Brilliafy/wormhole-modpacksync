package transport

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

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
func (sm *SessionManager) StartSenderSession(rootDir string, selectedPaths []string, absoluteSyncFolders []string, mode string) (*models.SenderInitResult, error) {
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

	code := signaling.GenerateRoomCode()

	netInfo := sm.GetNetworkInfo()

	// When no shared relay is configured, use the sender's own embedded relay
	// as the rendezvous point and advertise its address inside the shared code
	// (form "<code>@<host:port>"). This lets code-based WebRTC work on a LAN or
	// over Tailscale with zero extra infrastructure. With WORMHOLE_RELAY set,
	// the plain code is used and both peers meet on the shared relay instead.
	advertisedCode := code
	if signaling.RelayURLFromEnv() == "http://127.0.0.1:42425" {
		relayHost := netInfo.TailscaleIP
		if relayHost == "" && len(netInfo.LocalIPs) > 0 {
			relayHost = netInfo.LocalIPs[0]
		}
		if relayHost != "" {
			advertisedCode = fmt.Sprintf("%s@%s:%d", code, relayHost, sm.relayPort)
		}
	}

	result := &models.SenderInitResult{
		Code:                advertisedCode,
		TailscaleIP:         netInfo.TailscaleIP,
		SelectedFiles:       manifest.TotalFiles,
		TotalBytes:          manifest.TotalBytes,
		AbsoluteSyncFolders: manifest.AbsoluteSyncFolders,
	}

	if mode == "direct" || mode == "tailscale" {
		// Launch Direct TCP Server. Bind first so the advertised port is the
		// real one even if the default port was busy and we fell back.
		sm.directSender = NewDirectSender(rootDir, manifest, netInfo.DefaultPort, func(stats models.SyncStats) {
			stats.Code = advertisedCode
			stats.MirroredFolders = manifest.AbsoluteSyncFolders
			sm.UpdateStats(stats)
		})

		actualPort := netInfo.DefaultPort
		if err := sm.directSender.Listen(fmt.Sprintf("0.0.0.0:%d", netInfo.DefaultPort)); err == nil {
			actualPort = sm.directSender.Port()
		}

		directAddrs := make([]string, 0)
		if netInfo.TailscaleIP != "" {
			directAddrs = append(directAddrs, fmt.Sprintf("%s:%d", netInfo.TailscaleIP, actualPort))
		}
		for _, ip := range netInfo.LocalIPs {
			directAddrs = append(directAddrs, fmt.Sprintf("%s:%d", ip, actualPort))
		}
		result.DirectAddresses = directAddrs

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
		directAddrs := make([]string, 0)
		if netInfo.TailscaleIP != "" {
			directAddrs = append(directAddrs, fmt.Sprintf("%s:%d", netInfo.TailscaleIP, netInfo.DefaultPort))
		}
		for _, ip := range netInfo.LocalIPs {
			directAddrs = append(directAddrs, fmt.Sprintf("%s:%d", ip, netInfo.DefaultPort))
		}
		result.DirectAddresses = directAddrs

		// Launch WebRTC Hole Punch Session
		sm.webrtcSender = NewWebRTCSender(rootDir, manifest, func(stats models.SyncStats) {
			stats.Code = advertisedCode
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
			if err := sigClient.PublishOffer(ctx, code, offerSDP, directHint); err != nil {
				sm.UpdateStats(models.SyncStats{
					Status:       "error",
					Role:         "sender",
					Mode:         "webrtc",
					Code:         advertisedCode,
					ErrorMessage: fmt.Sprintf("Could not reach the signaling relay: %v. Use Direct / Tailscale mode, or set WORMHOLE_RELAY to a relay both peers can reach.", err),
				})
				return
			}

			sm.UpdateStats(models.SyncStats{
				Status:  "waiting",
				Role:    "sender",
				Mode:    "webrtc",
				Code:    advertisedCode,
				Message: fmt.Sprintf("Ready! Share code [%s] with your friend to connect.", advertisedCode),
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

// parseReceiverInput splits the receiver's input into a room code, an optional
// "<code>@<host:port>" relay override, and whether the input targets a direct
// address (a bare IP or an IP:port). Room codes never contain ':' or '@'.
func parseReceiverInput(input string) (roomCode string, relayOverride string, isDirect bool) {
	roomCode = strings.TrimSpace(input)
	if at := strings.Index(roomCode, "@"); at > 0 {
		relayOverride = strings.TrimSpace(roomCode[at+1:])
		roomCode = strings.TrimSpace(roomCode[:at])
	}
	isDirect = strings.Contains(roomCode, ":") || net.ParseIP(roomCode) != nil
	return roomCode, relayOverride, isDirect
}

// StartReceiverSession connects to the sender using code or direct address
func (sm *SessionManager) StartReceiverSession(destDir string, mode string, codeOrAddr string) error {
	sm.Reset()

	ctx, cancel := context.WithCancel(context.Background())
	sm.cancelFunc = cancel

	roomCode, relayOverride, isDirectAddr := parseReceiverInput(codeOrAddr)

	if isDirectAddr {
		// Target address
		addr := roomCode
		if !strings.Contains(addr, ":") {
			addr = fmt.Sprintf("%s:42424", addr)
		}

		sm.directReceiver = NewDirectReceiver(destDir, addr, func(stats models.SyncStats) {
			stats.Code = roomCode
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
			stats.Code = roomCode
			sm.UpdateStats(stats)
		})

		go func() {
			sm.UpdateStats(models.SyncStats{
				Status:  "connecting",
				Role:    "receiver",
				Mode:    "webrtc",
				Code:    roomCode,
				Message: fmt.Sprintf("Connecting using code [%s]...", roomCode),
			})

			sigClient := signaling.NewSignalingClient(relayOverride)
			fetchCtx, cancelFetch := context.WithTimeout(ctx, 2*time.Minute)
			defer cancelFetch()
			offerSDP, directAddr, err := sigClient.FetchOffer(fetchCtx, roomCode)
			if err != nil {
				sm.UpdateStats(models.SyncStats{
					Status:       "error",
					Role:         "receiver",
					Mode:         "webrtc",
					Code:         roomCode,
					ErrorMessage: fmt.Sprintf("Could not find a session for code [%s]. Double-check the code, or use Direct / Tailscale mode with the host's IP address. (%v)", roomCode, err),
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

				if err := sigClient.PublishAnswer(ctx, roomCode, answerSDP); err != nil {
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
					stats.Code = roomCode
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
