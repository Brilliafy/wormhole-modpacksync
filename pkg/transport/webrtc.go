package transport

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pion/ice/v4"
	"github.com/pion/webrtc/v4"
	"wormhole-sync/pkg/atomicfile"
	"wormhole-sync/pkg/delta"
	"wormhole-sync/pkg/models"
)

const WebRTCChunkSize = 32 * 1024

// DefaultICEServers provides standard STUN servers for NAT hole-punching
var DefaultICEServers = []webrtc.ICEServer{
	{
		URLs: []string{
			"stun:stun.l.google.com:19302",
			"stun:stun1.l.google.com:19302",
			"stun:stun2.l.google.com:19302",
			"stun:stun.cloudflare.com:3478",
		},
	},
}

// WebRTCSender handles P2P file sync over WebRTC DataChannels
type WebRTCSender struct {
	mu               sync.Mutex
	peerConn         *webrtc.PeerConnection
	dataChan         *webrtc.DataChannel
	manifestProvider func() *models.SyncManifest
	rootDir          string
	speedTracker     *SpeedTracker
	paused           int32
	stopped          int32
	pauseCond        *sync.Cond
	pauseMu          sync.Mutex
	onProgress       func(models.SyncStats)
	openChan         chan struct{}
	msgChan          chan []byte
	openOnce         sync.Once
	symmetricNAT     int32
	relayCandidate   int32
}

// iceServers returns the STUN servers plus an optional TURN server taken from
// WORMHOLE_TURN_URL / WORMHOLE_TURN_USERNAME / WORMHOLE_TURN_CREDENTIAL. TURN
// is the only reliable fallback for symmetric NATs that STUN cannot punch.
func iceServers() []webrtc.ICEServer {
	servers := make([]webrtc.ICEServer, len(DefaultICEServers))
	copy(servers, DefaultICEServers)
	turnURLs := strings.TrimSpace(os.Getenv("WORMHOLE_TURN_URL"))
	if turnURLs != "" {
		servers = append(servers, webrtc.ICEServer{
			URLs:       strings.Split(turnURLs, ","),
			Username:   os.Getenv("WORMHOLE_TURN_USERNAME"),
			Credential: os.Getenv("WORMHOLE_TURN_CREDENTIAL"),
		})
	}
	return servers
}

// NewWebRTCSender creates a sender. The manifest is fetched through
// manifestProvider when the data channel opens, so the offer can be published
// and the share code shown while background hashing is still running.
func NewWebRTCSender(rootDir string, manifestProvider func() *models.SyncManifest, onProgress func(models.SyncStats)) *WebRTCSender {
	s := &WebRTCSender{
		rootDir:          rootDir,
		manifestProvider: manifestProvider,
		speedTracker:     NewSpeedTracker(),
		onProgress:       onProgress,
	}
	s.pauseCond = sync.NewCond(&s.pauseMu)
	return s
}

// newPeerConnection builds a PeerConnection with app-wide ICE tuning applied.
func newPeerConnection(cfg webrtc.Configuration) (*webrtc.PeerConnection, error) {
	settingEngine := webrtc.SettingEngine{}
	// Real IPs beat mDNS .local names for desktop-to-desktop transfers: mDNS
	// resolution silently fails on networks that filter multicast traffic.
	settingEngine.SetICEMulticastDNSMode(ice.MulticastDNSModeDisabled)
	api := webrtc.NewAPI(webrtc.WithSettingEngine(settingEngine))
	return api.NewPeerConnection(cfg)
}

// analyzeNAT inspects gathered candidates from a local SDP.
//
// A cone NAT reports the same external port to every STUN server; a symmetric
// (endpoint-dependent) NAT reports a different port per destination. When that
// happens no amount of STUN hole punching lets a peer reach us, so the only
// options are a TURN relay or a different path (Tailscale).
func analyzeNAT(sdp *webrtc.SessionDescription) (symmetric bool, relay bool) {
	if sdp == nil {
		return false, false
	}
	srflxPorts := map[string]struct{}{}
	for _, line := range strings.Split(sdp.SDP, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "a=candidate:") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 8 {
			continue
		}
		switch fields[7] {
		case "relay":
			relay = true
		case "srflx":
			srflxPorts[fields[5]] = struct{}{}
		}
	}
	return len(srflxPorts) > 1, relay
}

func boolToInt32(b bool) int32 {
	if b {
		return 1
	}
	return 0
}

// NATDiagnosis reports what the last gathered SDP revealed about this network.
func (s *WebRTCSender) NATDiagnosis() (symmetric bool, relay bool) {
	return atomic.LoadInt32(&s.symmetricNAT) == 1, atomic.LoadInt32(&s.relayCandidate) == 1
}

// natHint returns the actionable sentence appended to ICE failure errors.
func natHint(symmetric, relay bool) string {
	switch {
	case relay:
		return "TURN relay candidates were gathered but the connection still failed; check that the TURN server allows relaying and that both peers share the same code."
	case symmetric:
		return "Your network uses symmetric NAT (every destination gets a different external port), so STUN hole punching cannot work. Use Direct / Tailscale mode with Tailscale enabled on both PCs, or set WORMHOLE_TURN_URL to a TURN server (a free Cloudflare Calls TURN app works well)."
	default:
		return "Hole punching failed (one peer is behind a strict or symmetric NAT). Use Direct / Tailscale mode with Tailscale enabled on both PCs, or set WORMHOLE_TURN_URL to a TURN server."
	}
}

// CreateOffer initializes the WebRTC connection and returns base64-encoded offer with ICE candidates gathered
func (s *WebRTCSender) CreateOffer() (string, error) {
	pc, err := newPeerConnection(webrtc.Configuration{ICEServers: iceServers()})
	if err != nil {
		return "", fmt.Errorf("failed to create peer connection: %w", err)
	}
	s.peerConn = pc

	// Surface NAT traversal failures instead of hanging forever.
	pc.OnICEConnectionStateChange(func(state webrtc.ICEConnectionState) {
		if state == webrtc.ICEConnectionStateFailed {
			symmetric := atomic.LoadInt32(&s.symmetricNAT) == 1
			relay := atomic.LoadInt32(&s.relayCandidate) == 1
			s.updateStatus(models.SyncStats{
				Status:       "error",
				Role:         "sender",
				Mode:         "webrtc",
				ErrorMessage: "NAT traversal failed (ICE). " + natHint(symmetric, relay),
			})
		}
	})

	dc, err := pc.CreateDataChannel("wormhole-sync", nil)
	if err != nil {
		pc.Close()
		return "", fmt.Errorf("failed to create data channel: %w", err)
	}
	s.dataChan = dc

	// Register channel callbacks BEFORE negotiation so a fast connection can
	// never be missed by a late StartSession call.
	s.openChan = make(chan struct{})
	s.msgChan = make(chan []byte, 256)
	dc.OnOpen(func() {
		s.openOnce.Do(func() { close(s.openChan) })
	})
	dc.OnMessage(func(msg webrtc.DataChannelMessage) {
		s.msgChan <- msg.Data
	})

	offer, err := pc.CreateOffer(nil)
	if err != nil {
		pc.Close()
		return "", fmt.Errorf("failed to create offer: %w", err)
	}

	gatherComplete := webrtc.GatheringCompletePromise(pc)
	if err = pc.SetLocalDescription(offer); err != nil {
		pc.Close()
		return "", fmt.Errorf("failed to set local description: %w", err)
	}

	// Wait for ICE candidates gathering to finish so the SDP is completely self-contained
	<-gatherComplete

	offerJSON, err := json.Marshal(pc.LocalDescription())
	if err != nil {
		return "", err
	}

	symmetric, relay := analyzeNAT(pc.LocalDescription())
	atomic.StoreInt32(&s.symmetricNAT, boolToInt32(symmetric))
	atomic.StoreInt32(&s.relayCandidate, boolToInt32(relay))

	return base64.StdEncoding.EncodeToString(offerJSON), nil
}

// SetAnswer sets the remote SDP answer received from the peer
func (s *WebRTCSender) SetAnswer(answerBase64 string) error {
	raw, err := base64.StdEncoding.DecodeString(answerBase64)
	if err != nil {
		return fmt.Errorf("invalid base64 answer: %w", err)
	}

	var answer webrtc.SessionDescription
	if err := json.Unmarshal(raw, &answer); err != nil {
		return fmt.Errorf("invalid json answer: %w", err)
	}

	return s.peerConn.SetRemoteDescription(answer)
}

func (s *WebRTCSender) Pause() {
	atomic.StoreInt32(&s.paused, 1)
	s.updateStatus(models.SyncStats{
		Status:  "paused",
		Role:    "sender",
		Mode:    "webrtc",
		Message: "Transfer paused",
	})
}

func (s *WebRTCSender) Resume() {
	atomic.StoreInt32(&s.paused, 0)
	s.pauseCond.Broadcast()
	s.updateStatus(models.SyncStats{
		Status:  "syncing",
		Role:    "sender",
		Mode:    "webrtc",
		Message: "Resuming transfer...",
	})
}

func (s *WebRTCSender) Stop() {
	atomic.StoreInt32(&s.stopped, 1)
	atomic.StoreInt32(&s.paused, 0)
	s.pauseCond.Broadcast()
	if s.peerConn != nil {
		_ = s.peerConn.Close()
	}
}

func (s *WebRTCSender) checkPause() bool {
	for atomic.LoadInt32(&s.paused) == 1 {
		if atomic.LoadInt32(&s.stopped) == 1 {
			return false
		}
		s.pauseMu.Lock()
		s.pauseCond.Wait()
		s.pauseMu.Unlock()
	}
	return atomic.LoadInt32(&s.stopped) == 0
}

func (s *WebRTCSender) updateStatus(stats models.SyncStats) {
	if s.onProgress != nil {
		s.onProgress(stats)
	}
}

// StartSession waits for DataChannel open and coordinates file sync
func (s *WebRTCSender) StartSession(ctx context.Context) error {
	if s.dataChan == nil || s.openChan == nil || s.msgChan == nil {
		return fmt.Errorf("data channel not initialised: call CreateOffer first")
	}

	s.updateStatus(models.SyncStats{
		Status:  "waiting",
		Role:    "sender",
		Mode:    "webrtc",
		Message: "Hole-punching peer connection (WebRTC/STUN)...",
	})

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-s.openChan:
	}

	s.updateStatus(models.SyncStats{
		Status:  "connecting",
		Role:    "sender",
		Mode:    "webrtc",
		Message: "Direct P2P DataChannel established! Exchanging manifest...",
	})

	// Fetch the manifest. The provider blocks until background hashing
	// finishes, so offer publication and code display are never delayed by it.
	s.updateStatus(models.SyncStats{
		Status:  "analyzing",
		Role:    "sender",
		Mode:    "webrtc",
		Message: "Finishing file preparation (hashing)...",
	})
	manifest := s.manifestProvider()
	if manifest == nil {
		return fmt.Errorf("manifest preparation failed or was cancelled")
	}

	// Send manifest
	manifestMsg := &ControlMessage{Type: MsgManifest, Manifest: manifest}
	manifestBytes, _ := json.Marshal(manifestMsg)
	if err := s.dataChan.Send(manifestBytes); err != nil {
		return err
	}

	// Wait for REQUEST_DELTA
	var deltaMsg ControlMessage
	select {
	case <-ctx.Done():
		return ctx.Err()
	case data := <-s.msgChan:
		if err := json.Unmarshal(data, &deltaMsg); err != nil || deltaMsg.Type != MsgRequestDelta {
			return fmt.Errorf("unexpected message waiting for delta request: %v", err)
		}
	}

	neededPaths := make(map[string]bool)
	for _, p := range deltaMsg.DeltaReq {
		neededPaths[p] = true
	}

	filesToSend := make([]models.FileInfo, 0)
	var totalDeltaBytes int64 = 0
	for _, f := range manifest.Files {
		if neededPaths[f.RelPath] {
			filesToSend = append(filesToSend, f)
			totalDeltaBytes += f.Size
		}
	}

	totalNeededFiles := len(filesToSend)
	if totalNeededFiles == 0 {
		completeBytes, _ := json.Marshal(&ControlMessage{Type: MsgComplete})
		_ = s.dataChan.Send(completeBytes)
		s.updateStatus(models.SyncStats{
			Status:           "finished",
			Role:             "sender",
			Mode:             "webrtc",
			TotalFiles:       manifest.TotalFiles,
			CompletedFiles:   manifest.TotalFiles,
			TotalBytes:       manifest.TotalBytes,
			TransferredBytes: 0,
			Percent:          100,
			Message:          "All files already identical! P2P sync complete.",
		})
		return nil
	}

	var transferredBytes int64 = 0
	buf := make([]byte, WebRTCChunkSize)
	throttle := newProgressThrottle(100 * time.Millisecond)
	skippedFiles := 0
	sentFiles := 0

	for fileIdx, file := range filesToSend {
		if !s.checkPause() {
			return fmt.Errorf("transfer stopped by user")
		}

		fullPath := filepath.Join(s.rootDir, filepath.FromSlash(file.RelPath))
		f, err := os.Open(fullPath)
		if err != nil {
			// A file may have been deleted or locked after scanning. Skipping
			// it beats aborting the whole sync; the receiver ends its loop on
			// the completion message.
			skippedFiles++
			totalDeltaBytes -= file.Size
			if totalDeltaBytes < 0 {
				totalDeltaBytes = 0
			}
			s.updateStatus(models.SyncStats{
				Status:  "syncing",
				Role:    "sender",
				Mode:    "webrtc",
				Message: fmt.Sprintf("Skipping unreadable file %s (%v)", file.RelPath, err),
			})
			continue
		}

		startMsg, _ := json.Marshal(&ControlMessage{Type: MsgStartFile, File: &file})
		if err := s.dataChan.Send(startMsg); err != nil {
			f.Close()
			return err
		}

		for {
			if !s.checkPause() {
				f.Close()
				return fmt.Errorf("transfer stopped by user")
			}

			// Backpressure check: wait if buffered amount is high
			for s.dataChan.BufferedAmount() > 1024*1024 {
				if atomic.LoadInt32(&s.stopped) == 1 {
					f.Close()
					return fmt.Errorf("transfer stopped by user")
				}
				time.Sleep(10 * time.Millisecond)
			}

			n, readErr := f.Read(buf)
			if n > 0 {
				if err := s.dataChan.Send(buf[:n]); err != nil {
					f.Close()
					return err
				}

				transferredBytes += int64(n)
				s.speedTracker.AddBytes(int64(n))

				// Rate-limit UI events to at most 10 per second.
				if !throttle.allow() && readErr != io.EOF {
					continue
				}

				speed, _ := s.speedTracker.GetSpeed()
				percent := float64(transferredBytes) / float64(totalDeltaBytes) * 100
				if percent > 100 {
					percent = 100
				}

				s.updateStatus(models.SyncStats{
					Status:           "syncing",
					Role:             "sender",
					Mode:             "webrtc",
					TotalFiles:       totalNeededFiles,
					CompletedFiles:   fileIdx,
					TotalBytes:       totalDeltaBytes,
					TransferredBytes: transferredBytes,
					SpeedBytesPerSec: speed,
					SpeedFormatted:   models.FormatSpeed(speed),
					Percent:          percent,
					CurrentFile:      file.RelPath,
					Message:          fmt.Sprintf("P2P Syncing [%d/%d] %s (%s)", fileIdx+1, totalNeededFiles, filepath.Base(file.RelPath), models.FormatSpeed(speed)),
				})
			}

			if readErr == io.EOF {
				break
			}
			if readErr != nil {
				f.Close()
				return readErr
			}
		}
		f.Close()

		// Send end of file message
		endMsg, _ := json.Marshal(&ControlMessage{Type: MsgEndFile})
		if err := s.dataChan.Send(endMsg); err != nil {
			return err
		}

		// ACKs are batched by the receiver: waiting for one per file costs a
		// round trip per file, which dominates the transfer of instances with
		// 100k+ small files.
		sentFiles++
		if sentFiles%ackBatchSize == 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case data := <-s.msgChan:
				var ack ControlMessage
				_ = json.Unmarshal(data, &ack)
				if ack.Type == MsgError {
					return fmt.Errorf("receiver error: %s", ack.Error)
				}
				if ack.Type != MsgFileAck {
					return fmt.Errorf("unexpected ack response: %v", ack.Type)
				}
			}
		}
	}

	completeMsg, _ := json.Marshal(&ControlMessage{Type: MsgComplete})
	_ = s.dataChan.Send(completeMsg)

	finishMsg := fmt.Sprintf("P2P Hole-Punch sync completed (%s)! All files verified.", models.FormatBytes(transferredBytes))
	if skippedFiles > 0 {
		finishMsg = fmt.Sprintf("P2P sync completed (%s); skipped %d unreadable files.", models.FormatBytes(transferredBytes), skippedFiles)
	}
	s.updateStatus(models.SyncStats{
		Status:           "finished",
		Role:             "sender",
		Mode:             "webrtc",
		TotalFiles:       totalNeededFiles,
		CompletedFiles:   totalNeededFiles - skippedFiles,
		TotalBytes:       totalDeltaBytes,
		TransferredBytes: transferredBytes,
		Percent:          100,
		Message:          finishMsg,
	})

	return nil
}

// WebRTCReceiver connects to a peer via WebRTC and downloads delta files
type WebRTCReceiver struct {
	destDir        string
	peerConn       *webrtc.PeerConnection
	speedTracker   *SpeedTracker
	onProgress     func(models.SyncStats)
	stopped        int32
	dcChan         chan *webrtc.DataChannel
	openChan       chan struct{}
	msgChan        chan []byte
	openOnce       sync.Once
	symmetricNAT   int32
	relayCandidate int32
}

func NewWebRTCReceiver(destDir string, onProgress func(models.SyncStats)) *WebRTCReceiver {
	return &WebRTCReceiver{
		destDir:      destDir,
		speedTracker: NewSpeedTracker(),
		onProgress:   onProgress,
	}
}

// CreateAnswer accepts the sender's offer, configures WebRTC, and returns an Answer with gathered candidates
func (r *WebRTCReceiver) CreateAnswer(offerBase64 string) (string, error) {
	raw, err := base64.StdEncoding.DecodeString(offerBase64)
	if err != nil {
		return "", fmt.Errorf("invalid base64 offer: %w", err)
	}

	var offer webrtc.SessionDescription
	if err := json.Unmarshal(raw, &offer); err != nil {
		return "", fmt.Errorf("invalid json offer: %w", err)
	}

	pc, err := newPeerConnection(webrtc.Configuration{ICEServers: iceServers()})
	if err != nil {
		return "", err
	}
	r.peerConn = pc

	// Register handlers BEFORE negotiation so no event is ever missed.
	r.dcChan = make(chan *webrtc.DataChannel, 1)
	r.openChan = make(chan struct{})
	r.msgChan = make(chan []byte, 256)
	pc.OnDataChannel(func(dc *webrtc.DataChannel) {
		dc.OnOpen(func() {
			r.openOnce.Do(func() { close(r.openChan) })
		})
		dc.OnMessage(func(msg webrtc.DataChannelMessage) {
			r.msgChan <- msg.Data
		})
		select {
		case r.dcChan <- dc:
		default:
		}
	})
	pc.OnICEConnectionStateChange(func(state webrtc.ICEConnectionState) {
		if state == webrtc.ICEConnectionStateFailed {
			symmetric := atomic.LoadInt32(&r.symmetricNAT) == 1
			relay := atomic.LoadInt32(&r.relayCandidate) == 1
			r.updateStatus(models.SyncStats{
				Status:       "error",
				Role:         "receiver",
				Mode:         "webrtc",
				ErrorMessage: "NAT traversal failed (ICE). " + natHint(symmetric, relay),
			})
		}
	})

	if err := pc.SetRemoteDescription(offer); err != nil {
		pc.Close()
		return "", err
	}

	answer, err := pc.CreateAnswer(nil)
	if err != nil {
		pc.Close()
		return "", err
	}

	gatherComplete := webrtc.GatheringCompletePromise(pc)
	if err := pc.SetLocalDescription(answer); err != nil {
		pc.Close()
		return "", err
	}

	<-gatherComplete

	answerJSON, err := json.Marshal(pc.LocalDescription())
	if err != nil {
		return "", err
	}

	symmetric, relay := analyzeNAT(pc.LocalDescription())
	atomic.StoreInt32(&r.symmetricNAT, boolToInt32(symmetric))
	atomic.StoreInt32(&r.relayCandidate, boolToInt32(relay))

	return base64.StdEncoding.EncodeToString(answerJSON), nil
}

func (r *WebRTCReceiver) Stop() {
	atomic.StoreInt32(&r.stopped, 1)
	if r.peerConn != nil {
		_ = r.peerConn.Close()
	}
}

func (r *WebRTCReceiver) updateStatus(stats models.SyncStats) {
	if r.onProgress != nil {
		r.onProgress(stats)
	}
}

func (r *WebRTCReceiver) StartSession(ctx context.Context) error {
	if r.dcChan == nil || r.openChan == nil || r.msgChan == nil {
		return fmt.Errorf("data channel not initialised: call CreateAnswer first")
	}

	r.updateStatus(models.SyncStats{
		Status:  "waiting",
		Role:    "receiver",
		Mode:    "webrtc",
		Message: "Connecting to sender via WebRTC STUN hole-punching...",
	})

	var dc *webrtc.DataChannel
	select {
	case <-ctx.Done():
		return ctx.Err()
	case dc = <-r.dcChan:
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-r.openChan:
	}

	r.updateStatus(models.SyncStats{
		Status:  "analyzing",
		Role:    "receiver",
		Mode:    "webrtc",
		Message: "Connected! Receiving manifest...",
	})

	// Receive manifest
	var manifestMsg ControlMessage
	select {
	case <-ctx.Done():
		return ctx.Err()
	case data := <-r.msgChan:
		if err := json.Unmarshal(data, &manifestMsg); err != nil || manifestMsg.Type != MsgManifest {
			return fmt.Errorf("expected manifest message, got: %v", err)
		}
	}
	manifest := manifestMsg.Manifest

	// Plan delta
	plan, err := delta.PlanDelta(ctx, r.destDir, manifest, func(checked, total int, current string) {
		r.updateStatus(models.SyncStats{
			Status:         "analyzing",
			Role:           "receiver",
			Mode:           "webrtc",
			TotalFiles:     total,
			CompletedFiles: checked,
			CurrentFile:    current,
			Percent:        float64(checked) / float64(total) * 100,
			Message:        fmt.Sprintf("Verifying files: [%d/%d] %s", checked, total, filepath.Base(current)),
		})
	})
	if err != nil {
		return fmt.Errorf("delta planning failed: %w", err)
	}

	neededPaths := make([]string, len(plan.FilesToSync))
	for i, f := range plan.FilesToSync {
		neededPaths[i] = f.RelPath
	}

	// Safely prune extra/obsolete files inside absolute-sync folders
	prunedCount := 0
	if len(plan.FilesToDelete) > 0 {
		for _, relDel := range plan.FilesToDelete {
			allowed := false
			for _, absFolder := range manifest.AbsoluteSyncFolders {
				cleanAbs := filepath.ToSlash(filepath.Clean(absFolder))
				if cleanAbs != "" && cleanAbs != "." && (relDel == cleanAbs || strings.HasPrefix(relDel, cleanAbs+"/")) {
					allowed = true
					break
				}
			}
			if allowed {
				fullDelPath := filepath.Join(r.destDir, filepath.FromSlash(relDel))
				if os.Remove(fullDelPath) == nil {
					prunedCount++
				}
			}
		}
	}

	// Send delta request
	reqBytes, _ := json.Marshal(&ControlMessage{Type: MsgRequestDelta, DeltaReq: neededPaths})
	if err := dc.Send(reqBytes); err != nil {
		return err
	}

	if len(plan.FilesToSync) == 0 {
		finishMsg := fmt.Sprintf("All %d files already match! Saved %s.", manifest.TotalFiles, models.FormatBytes(plan.SavedBytes))
		if prunedCount > 0 {
			finishMsg = fmt.Sprintf("Pruned %d obsolete files in mirrored folders. All %d files up-to-date! (Saved %s)", prunedCount, manifest.TotalFiles, models.FormatBytes(plan.SavedBytes))
		}
		r.updateStatus(models.SyncStats{
			Status:           "finished",
			Role:             "receiver",
			Mode:             "webrtc",
			TotalFiles:       manifest.TotalFiles,
			CompletedFiles:   manifest.TotalFiles,
			PrunedFiles:      prunedCount,
			TotalBytes:       manifest.TotalBytes,
			TransferredBytes: 0,
			Percent:          100,
			Message:          finishMsg,
			MirroredFolders:  manifest.AbsoluteSyncFolders,
		})
		return nil
	}

	totalNeededFiles := len(plan.FilesToSync)
	totalDeltaBytes := plan.SyncBytes
	var transferredBytes int64 = 0
	receivedFiles := 0

	for fileIdx := 0; fileIdx < totalNeededFiles; fileIdx++ {
		if atomic.LoadInt32(&r.stopped) == 1 {
			return fmt.Errorf("transfer stopped by user")
		}

		// Wait for START_FILE
		var startMsg ControlMessage
		select {
		case <-ctx.Done():
			return ctx.Err()
		case data := <-r.msgChan:
			_ = json.Unmarshal(data, &startMsg)
		}

		if startMsg.Type == MsgComplete {
			break
		}
		if startMsg.Type != MsgStartFile || startMsg.File == nil {
			return fmt.Errorf("expected start file message")
		}

		curFile := startMsg.File
		destFilePath := filepath.Join(r.destDir, filepath.FromSlash(curFile.RelPath))

		// Read binary chunks until END_FILE
		pipeReader, pipeWriter := io.Pipe()

		go func() {
			for {
				select {
				case <-ctx.Done():
					pipeWriter.CloseWithError(ctx.Err())
					return
				case data := <-r.msgChan:
					var checkMsg ControlMessage
					if len(data) < 256 && json.Unmarshal(data, &checkMsg) == nil && checkMsg.Type == MsgEndFile {
						pipeWriter.Close()
						return
					}

					n, wErr := pipeWriter.Write(data)
					if n > 0 {
						transferredBytes += int64(n)
						r.speedTracker.AddBytes(int64(n))

						speed, _ := r.speedTracker.GetSpeed()
						percent := float64(transferredBytes) / float64(totalDeltaBytes) * 100
						if percent > 100 {
							percent = 100
						}

						r.updateStatus(models.SyncStats{
							Status:           "syncing",
							Role:             "receiver",
							Mode:             "webrtc",
							TotalFiles:       totalNeededFiles,
							CompletedFiles:   fileIdx,
							TotalBytes:       totalDeltaBytes,
							TransferredBytes: transferredBytes,
							SpeedBytesPerSec: speed,
							SpeedFormatted:   models.FormatSpeed(speed),
							Percent:          percent,
							CurrentFile:      curFile.RelPath,
							Message:          fmt.Sprintf("P2P Syncing [%d/%d] %s (%s)", fileIdx+1, totalNeededFiles, filepath.Base(curFile.RelPath), models.FormatSpeed(speed)),
						})
					}
					if wErr != nil {
						return
					}
				}
			}
		}()

		_, _, err = atomicfile.WriteAtomic(destFilePath, curFile.Hash, pipeReader)
		// Always close the read end so the chunk-pump goroutine cannot block
		// forever if the write failed before the END_FILE marker arrived.
		_ = pipeReader.Close()
		if err != nil {
			return fmt.Errorf("failed atomic write for %s: %w", curFile.RelPath, err)
		}

		// Batch ACKs to avoid one DataChannel round trip per file.
		receivedFiles++
		if receivedFiles%ackBatchSize == 0 {
			ackBytes, _ := json.Marshal(&ControlMessage{Type: MsgFileAck})
			if err := dc.Send(ackBytes); err != nil {
				return err
			}
		}
	}

	finishMsg := fmt.Sprintf("P2P Sync completed successfully (%s)! Saved %s.", models.FormatBytes(transferredBytes), models.FormatBytes(plan.SavedBytes))
	if prunedCount > 0 {
		finishMsg = fmt.Sprintf("P2P Sync completed! Pruned %d obsolete files in mirrored folders. Transferred %s (saved %s).", prunedCount, models.FormatBytes(transferredBytes), models.FormatBytes(plan.SavedBytes))
	}
	r.updateStatus(models.SyncStats{
		Status:           "finished",
		Role:             "receiver",
		Mode:             "webrtc",
		TotalFiles:       totalNeededFiles,
		CompletedFiles:   receivedFiles,
		PrunedFiles:      prunedCount,
		TotalBytes:       totalDeltaBytes,
		TransferredBytes: transferredBytes,
		Percent:          100,
		Message:          finishMsg,
		MirroredFolders:  manifest.AbsoluteSyncFolders,
	})

	return nil
}
