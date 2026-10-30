package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"wormhole-sync/pkg/models"
	"wormhole-sync/pkg/transport"
)

func main() {
	role := flag.String("role", "", "sender or receiver")
	mode := flag.String("mode", "webrtc", "webrtc or direct")
	dir := flag.String("dir", "", "instance / target directory")
	code := flag.String("code", "", "code or address to connect to (for receiver)")
	sel := flag.String("sel", "", "relative path to sync (for sender, empty = all files in dir)")
	flag.Parse()

	if *role != "sender" && *role != "receiver" {
		log.Fatalf("Invalid role: %s (must be sender or receiver)", *role)
	}
	if *dir == "" {
		log.Fatalf("dir is required")
	}

	absDir, err := filepath.Abs(*dir)
	if err != nil {
		log.Fatalf("filepath.Abs: %v", err)
	}

	sm := transport.NewSessionManager(func(stats models.SyncStats) {
		log.Printf("[STATS] status=%s role=%s mode=%s pct=%.1f files=%d/%d msg=%s err=%s",
			stats.Status, stats.Role, stats.Mode, stats.Percent, stats.CompletedFiles, stats.TotalFiles, stats.Message, stats.ErrorMessage)
	})

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	if *role == "sender" {
		var selectedPaths []string
		if *sel != "" {
			selectedPaths = []string{*sel}
		}
		log.Printf("Starting sender session on %s (mode=%s)...", absDir, *mode)
		res, err := sm.StartSenderSession(absDir, selectedPaths, nil, *mode)
		if err != nil {
			log.Fatalf("StartSenderSession failed: %v", err)
		}
		fmt.Printf("=== SENDER READY ===\n")
		fmt.Printf("CODE: %s\n", res.Code)
		fmt.Printf("DIRECT_ADDRS: %v\n", res.DirectAddresses)
		fmt.Printf("RELAY_SCOPE: %s\n", res.RelayScope)
		fmt.Printf("RELAY_ADDR: %s\n", res.RelayAddr)
		fmt.Printf("====================\n")
	} else {
		if *code == "" {
			log.Fatalf("code is required for receiver")
		}
		log.Printf("Starting receiver session into %s (mode=%s, code=%s)...", absDir, *mode, *code)
		if err := sm.StartReceiverSession(absDir, *mode, *code); err != nil {
			log.Fatalf("StartReceiverSession failed: %v", err)
		}
	}

	for {
		select {
		case <-sigChan:
			log.Printf("Interrupted, stopping...")
			sm.Stop()
			return
		case <-time.After(500 * time.Millisecond):
			stats := sm.GetStats()
			if stats.Status == "finished" {
				log.Printf("Session finished successfully: %s", stats.Message)
				return
			}
			if stats.Status == "error" {
				log.Printf("Session encountered error: %s", stats.ErrorMessage)
				return
			}
		}
	}
}
