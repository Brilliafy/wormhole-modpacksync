package signaling

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"sync"
	"time"
)

// DefaultPublicRelays provides public fallback signaling servers
var DefaultPublicRelays = []string{
	"https://wormhole-relay.fly.dev",
	"http://relay.wormhole-sync.internal",
}

// GenerateRoomCode generates a friendly, memorable 6-digit or word code
func GenerateRoomCode() string {
	n, err := rand.Int(rand.Reader, big.NewInt(900000))
	if err != nil {
		return fmt.Sprintf("%d", time.Now().Unix()%1000000)
	}
	return fmt.Sprintf("%06d", n.Int64()+100000)
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
