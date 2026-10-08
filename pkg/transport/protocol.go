package transport

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"sync"
	"time"

	"wormhole-sync/pkg/models"
)

// Message types
const (
	MsgHello        = "HELLO"
	MsgManifest     = "MANIFEST"
	MsgRequestDelta = "REQUEST_DELTA"
	MsgStartFile    = "START_FILE"
	MsgFileChunk    = "FILE_CHUNK"
	MsgEndFile      = "END_FILE"
	MsgFileAck      = "FILE_ACK"
	MsgPause        = "PAUSE"
	MsgResume       = "RESUME"
	MsgStop         = "STOP"
	MsgComplete     = "COMPLETE"
	MsgError        = "ERROR"
)

// ControlMessage is the JSON payload for coordination
type ControlMessage struct {
	Type     string               `json:"type"`
	Role     string               `json:"role,omitempty"`
	Manifest *models.SyncManifest `json:"manifest,omitempty"`
	DeltaReq []string             `json:"deltaReq,omitempty"`
	File     *models.FileInfo     `json:"file,omitempty"`
	Error    string               `json:"error,omitempty"`
}

// WriteFramedJSON encodes a JSON message with 4-byte length prefix
func WriteFramedJSON(w io.Writer, msg *ControlMessage) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	length := uint32(len(data))
	lenBuf := make([]byte, 4)
	binary.BigEndian.PutUint32(lenBuf, length)

	if _, err := w.Write(lenBuf); err != nil {
		return err
	}
	if _, err := w.Write(data); err != nil {
		return err
	}
	return nil
}

// ReadFramedJSON decodes a length-prefixed JSON message
func ReadFramedJSON(r io.Reader) (*ControlMessage, error) {
	lenBuf := make([]byte, 4)
	if _, err := io.ReadFull(r, lenBuf); err != nil {
		return nil, err
	}

	length := binary.BigEndian.Uint32(lenBuf)
	if length > 50*1024*1024 { // 50MB safety limit for control message
		return nil, fmt.Errorf("message too large: %d bytes", length)
	}

	data := make([]byte, length)
	if _, err := io.ReadFull(r, data); err != nil {
		return nil, err
	}

	var msg ControlMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		return nil, err
	}
	return &msg, nil
}

// SpeedTracker monitors transfer rate using a 1-second rolling window
type SpeedTracker struct {
	mu           sync.Mutex
	transferred  int64
	lastTime     time.Time
	lastBytes    int64
	currentSpeed float64
}

func NewSpeedTracker() *SpeedTracker {
	return &SpeedTracker{
		lastTime: time.Now(),
	}
}

func (st *SpeedTracker) AddBytes(n int64) {
	st.mu.Lock()
	defer st.mu.Unlock()
	st.transferred += n

	now := time.Now()
	elapsed := now.Sub(st.lastTime).Seconds()
	if elapsed >= 0.5 {
		deltaBytes := st.transferred - st.lastBytes
		st.currentSpeed = float64(deltaBytes) / elapsed
		st.lastTime = now
		st.lastBytes = st.transferred
	}
}

func (st *SpeedTracker) GetSpeed() (float64, int64) {
	st.mu.Lock()
	defer st.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(st.lastTime).Seconds()
	if elapsed >= 2.0 {
		st.currentSpeed = 0
	}
	return st.currentSpeed, st.transferred
}
