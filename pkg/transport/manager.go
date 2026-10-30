package transport

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
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
	handler := sm.embeddedRelay.Handler()

	// Bind the default port, falling back to a dynamic one if it is taken by
	// another instance. Advertising a port the relay never bound would break
	// code-based WebRTC entirely, so the resolved port is recorded.
	ln, err := net.Listen("tcp", fmt.Sprintf("0.0.0.0:%d", sm.relayPort))
	if err != nil {
		ln, err = net.Listen("tcp", "0.0.0.0:0")
		if err != nil {
			sm.embeddedRelay = nil
			return
		}
	}
	sm.relayPort = ln.Addr().(*net.TCPAddr).Port
	sm.relayServer = &http.Server{Handler: handler}

	go func() {
		_ = sm.relayServer.Serve(ln)
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

// StartSenderSession launches the network listener and generates the share
// code immediately, then hashes the selected files in the background. The
// share code is available within milliseconds of hitting Start; the manifest
// is delivered to the peer only once hashing completes.
func (sm *SessionManager) StartSenderSession(rootDir string, selectedPaths []string, absoluteSyncFolders []string, mode string) (*models.SenderInitResult, error) {
	sm.Reset()

	ctx, cancel := context.WithCancel(context.Background())
	sm.mu.Lock()
	sm.cancelFunc = cancel
	sm.mu.Unlock()

	code := signaling.GenerateRoomCode()
	netInfo := sm.GetNetworkInfo()

	// Rendezvous selection. A relay both peers can reach is mandatory for the
	// WebRTC handshake; there is no peer discovery without one.
	//   1. WORMHOLE_RELAY set  -> shared self-hosted relay, plain code.
	//   2. Tailscale present   -> sender's embedded relay on the tailnet.
	//   3. otherwise           -> public MQTT rendezvous, so plain codes work
	//                             between different networks with zero setup.
	// WORMHOLE_SIGNAL=local forces case 2 (LAN/Tailnet only, no third party).
	advertisedCode := code
	relayScope := "public"
	relayAddr := ""
	useEmbeddedRelay := false
	switch {
	case signaling.RelayURLFromEnv() != signaling.DefaultRelayURL:
		relayScope = "shared"
		relayAddr = signaling.RelayURLFromEnv()
	case strings.EqualFold(os.Getenv("WORMHOLE_SIGNAL"), "local") || mode == "tailscale":
		useEmbeddedRelay = true
		relayHost := netInfo.TailscaleIP
		if relayHost != "" {
			relayScope = "tailscale"
		} else if len(netInfo.LocalIPs) > 0 {
			relayScope = "lan"
			relayHost = netInfo.LocalIPs[0]
		} else {
			relayScope = "none"
		}
		if relayHost != "" {
			relayAddr = fmt.Sprintf("%s:%d", relayHost, sm.relayPort)
			advertisedCode = fmt.Sprintf("%s@%s", code, relayAddr)
		}
	default:
		relayScope = "public"
	}

	result := &models.SenderInitResult{
		Code:        advertisedCode,
		TailscaleIP: netInfo.TailscaleIP,
		RelayScope:  relayScope,
		RelayAddr:   relayAddr,
	}

	// Hash progress is throttled: emitting one Wails event per file floods
	// the WebView on instances with tens of thousands of files.
	progressThrottle := newProgressThrottle(100 * time.Millisecond)
	var progressMu sync.Mutex

	// Manifest preparation. The provider hands the finished manifest to the
	// transport layer whenever the peer handshake happens; if the peer
	// connects first, the transport waits for hashing to complete.
	var manifest *models.SyncManifest
	ready := make(chan struct{})
	provider := func() *models.SyncManifest {
		<-ready
		return manifest
	}

	go func() {
		defer close(ready)

		onScan := func(found int, current string) {
			progressMu.Lock()
			defer progressMu.Unlock()
			if ctx.Err() != nil || !progressThrottle.allow() {
				return
			}
			sm.UpdateStats(models.SyncStats{
				Status:         "scanning",
				Role:           "sender",
				Mode:           mode,
				Code:           advertisedCode,
				CompletedFiles: found,
				CurrentFile:    current,
				Message:        fmt.Sprintf("Scanning selection: %d files found...", found),
			})
		}

		m, err := scanner.ScanSelectedFiles(ctx, rootDir, selectedPaths, absoluteSyncFolders, onScan, func(done, total int, current string) {
			progressMu.Lock()
			defer progressMu.Unlock()
			if !progressThrottle.allow() && done != total {
				return
			}
			if ctx.Err() != nil {
				return
			}
			sm.UpdateStats(models.SyncStats{
				Status:          "scanning",
				Role:            "sender",
				Mode:            mode,
				Code:            advertisedCode,
				TotalFiles:      total,
				CompletedFiles:  done,
				CurrentFile:     current,
				Percent:         float64(done) / float64(total) * 100,
				Message:         fmt.Sprintf("Hashing files: [%d/%d] %s", done, total, current),
				MirroredFolders: absoluteSyncFolders,
			})
		})

		manifest = m
		if err != nil {
			if ctx.Err() == nil {
				sm.UpdateStats(models.SyncStats{
					Status:          "error",
					Role:            "sender",
					Mode:            mode,
					Code:            advertisedCode,
					ErrorMessage:    fmt.Sprintf("Failed to scan directory: %v", err),
					MirroredFolders: absoluteSyncFolders,
				})
			}
			return
		}
		if ctx.Err() != nil {
			return
		}
		// NOTE: result was already returned to the caller; file counts reach
		// the UI through the stats event below instead of mutating it here.
		sm.UpdateStats(models.SyncStats{
			Status:          "waiting",
			Role:            "sender",
			Mode:            mode,
			Code:            advertisedCode,
			TotalFiles:      m.TotalFiles,
			TotalBytes:      m.TotalBytes,
			MirroredFolders: absoluteSyncFolders,
			Message:         fmt.Sprintf("Ready to send %d files (%s). Waiting for friend to connect...", m.TotalFiles, models.FormatBytes(m.TotalBytes)),
		})
	}()

	if mode == "direct" || mode == "tailscale" {
		// Launch Direct TCP Server. Bind immediately so the advertised port is
		// known and the share code can be shown right away.
		directSender := NewDirectSender(rootDir, provider, netInfo.DefaultPort, func(stats models.SyncStats) {
			stats.Code = advertisedCode
			stats.MirroredFolders = absoluteSyncFolders
			sm.UpdateStats(stats)
		})
		sm.mu.Lock()
		sm.directSender = directSender
		sm.mu.Unlock()

		actualPort := netInfo.DefaultPort
		if err := directSender.Listen(fmt.Sprintf("0.0.0.0:%d", netInfo.DefaultPort)); err == nil {
			actualPort = directSender.Port()
		}

		directAddrs := make([]string, 0)
		if netInfo.TailscaleIP != "" {
			directAddrs = append(directAddrs, fmt.Sprintf("%s:%d", netInfo.TailscaleIP, actualPort))
		}
		for _, ip := range netInfo.LocalIPs {
			directAddrs = append(directAddrs, fmt.Sprintf("%s:%d", ip, actualPort))
		}
		result.DirectAddresses = directAddrs
		if len(directAddrs) > 0 {
			advertisedCode = directAddrs[0]
			result.Code = directAddrs[0]
		}

		go func() {
			// Local capture: Reset() may nil out sm.directSender while this
			// goroutine is still pending, which would crash here.
			err := directSender.Start(ctx)
			if err != nil && ctx.Err() == nil {
				sm.UpdateStats(models.SyncStats{
					Status:          "error",
					Role:            "sender",
					Mode:            "direct",
					ErrorMessage:    err.Error(),
					MirroredFolders: absoluteSyncFolders,
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
		webrtcSender := NewWebRTCSender(rootDir, provider, func(stats models.SyncStats) {
			stats.Code = advertisedCode
			stats.MirroredFolders = absoluteSyncFolders
			sm.UpdateStats(stats)
		})
		sm.mu.Lock()
		sm.webrtcSender = webrtcSender
		sm.mu.Unlock()

		go func() {
			// Local capture: Reset() may nil out sm.webrtcSender while this
			// goroutine is still pending, which would crash here.
			offerSDP, err := webrtcSender.CreateOffer()
			if err != nil {
				sm.UpdateStats(models.SyncStats{
					Status:       "error",
					Role:         "sender",
					Mode:         "webrtc",
					Code:         advertisedCode,
					ErrorMessage: fmt.Sprintf("WebRTC offer failed: %v", err),
				})
				return
			}

			// Publish to the rendezvous so the receiver can answer as soon as
			// it wants; hashing does not gate the handshake.
			sigClient := signaling.NewRoomSignaler("", code)
			if useEmbeddedRelay {
				sigClient = signaling.NewSignalingClient(fmt.Sprintf("http://127.0.0.1:%d", sm.relayPort))
			}
			defer sigClient.Close()
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

			waitingMsg := fmt.Sprintf("Ready! Share code [%s] with your friend to connect.", advertisedCode)
			if symmetric, relay := webrtcSender.NATDiagnosis(); symmetric && !relay {
				waitingMsg = fmt.Sprintf("Ready! Share code [%s] with your friend. (Symmetric NAT detected: direct P2P or encrypted transit bridge will connect automatically)", advertisedCode)
			}
			sm.UpdateStats(models.SyncStats{
				Status:  "waiting",
				Role:    "sender",
				Mode:    "webrtc",
				Code:    advertisedCode,
				Message: waitingMsg,
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

			if err := webrtcSender.SetAnswer(answerSDP); err != nil {
				sm.UpdateStats(models.SyncStats{
					Status:       "error",
					Role:         "sender",
					Mode:         "webrtc",
					ErrorMessage: fmt.Sprintf("Failed applying answer: %v", err),
				})
				return
			}

			if err := webrtcSender.StartSession(ctx); err != nil && ctx.Err() == nil {
				if errors.Is(err, context.Canceled) {
					return
				}
				log.Printf("[SENDER] Direct WebRTC hole-punch could not establish (%v). Falling back to secure Transit Relay...", err)
				sm.UpdateStats(models.SyncStats{
					Status:          "connecting",
					Role:            "sender",
					Mode:            "webrtc",
					Code:            advertisedCode,
					Message:         "Direct P2P punch blocked by symmetric NAT. Falling back to Encrypted Transit Bridge...",
					MirroredFolders: absoluteSyncFolders,
				})

				transitConn, tErr := DialTransitRelay(ctx, code, true)
				if tErr != nil {
					sm.UpdateStats(models.SyncStats{
						Status:          "error",
						Role:            "sender",
						Mode:            "webrtc",
						Code:            advertisedCode,
						ErrorMessage:    fmt.Sprintf("NAT traversal and relay fallback failed: %v", tErr),
						MirroredFolders: absoluteSyncFolders,
					})
					return
				}

				directSender := NewDirectSender(rootDir, provider, 0, func(stats models.SyncStats) {
					stats.Code = advertisedCode
					stats.Mode = "webrtc"
					stats.MirroredFolders = absoluteSyncFolders
					sm.UpdateStats(stats)
				})
				sm.mu.Lock()
				sm.directSender = directSender
				sm.mu.Unlock()

				if err := directSender.StartWithConn(ctx, transitConn); err != nil && ctx.Err() == nil {
					sm.UpdateStats(models.SyncStats{
						Status:          "error",
						Role:            "sender",
						Mode:            "webrtc",
						Code:            advertisedCode,
						ErrorMessage:    err.Error(),
						MirroredFolders: absoluteSyncFolders,
					})
				}
				return
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
	sm.mu.Lock()
	sm.cancelFunc = cancel
	sm.mu.Unlock()

	roomCode, relayOverride, isDirectAddr := parseReceiverInput(codeOrAddr)

	if mode == "direct" || mode == "tailscale" || isDirectAddr {
		// Target address
		addr := roomCode
		if !strings.Contains(addr, ":") {
			addr = fmt.Sprintf("%s:42424", addr)
		}

		directReceiver := NewDirectReceiver(destDir, addr, func(stats models.SyncStats) {
			stats.Code = roomCode
			sm.UpdateStats(stats)
		})
		sm.mu.Lock()
		sm.directReceiver = directReceiver
		sm.mu.Unlock()

		go func() {
			// Local capture: Reset() may nil out sm.directReceiver while
			// this goroutine is still pending, which would crash here.
			err := directReceiver.Start(ctx)
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
		webrtcReceiver := NewWebRTCReceiver(destDir, func(stats models.SyncStats) {
			stats.Code = roomCode
			sm.UpdateStats(stats)
		})
		sm.mu.Lock()
		sm.webrtcReceiver = webrtcReceiver
		sm.mu.Unlock()

		go func() {
			effectiveRelay := signaling.RoomSignalerLabel(relayOverride)
			sm.UpdateStats(models.SyncStats{
				Status:  "connecting",
				Role:    "receiver",
				Mode:    "webrtc",
				Code:    roomCode,
				Message: fmt.Sprintf("Contacting rendezvous %s for code [%s]...", effectiveRelay, roomCode),
			})

			sigClient := signaling.NewRoomSignaler(relayOverride, roomCode)
			defer sigClient.Close()
			fetchCtx, cancelFetch := context.WithTimeout(ctx, 2*time.Minute)
			defer cancelFetch()
			offerSDP, directAddr, err := sigClient.FetchOffer(fetchCtx, roomCode)
			if err != nil {
				if ctx.Err() != nil {
					return
				}
				if fetchCtx.Err() != nil {
					sm.UpdateStats(models.SyncStats{
						Status:       "error",
						Role:         "receiver",
						Mode:         "webrtc",
						Code:         roomCode,
						ErrorMessage: fmt.Sprintf("No session found for code [%s] on relay %s. Check the code, or make sure the sender is still running.", roomCode, effectiveRelay),
					})
					return
				}
				sm.UpdateStats(models.SyncStats{
					Status:       "error",
					Role:         "receiver",
					Mode:         "webrtc",
					Code:         roomCode,
					ErrorMessage: fmt.Sprintf("Could not reach the signaling relay %s for code [%s]. A code containing a 192.168.x.x / 10.x.x.x address only works on the sender's local network: use Direct / Tailscale mode, enable Tailscale on both PCs, or run a shared relay and set WORMHOLE_RELAY on both sides. (%v)", effectiveRelay, roomCode, err),
				})
				return
			}

			// If offerSDP is available, use WebRTC hole punch
			if offerSDP != "" {
				answerSDP, err := webrtcReceiver.CreateAnswer(offerSDP)
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

				if err := webrtcReceiver.StartSession(ctx); err != nil && ctx.Err() == nil {
					if errors.Is(err, context.Canceled) {
						return
					}
					log.Printf("[RECEIVER] Direct WebRTC hole-punch could not establish (%v). Falling back to secure Transit Relay...", err)
					sm.UpdateStats(models.SyncStats{
						Status:  "connecting",
						Role:    "receiver",
						Mode:    "webrtc",
						Code:    roomCode,
						Message: "Direct P2P punch blocked by symmetric NAT. Falling back to Encrypted Transit Bridge...",
					})

					transitConn, tErr := DialTransitRelay(ctx, roomCode, false)
					if tErr != nil {
						sm.UpdateStats(models.SyncStats{
							Status:       "error",
							Role:         "receiver",
							Mode:         "webrtc",
							Code:         roomCode,
							ErrorMessage: fmt.Sprintf("NAT traversal and relay fallback failed: %v", tErr),
						})
						return
					}

					directReceiver := NewDirectReceiver(destDir, "", func(stats models.SyncStats) {
						stats.Code = roomCode
						stats.Mode = "webrtc"
						sm.UpdateStats(stats)
					})
					sm.mu.Lock()
					sm.directReceiver = directReceiver
					sm.mu.Unlock()

					if err := directReceiver.StartWithConn(ctx, transitConn); err != nil && ctx.Err() == nil {
						sm.UpdateStats(models.SyncStats{
							Status:       "error",
							Role:         "receiver",
							Mode:         "webrtc",
							Code:         roomCode,
							ErrorMessage: err.Error(),
						})
					}
					return
				}
			} else if directAddr != "" {
				// Fallback to direct address discovered via signaling
				fallbackReceiver := NewDirectReceiver(destDir, directAddr, func(stats models.SyncStats) {
					stats.Code = roomCode
					sm.UpdateStats(stats)
				})
				sm.mu.Lock()
				sm.directReceiver = fallbackReceiver
				sm.mu.Unlock()
				_ = fallbackReceiver.Start(ctx)
			}
		}()
	}

	return nil
}

func (sm *SessionManager) Pause() {
	// Field reads must hold sm.mu: StartSenderSession/Reset mutate them
	// concurrently.
	sm.mu.Lock()
	directSender := sm.directSender
	webrtcSender := sm.webrtcSender
	sm.mu.Unlock()
	if directSender != nil {
		directSender.Pause()
	}
	if webrtcSender != nil {
		webrtcSender.Pause()
	}
}

func (sm *SessionManager) Resume() {
	sm.mu.Lock()
	directSender := sm.directSender
	webrtcSender := sm.webrtcSender
	sm.mu.Unlock()
	if directSender != nil {
		directSender.Resume()
	}
	if webrtcSender != nil {
		webrtcSender.Resume()
	}
}

func (sm *SessionManager) Stop() {
	if sm.stopActive() {
		sm.UpdateStats(models.SyncStats{
			Status:  "stopped",
			Message: "Transfer stopped.",
		})
	}
}

// stopActive cancels the session context and tears down transports without
// emitting a stats event. It reports whether a session was actually running.
// Reset uses this so starting a new session does not flash a stale "stopped"
// status at the UI.
func (sm *SessionManager) stopActive() bool {
	sm.mu.Lock()
	cancelFunc := sm.cancelFunc
	directSender := sm.directSender
	webrtcSender := sm.webrtcSender
	directReceiver := sm.directReceiver
	webrtcReceiver := sm.webrtcReceiver
	sm.mu.Unlock()

	active := cancelFunc != nil || directSender != nil || webrtcSender != nil ||
		directReceiver != nil || webrtcReceiver != nil

	if cancelFunc != nil {
		cancelFunc()
	}
	if directSender != nil {
		directSender.Stop()
	}
	if webrtcSender != nil {
		webrtcSender.Stop()
	}
	if directReceiver != nil {
		directReceiver.Stop()
	}
	if webrtcReceiver != nil {
		webrtcReceiver.Stop()
	}
	return active
}

func (sm *SessionManager) Reset() {
	sm.stopActive()
	sm.mu.Lock()
	sm.cancelFunc = nil
	sm.directSender = nil
	sm.webrtcSender = nil
	sm.directReceiver = nil
	sm.webrtcReceiver = nil
	sm.currentStats = models.SyncStats{
		Status: "idle",
	}
	sm.mu.Unlock()
}
