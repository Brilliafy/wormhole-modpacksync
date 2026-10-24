package transport

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"wormhole-sync/pkg/atomicfile"
	"wormhole-sync/pkg/delta"
	"wormhole-sync/pkg/models"
)

const ChunkSize = 64 * 1024

// ackBatchSize is how many files the receiver acknowledges at once. One ACK per
// file adds a network round trip per file, which dominates transfer time when a
// modpack contains 100k+ tiny files.
const ackBatchSize = 32

// DirectSender runs a TCP server waiting for the receiver to connect
type DirectSender struct {
	mu               sync.Mutex
	listener         net.Listener
	conn             net.Conn
	port             int
	manifestProvider func() *models.SyncManifest
	rootDir          string
	speedTracker     *SpeedTracker
	paused           int32
	stopped          int32
	pauseCond        *sync.Cond
	pauseMu          sync.Mutex
	onProgress       func(stats models.SyncStats)
}

// NewDirectSender creates a sender. The manifest is fetched through
// manifestProvider at the moment the peer handshake completes, which lets the
// TCP listener bind and the share code appear while files are still being
// hashed in the background.
func NewDirectSender(rootDir string, manifestProvider func() *models.SyncManifest, port int, onProgress func(models.SyncStats)) *DirectSender {
	s := &DirectSender{
		rootDir:          rootDir,
		manifestProvider: manifestProvider,
		port:             port,
		speedTracker:     NewSpeedTracker(),
		onProgress:       onProgress,
	}
	s.pauseCond = sync.NewCond(&s.pauseMu)
	return s
}

// Listen binds the sender's TCP listener. If the requested address is busy it
// falls back to a dynamic port. The resolved port is exposed via Port().
func (s *DirectSender) Listen(addr string) error {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		ln, err = net.Listen("tcp", "0.0.0.0:0")
		if err != nil {
			return fmt.Errorf("failed to listen on tcp: %w", err)
		}
	}
	s.mu.Lock()
	s.listener = ln
	s.port = ln.Addr().(*net.TCPAddr).Port
	s.mu.Unlock()
	return nil
}

func (s *DirectSender) Start(ctx context.Context) error {
	s.mu.Lock()
	if s.listener == nil {
		s.mu.Unlock()
		if err := s.Listen(fmt.Sprintf("0.0.0.0:%d", s.port)); err != nil {
			return err
		}
		s.mu.Lock()
	}
	ln := s.listener
	port := s.port
	s.mu.Unlock()

	s.updateStatus(models.SyncStats{
		Status:  "waiting",
		Role:    "sender",
		Mode:    "direct",
		Message: fmt.Sprintf("Waiting for friend to connect on port %d...", port),
	})

	// Wait for receiver connection
	connChan := make(chan net.Conn, 1)
	errChan := make(chan error, 1)

	go func() {
		conn, err := ln.Accept()
		if err != nil {
			errChan <- err
			return
		}
		connChan <- conn
	}()

	select {
	case <-ctx.Done():
		ln.Close()
		return ctx.Err()
	case err := <-errChan:
		return err
	case conn := <-connChan:
		s.mu.Lock()
		s.conn = conn
		s.mu.Unlock()
		defer conn.Close()
		defer ln.Close()
		return s.handleSenderSession(ctx, conn)
	}
}

func (s *DirectSender) Port() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.port
}

func (s *DirectSender) Pause() {
	atomic.StoreInt32(&s.paused, 1)
	s.updateStatus(models.SyncStats{
		Status:  "paused",
		Role:    "sender",
		Mode:    "direct",
		Message: "Transfer paused",
	})
}

func (s *DirectSender) Resume() {
	atomic.StoreInt32(&s.paused, 0)
	s.pauseCond.Broadcast()
	s.updateStatus(models.SyncStats{
		Status:  "syncing",
		Role:    "sender",
		Mode:    "direct",
		Message: "Resuming transfer...",
	})
}

func (s *DirectSender) Stop() {
	atomic.StoreInt32(&s.stopped, 1)
	atomic.StoreInt32(&s.paused, 0)
	s.pauseCond.Broadcast()
	s.mu.Lock()
	conn := s.conn
	listener := s.listener
	s.mu.Unlock()
	if conn != nil {
		_ = conn.Close()
	}
	if listener != nil {
		_ = listener.Close()
	}
}

func (s *DirectSender) checkPause() bool {
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

func (s *DirectSender) updateStatus(stats models.SyncStats) {
	if s.onProgress != nil {
		s.onProgress(stats)
	}
}

func (s *DirectSender) handleSenderSession(ctx context.Context, conn net.Conn) error {
	peerAddr := conn.RemoteAddr().String()
	s.updateStatus(models.SyncStats{
		Status:      "connecting",
		Role:        "sender",
		Mode:        "direct",
		PeerAddress: peerAddr,
		Message:     "Connected to peer. Exchanging manifest...",
	})

	// Handshake
	if err := WriteFramedJSON(conn, &ControlMessage{Type: MsgHello, Role: "sender"}); err != nil {
		return err
	}

	peerMsg, err := ReadFramedJSON(conn)
	if err != nil || peerMsg.Type != MsgHello {
		return fmt.Errorf("failed peer hello handshake: %v", err)
	}

	// Send Manifest. The provider blocks until background hashing finishes,
	// so the listener and share code are live while scanning still runs.
	s.updateStatus(models.SyncStats{
		Status:      "analyzing",
		Role:        "sender",
		Mode:        "direct",
		PeerAddress: peerAddr,
		Message:     "Finishing file preparation (hashing)...",
	})
	manifest := s.manifestProvider()
	if manifest == nil {
		return fmt.Errorf("manifest preparation failed or was cancelled")
	}

	if err := WriteFramedJSON(conn, &ControlMessage{Type: MsgManifest, Manifest: manifest}); err != nil {
		return err
	}

	s.updateStatus(models.SyncStats{
		Status:      "analyzing",
		Role:        "sender",
		Mode:        "direct",
		PeerAddress: peerAddr,
		Message:     "Friend is analyzing existing local files to calculate delta...",
	})

	// Wait for receiver's delta request
	deltaMsg, err := ReadFramedJSON(conn)
	if err != nil || deltaMsg.Type != MsgRequestDelta {
		return fmt.Errorf("failed receiving delta request: %v", err)
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
		// Nothing to send, already identical!
		_ = WriteFramedJSON(conn, &ControlMessage{Type: MsgComplete})
		s.updateStatus(models.SyncStats{
			Status:           "finished",
			Role:             "sender",
			Mode:             "direct",
			PeerAddress:      peerAddr,
			TotalFiles:       manifest.TotalFiles,
			CompletedFiles:   manifest.TotalFiles,
			TotalBytes:       manifest.TotalBytes,
			TransferredBytes: 0,
			Percent:          100,
			Message:          "All files were already up-to-date! Delta sync complete.",
		})
		return nil
	}

	s.updateStatus(models.SyncStats{
		Status:           "syncing",
		Role:             "sender",
		Mode:             "direct",
		PeerAddress:      peerAddr,
		TotalFiles:       totalNeededFiles,
		CompletedFiles:   0,
		TotalBytes:       totalDeltaBytes,
		TransferredBytes: 0,
		Percent:          0,
		Message:          fmt.Sprintf("Sending %d delta files (%s)...", totalNeededFiles, models.FormatBytes(totalDeltaBytes)),
	})

	var transferredBytes int64 = 0
	buf := make([]byte, ChunkSize)
	throttle := newProgressThrottle(100 * time.Millisecond)

	// ACKs arrive in batches now. Waiting for one ACK per file used to cost a
	// full network round trip per file: on Tailscale (~30ms RTT) a modpack with
	// 126k files spent an hour doing nothing but ACK round trips. Drain them in
	// the background and only surface fatal receiver errors.
	recvErr := make(chan error, 1)
	go func() {
		for {
			msg, err := ReadFramedJSON(conn)
			if err != nil {
				return
			}
			if msg.Type == MsgError {
				select {
				case recvErr <- fmt.Errorf("receiver error: %s", msg.Error):
				default:
				}
				return
			}
		}
	}()

	skippedFiles := 0
	for fileIdx, file := range filesToSend {
		select {
		case err := <-recvErr:
			return err
		default:
		}

		if !s.checkPause() {
			return fmt.Errorf("transfer stopped by user")
		}

		fullPath := filepath.Join(s.rootDir, filepath.FromSlash(file.RelPath))
		f, err := os.Open(fullPath)
		if err != nil {
			// A file may have been deleted or locked after scanning. Skipping
			// it beats aborting the whole sync; the receiver simply never
			// receives its START_FILE and the completion message ends the loop.
			skippedFiles++
			totalDeltaBytes -= file.Size
			if totalDeltaBytes < 0 {
				totalDeltaBytes = 0
			}
			s.updateStatus(models.SyncStats{
				Status:      "syncing",
				Role:        "sender",
				Mode:        "direct",
				PeerAddress: peerAddr,
				Message:     fmt.Sprintf("Skipping unreadable file %s (%v)", file.RelPath, err),
			})
			continue
		}

		// Send START_FILE
		if err := WriteFramedJSON(conn, &ControlMessage{Type: MsgStartFile, File: &file}); err != nil {
			f.Close()
			return err
		}

		for {
			if !s.checkPause() {
				f.Close()
				return fmt.Errorf("transfer stopped by user")
			}

			n, readErr := f.Read(buf)
			if n > 0 {
				chunkLenBuf := make([]byte, 4)
				binary.BigEndian.PutUint32(chunkLenBuf, uint32(n))
				if _, err := conn.Write(chunkLenBuf); err != nil {
					f.Close()
					return err
				}
				if _, err := conn.Write(buf[:n]); err != nil {
					f.Close()
					return err
				}

				transferredBytes += int64(n)
				s.speedTracker.AddBytes(int64(n))

				// Rate-limit UI events: only emit at most 10 per second, or
				// immediately when a file completes.
				fileDone := readErr == io.EOF
				if !throttle.allow() && !fileDone {
					if readErr != nil && readErr != io.EOF {
						f.Close()
						return readErr
					}
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
					Mode:             "direct",
					PeerAddress:      peerAddr,
					TotalFiles:       totalNeededFiles,
					CompletedFiles:   fileIdx,
					TotalBytes:       totalDeltaBytes,
					TransferredBytes: transferredBytes,
					SpeedBytesPerSec: speed,
					SpeedFormatted:   models.FormatSpeed(speed),
					Percent:          percent,
					CurrentFile:      file.RelPath,
					Message:          fmt.Sprintf("Syncing [%d/%d] %s (%s)", fileIdx+1, totalNeededFiles, filepath.Base(file.RelPath), models.FormatSpeed(speed)),
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

		// Write zero length chunk to signal EOF for this file
		zeroLen := make([]byte, 4)
		if _, err := conn.Write(zeroLen); err != nil {
			return err
		}
	}

	// Notify completed
	_ = WriteFramedJSON(conn, &ControlMessage{Type: MsgComplete})

	finishMsg := fmt.Sprintf("Finished syncing %d files (%s) successfully!", totalNeededFiles-skippedFiles, models.FormatBytes(transferredBytes))
	if skippedFiles > 0 {
		finishMsg = fmt.Sprintf("Finished syncing %d files (%s); skipped %d unreadable.", totalNeededFiles-skippedFiles, models.FormatBytes(transferredBytes), skippedFiles)
	}
	s.updateStatus(models.SyncStats{
		Status:           "finished",
		Role:             "sender",
		Mode:             "direct",
		PeerAddress:      peerAddr,
		TotalFiles:       totalNeededFiles,
		CompletedFiles:   totalNeededFiles - skippedFiles,
		TotalBytes:       totalDeltaBytes,
		TransferredBytes: transferredBytes,
		Percent:          100,
		Message:          finishMsg,
	})

	return nil
}

// DirectReceiver connects to a sender over TCP and receives delta files
type DirectReceiver struct {
	destDir      string
	addr         string
	speedTracker *SpeedTracker
	onProgress   func(models.SyncStats)
	conn         net.Conn
	stopped      int32
}

func NewDirectReceiver(destDir string, addr string, onProgress func(models.SyncStats)) *DirectReceiver {
	return &DirectReceiver{
		destDir:      destDir,
		addr:         addr,
		speedTracker: NewSpeedTracker(),
		onProgress:   onProgress,
	}
}

func (r *DirectReceiver) Stop() {
	atomic.StoreInt32(&r.stopped, 1)
	if r.conn != nil {
		_ = r.conn.Close()
	}
}

func (r *DirectReceiver) updateStatus(stats models.SyncStats) {
	if r.onProgress != nil {
		r.onProgress(stats)
	}
}

func (r *DirectReceiver) Start(ctx context.Context) error {
	r.updateStatus(models.SyncStats{
		Status:  "connecting",
		Role:    "receiver",
		Mode:    "direct",
		Message: fmt.Sprintf("Connecting to %s...", r.addr),
	})

	// Bound the dial so a wrong or unreachable address fails with a clear
	// error instead of hanging on TCP SYN retries for minutes.
	dialCtx, cancelDial := context.WithTimeout(ctx, 30*time.Second)
	defer cancelDial()
	var d net.Dialer
	conn, err := d.DialContext(dialCtx, "tcp", r.addr)
	if err != nil {
		r.updateStatus(models.SyncStats{
			Status:       "error",
			Role:         "receiver",
			Mode:         "direct",
			ErrorMessage: fmt.Sprintf("Failed to connect to %s: %v", r.addr, err),
		})
		return err
	}
	r.conn = conn
	defer conn.Close()

	peerAddr := conn.RemoteAddr().String()

	// Handshake
	peerMsg, err := ReadFramedJSON(conn)
	if err != nil || peerMsg.Type != MsgHello {
		return fmt.Errorf("failed peer hello handshake: %v", err)
	}

	if err := WriteFramedJSON(conn, &ControlMessage{Type: MsgHello, Role: "receiver"}); err != nil {
		return err
	}

	// Receive Manifest
	manifestMsg, err := ReadFramedJSON(conn)
	if err != nil || manifestMsg.Type != MsgManifest || manifestMsg.Manifest == nil {
		return fmt.Errorf("failed receiving manifest: %v", err)
	}
	manifest := manifestMsg.Manifest

	r.updateStatus(models.SyncStats{
		Status:      "analyzing",
		Role:        "receiver",
		Mode:        "direct",
		PeerAddress: peerAddr,
		Message:     "Analyzing existing local files and calculating hashes...",
		TotalFiles:  manifest.TotalFiles,
		TotalBytes:  manifest.TotalBytes,
	})

	// Plan delta
	plan, err := delta.PlanDelta(ctx, r.destDir, manifest, func(checked, total int, current string) {
		r.updateStatus(models.SyncStats{
			Status:         "analyzing",
			Role:           "receiver",
			Mode:           "direct",
			PeerAddress:    peerAddr,
			TotalFiles:     total,
			CompletedFiles: checked,
			CurrentFile:    current,
			Percent:        float64(checked) / float64(total) * 100,
			Message:        fmt.Sprintf("Verifying existing files: [%d/%d] %s", checked, total, filepath.Base(current)),
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
	if err := WriteFramedJSON(conn, &ControlMessage{Type: MsgRequestDelta, DeltaReq: neededPaths}); err != nil {
		return err
	}

	if len(plan.FilesToSync) == 0 {
		// All files already present and identical!
		completeMsg, _ := ReadFramedJSON(conn)
		_ = completeMsg
		finishMsg := fmt.Sprintf("All %d files already match! Zero bytes downloaded (Saved %s).", manifest.TotalFiles, models.FormatBytes(plan.SavedBytes))
		if prunedCount > 0 {
			finishMsg = fmt.Sprintf("Pruned %d obsolete files in mirrored folders. All %d files up-to-date! (Saved %s)", prunedCount, manifest.TotalFiles, models.FormatBytes(plan.SavedBytes))
		}
		r.updateStatus(models.SyncStats{
			Status:           "finished",
			Role:             "receiver",
			Mode:             "direct",
			PeerAddress:      peerAddr,
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
	throttle := newProgressThrottle(100 * time.Millisecond)

	r.updateStatus(models.SyncStats{
		Status:           "syncing",
		Role:             "receiver",
		Mode:             "direct",
		PeerAddress:      peerAddr,
		TotalFiles:       totalNeededFiles,
		CompletedFiles:   0,
		TotalBytes:       totalDeltaBytes,
		TransferredBytes: 0,
		Percent:          0,
		Message:          fmt.Sprintf("Receiving %d files (%s, saved %s)...", totalNeededFiles, models.FormatBytes(totalDeltaBytes), models.FormatBytes(plan.SavedBytes)),
	})

	for fileIdx := 0; fileIdx < totalNeededFiles; fileIdx++ {
		if atomic.LoadInt32(&r.stopped) == 1 {
			return fmt.Errorf("transfer stopped by user")
		}

		msg, err := ReadFramedJSON(conn)
		if err != nil {
			return fmt.Errorf("failed reading file header: %w", err)
		}

		if msg.Type == MsgComplete {
			break
		}
		if msg.Type != MsgStartFile || msg.File == nil {
			return fmt.Errorf("unexpected message: %v", msg.Type)
		}

		curFile := msg.File
		destFilePath := filepath.Join(r.destDir, filepath.FromSlash(curFile.RelPath))

		// Stream chunks into temporary file with atomic rename and hash verification
		chunkReader := &networkChunkReader{
			conn:         conn,
			speedTracker: r.speedTracker,
			onProgress: func(bytesRead int) {
				transferredBytes += int64(bytesRead)
				// Rate-limit UI events to at most 10 per second.
				if !throttle.allow() {
					return
				}
				speed, _ := r.speedTracker.GetSpeed()
				percent := float64(transferredBytes) / float64(totalDeltaBytes) * 100
				if percent > 100 {
					percent = 100
				}
				r.updateStatus(models.SyncStats{
					Status:           "syncing",
					Role:             "receiver",
					Mode:             "direct",
					PeerAddress:      peerAddr,
					TotalFiles:       totalNeededFiles,
					CompletedFiles:   fileIdx,
					TotalBytes:       totalDeltaBytes,
					TransferredBytes: transferredBytes,
					SpeedBytesPerSec: speed,
					SpeedFormatted:   models.FormatSpeed(speed),
					Percent:          percent,
					CurrentFile:      curFile.RelPath,
					Message:          fmt.Sprintf("Syncing [%d/%d] %s (%s)", fileIdx+1, totalNeededFiles, filepath.Base(curFile.RelPath), models.FormatSpeed(speed)),
				})
			},
		}

		_, _, err = atomicfile.WriteAtomic(destFilePath, curFile.Hash, chunkReader)
		if err != nil {
			return fmt.Errorf("failed atomic write for %s: %w", curFile.RelPath, err)
		}

		// Batch ACKs: one per file costs a round trip per file, which dominates
		// transfer time for instances with tens of thousands of small files.
		receivedFiles++
		if receivedFiles%ackBatchSize == 0 {
			if err := WriteFramedJSON(conn, &ControlMessage{Type: MsgFileAck}); err != nil {
				return err
			}
		}
	}

	// Final complete message
	finishMsg := fmt.Sprintf("Sync complete! Transferred %s (saved %s).", models.FormatBytes(transferredBytes), models.FormatBytes(plan.SavedBytes))
	if prunedCount > 0 {
		finishMsg = fmt.Sprintf("Sync complete! Pruned %d obsolete files in mirrored folders. Transferred %s (saved %s).", prunedCount, models.FormatBytes(transferredBytes), models.FormatBytes(plan.SavedBytes))
	}
	r.updateStatus(models.SyncStats{
		Status:           "finished",
		Role:             "receiver",
		Mode:             "direct",
		PeerAddress:      peerAddr,
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

// networkChunkReader reads chunked frames from TCP connection until zero-length chunk
type networkChunkReader struct {
	conn         net.Conn
	speedTracker *SpeedTracker
	onProgress   func(n int)
	currChunkLen int
	chunkRead    int
	eofReached   bool
}

func (r *networkChunkReader) Read(p []byte) (int, error) {
	if r.eofReached {
		return 0, io.EOF
	}

	if r.chunkRead >= r.currChunkLen {
		lenBuf := make([]byte, 4)
		if _, err := io.ReadFull(r.conn, lenBuf); err != nil {
			return 0, err
		}
		chunkLen := int(binary.BigEndian.Uint32(lenBuf))
		if chunkLen == 0 {
			r.eofReached = true
			return 0, io.EOF
		}
		r.currChunkLen = chunkLen
		r.chunkRead = 0
	}

	toRead := len(p)
	remainingInChunk := r.currChunkLen - r.chunkRead
	if toRead > remainingInChunk {
		toRead = remainingInChunk
	}

	n, err := r.conn.Read(p[:toRead])
	if n > 0 {
		r.chunkRead += n
		if r.speedTracker != nil {
			r.speedTracker.AddBytes(int64(n))
		}
		if r.onProgress != nil {
			r.onProgress(n)
		}
	}
	return n, err
}
