package transport

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"

	"wormhole-sync/pkg/atomicfile"
	"wormhole-sync/pkg/delta"
	"wormhole-sync/pkg/models"
)

const ChunkSize = 64 * 1024

// DirectSender runs a TCP server waiting for the receiver to connect
type DirectSender struct {
	mu           sync.Mutex
	listener     net.Listener
	conn         net.Conn
	port         int
	manifest     *models.SyncManifest
	rootDir      string
	speedTracker *SpeedTracker
	paused       int32
	stopped      int32
	pauseCond    *sync.Cond
	pauseMu      sync.Mutex
	onProgress   func(stats models.SyncStats)
}

func NewDirectSender(rootDir string, manifest *models.SyncManifest, port int, onProgress func(models.SyncStats)) *DirectSender {
	s := &DirectSender{
		rootDir:      rootDir,
		manifest:     manifest,
		port:         port,
		speedTracker: NewSpeedTracker(),
		onProgress:   onProgress,
	}
	s.pauseCond = sync.NewCond(&s.pauseMu)
	return s
}

func (s *DirectSender) Start(ctx context.Context) error {
	addr := fmt.Sprintf("0.0.0.0:%d", s.port)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		// If port is occupied, try dynamic port
		ln, err = net.Listen("tcp", "0.0.0.0:0")
		if err != nil {
			return fmt.Errorf("failed to listen on tcp: %w", err)
		}
	}
	s.listener = ln
	s.port = ln.Addr().(*net.TCPAddr).Port

	s.updateStatus(models.SyncStats{
		Status:  "waiting",
		Role:    "sender",
		Mode:    "direct",
		Message: fmt.Sprintf("Waiting for friend to connect on port %d...", s.port),
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
		s.conn = conn
		defer conn.Close()
		defer ln.Close()
		return s.handleSenderSession(ctx, conn)
	}
}

func (s *DirectSender) Port() int {
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
	if s.conn != nil {
		_ = s.conn.Close()
	}
	if s.listener != nil {
		_ = s.listener.Close()
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

	// Send Manifest
	if err := WriteFramedJSON(conn, &ControlMessage{Type: MsgManifest, Manifest: s.manifest}); err != nil {
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
	for _, f := range s.manifest.Files {
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
			TotalFiles:       s.manifest.TotalFiles,
			CompletedFiles:   s.manifest.TotalFiles,
			TotalBytes:       s.manifest.TotalBytes,
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

	for fileIdx, file := range filesToSend {
		if !s.checkPause() {
			return fmt.Errorf("transfer stopped by user")
		}

		fullPath := filepath.Join(s.rootDir, filepath.FromSlash(file.RelPath))
		f, err := os.Open(fullPath)
		if err != nil {
			return fmt.Errorf("failed opening file %s: %w", file.RelPath, err)
		}

		// Send START_FILE
		if err := WriteFramedJSON(conn, &ControlMessage{Type: MsgStartFile, File: &file}); err != nil {
			f.Close()
			return err
		}

		var fileTransferred int64 = 0
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

				fileTransferred += int64(n)
				transferredBytes += int64(n)
				s.speedTracker.AddBytes(int64(n))

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

		// Wait for ACK from receiver
		ackMsg, err := ReadFramedJSON(conn)
		if err != nil || ackMsg.Type != MsgFileAck {
			return fmt.Errorf("failed waiting for file ack: %v", err)
		}
	}

	// Notify completed
	_ = WriteFramedJSON(conn, &ControlMessage{Type: MsgComplete})

	s.updateStatus(models.SyncStats{
		Status:           "finished",
		Role:             "sender",
		Mode:             "direct",
		PeerAddress:      peerAddr,
		TotalFiles:       totalNeededFiles,
		CompletedFiles:   totalNeededFiles,
		TotalBytes:       totalDeltaBytes,
		TransferredBytes: transferredBytes,
		Percent:          100,
		Message:          fmt.Sprintf("Finished syncing %d files (%s) successfully!", totalNeededFiles, models.FormatBytes(transferredBytes)),
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

	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp", r.addr)
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
	plan, err := delta.PlanDelta(r.destDir, manifest, func(checked, total int, current string) {
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

	// Send delta request
	if err := WriteFramedJSON(conn, &ControlMessage{Type: MsgRequestDelta, DeltaReq: neededPaths}); err != nil {
		return err
	}

	if len(plan.FilesToSync) == 0 {
		// All files already present and identical!
		completeMsg, _ := ReadFramedJSON(conn)
		_ = completeMsg
		r.updateStatus(models.SyncStats{
			Status:           "finished",
			Role:             "receiver",
			Mode:             "direct",
			PeerAddress:      peerAddr,
			TotalFiles:       manifest.TotalFiles,
			CompletedFiles:   manifest.TotalFiles,
			TotalBytes:       manifest.TotalBytes,
			TransferredBytes: 0,
			Percent:          100,
			Message:          fmt.Sprintf("All %d files already match! Zero bytes downloaded (Saved %s).", manifest.TotalFiles, models.FormatBytes(plan.SavedBytes)),
		})
		return nil
	}

	totalNeededFiles := len(plan.FilesToSync)
	totalDeltaBytes := plan.SyncBytes
	var transferredBytes int64 = 0

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

		// Send ACK to sender
		if err := WriteFramedJSON(conn, &ControlMessage{Type: MsgFileAck}); err != nil {
			return err
		}
	}

	// Final complete message
	r.updateStatus(models.SyncStats{
		Status:           "finished",
		Role:             "receiver",
		Mode:             "direct",
		PeerAddress:      peerAddr,
		TotalFiles:       totalNeededFiles,
		CompletedFiles:   totalNeededFiles,
		TotalBytes:       totalDeltaBytes,
		TransferredBytes: transferredBytes,
		Percent:          100,
		Message:          fmt.Sprintf("Sync complete! Transferred %s (saved %s).", models.FormatBytes(transferredBytes), models.FormatBytes(plan.SavedBytes)),
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
