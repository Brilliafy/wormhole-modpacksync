package transport

import (
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"wormhole-sync/pkg/models"
)

func TestDirectRepro(t *testing.T) {
	root := os.Getenv("REAL_INSTANCE")
	if root == "" {
		t.Skip("no REAL_INSTANCE")
	}
	sel := os.Getenv("SEL")
	if sel == "" {
		sel = "config"
	}
	dest := t.TempDir()

	var mu sync.Mutex
	var last string
	start := time.Now()
	sink := func(tag string) func(s models.SyncStats) {
		return func(s models.SyncStats) {
			mu.Lock()
			defer mu.Unlock()
			msg := fmt.Sprintf("%s status=%s pct=%.1f files=%d/%d msg=%s err=%s", tag, s.Status, s.Percent, s.CompletedFiles, s.TotalFiles, s.Message, s.ErrorMessage)
			if msg != last {
				last = msg
				t.Logf("[%6.1fs] %s", time.Since(start).Seconds(), msg)
			}
		}
	}

	sender := NewSessionManager(sink("S"))
	receiver := NewSessionManager(sink("R"))

	res, err := sender.StartSenderSession(root, []string{sel}, nil, "direct")
	if err != nil {
		t.Fatalf("start sender: %v", err)
	}
	t.Logf("code=%s addrs=%v", res.Code, res.DirectAddresses)
	if len(res.DirectAddresses) == 0 {
		t.Fatal("no direct address")
	}
	if err := receiver.StartReceiverSession(dest, "direct", res.DirectAddresses[0]); err != nil {
		t.Fatalf("start receiver: %v", err)
	}

	deadline := time.Now().Add(150 * time.Second)
	for time.Now().Before(deadline) {
		s := sender.GetStats()
		r := receiver.GetStats()
		if s.Status == "finished" || s.Status == "error" || r.Status == "error" {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	mu.Lock()
	t.Logf("final sender: %+v", sender.GetStats().Status+" "+sender.GetStats().Message)
	t.Logf("final receiver: %+v", receiver.GetStats().Status+" "+receiver.GetStats().Message)
	mu.Unlock()
}
