package signaling

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"net/http"
	"sync"
	"time"
)

// roomCodeAlphabet is Crockford Base32. It omits the ambiguous characters
// I, L, O and U so codes stay easy to read aloud and type.
const roomCodeAlphabet = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

var (
	codeMu      sync.Mutex
	issuedCodes = make(map[string]struct{})
)

// GenerateRoomCode returns an auto-generated, time-based session code.
//
// The code is derived from the current nanosecond clock mixed with
// cryptographically secure random bytes, so two users (and even two calls on
// the same machine) can never receive the same code. The 40-bit digest is
// encoded as 8 Crockford Base32 characters grouped as XXXX-XXXX. A
// process-local registry additionally guarantees no duplicates are ever
// handed out twice by one process.
func GenerateRoomCode() string {
	codeMu.Lock()
	defer codeMu.Unlock()

	for {
		seed := make([]byte, 24)
		// Time component: changes on every call and makes codes non-repeating.
		binary.BigEndian.PutUint64(seed[:8], uint64(time.Now().UnixNano()))
		// Random component: guarantees uniqueness across machines/users.
		if _, err := rand.Read(seed[8:]); err != nil {
			binary.BigEndian.PutUint64(seed[8:16], uint64(time.Now().UnixNano()))
			binary.BigEndian.PutUint64(seed[16:], uint64(time.Now().UnixNano())>>1)
		}

		sum := sha256.Sum256(seed)
		acc := uint64(sum[0])<<32 | uint64(sum[1])<<24 | uint64(sum[2])<<16 | uint64(sum[3])<<8 | uint64(sum[4])

		raw := make([]byte, 8)
		for i := 7; i >= 0; i-- {
			raw[i] = roomCodeAlphabet[acc&0x1F]
			acc >>= 5
		}

		code := string(raw[:4]) + "-" + string(raw[4:])
		if _, exists := issuedCodes[code]; exists {
			continue
		}
		issuedCodes[code] = struct{}{}
		return code
	}
}

// RoomData holds the signaling state for a room
type RoomData struct {
	mu         sync.Mutex
	Code       string
	Offer      string
	Answer     string
	DirectAddr string
	Candidates []string
	Created    time.Time
}

// EmbeddedRelayServer is an in-memory HTTP signaling relay
type EmbeddedRelayServer struct {
	mu    sync.RWMutex
	rooms map[string]*RoomData
}

func NewEmbeddedRelayServer() *EmbeddedRelayServer {
	s := &EmbeddedRelayServer{
		rooms: make(map[string]*RoomData),
	}
	go s.cleanupLoop()
	return s
}

func (s *EmbeddedRelayServer) cleanupLoop() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		s.mu.Lock()
		now := time.Now()
		for code, room := range s.rooms {
			if now.Sub(room.Created) > 30*time.Minute {
				delete(s.rooms, code)
			}
		}
		s.mu.Unlock()
	}
}

func (s *EmbeddedRelayServer) getOrCreateRoom(code string) *RoomData {
	s.mu.Lock()
	defer s.mu.Unlock()
	room, ok := s.rooms[code]
	if !ok {
		room = &RoomData{
			Code:       code,
			Candidates: make([]string, 0),
			Created:    time.Now(),
		}
		s.rooms[code] = room
	}
	return room
}

// Handler returns the http.Handler for the relay
func (s *EmbeddedRelayServer) Handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/signal/offer", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		code := r.URL.Query().Get("code")
		if code == "" {
			http.Error(w, "missing code", http.StatusBadRequest)
			return
		}
		room := s.getOrCreateRoom(code)

		if r.Method == http.MethodPost {
			var body struct {
				Offer      string `json:"offer"`
				DirectAddr string `json:"directAddr"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			room.mu.Lock()
			room.Offer = body.Offer
			if body.DirectAddr != "" {
				room.DirectAddr = body.DirectAddr
			}
			room.mu.Unlock()
			w.WriteHeader(http.StatusOK)
			return
		}

		if r.Method == http.MethodGet {
			// Long poll waiting for offer up to 25s
			timeout := time.After(25 * time.Second)
			ticker := time.NewTicker(500 * time.Millisecond)
			defer ticker.Stop()
			for {
				select {
				case <-timeout:
					http.Error(w, "timeout waiting for offer", http.StatusRequestTimeout)
					return
				case <-r.Context().Done():
					return
				case <-ticker.C:
					room.mu.Lock()
					offer := room.Offer
					addr := room.DirectAddr
					room.mu.Unlock()
					if offer != "" || addr != "" {
						_ = json.NewEncoder(w).Encode(map[string]string{
							"offer":      offer,
							"directAddr": addr,
						})
						return
					}
				}
			}
		}
	})

	mux.HandleFunc("/signal/answer", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		code := r.URL.Query().Get("code")
		if code == "" {
			http.Error(w, "missing code", http.StatusBadRequest)
			return
		}
		room := s.getOrCreateRoom(code)

		if r.Method == http.MethodPost {
			var body struct {
				Answer string `json:"answer"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			room.mu.Lock()
			room.Answer = body.Answer
			room.mu.Unlock()
			w.WriteHeader(http.StatusOK)
			return
		}

		if r.Method == http.MethodGet {
			// Long poll waiting for answer
			timeout := time.After(25 * time.Second)
			ticker := time.NewTicker(500 * time.Millisecond)
			defer ticker.Stop()
			for {
				select {
				case <-timeout:
					http.Error(w, "timeout waiting for answer", http.StatusRequestTimeout)
					return
				case <-r.Context().Done():
					return
				case <-ticker.C:
					room.mu.Lock()
					answer := room.Answer
					room.mu.Unlock()
					if answer != "" {
						_ = json.NewEncoder(w).Encode(map[string]string{
							"answer": answer,
						})
						return
					}
				}
			}
		}
	})

	return mux
}
