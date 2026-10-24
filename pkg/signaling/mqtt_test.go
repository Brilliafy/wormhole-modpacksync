package signaling

import (
	"context"
	"os"
	"testing"
	"time"
)

// TestMQTTRendezvousLive exercises the public rendezvous against the real
// brokers. It needs internet access, so it only runs with WORMHOLE_LIVE=1.
func TestMQTTRendezvousLive(t *testing.T) {
	if os.Getenv("WORMHOLE_LIVE") == "" {
		t.Skip("set WORMHOLE_LIVE=1 to hit the public MQTT brokers")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	code := GenerateRoomCode()
	sender := NewMQTTClient(code)
	defer sender.Close()
	receiver := NewMQTTClient(code)
	defer receiver.Close()

	if err := sender.PublishOffer(ctx, code, "OFFER-BLOB", "192.168.1.5:42424"); err != nil {
		t.Fatalf("publish offer: %v", err)
	}

	offer, direct, err := receiver.FetchOffer(ctx, code)
	if err != nil {
		t.Fatalf("fetch offer: %v", err)
	}
	if offer != "OFFER-BLOB" || direct != "192.168.1.5:42424" {
		t.Fatalf("unexpected offer payload: %q %q", offer, direct)
	}

	if err := receiver.PublishAnswer(ctx, code, "ANSWER-BLOB"); err != nil {
		t.Fatalf("publish answer: %v", err)
	}

	answer, err := sender.WaitForAnswer(ctx, code)
	if err != nil {
		t.Fatalf("wait for answer: %v", err)
	}
	if answer != "ANSWER-BLOB" {
		t.Fatalf("unexpected answer: %q", answer)
	}
}

func TestRoomSignalerSelection(t *testing.T) {
	t.Setenv("WORMHOLE_RELAY", "")
	t.Setenv("WORMHOLE_SIGNAL", "")
	if _, ok := NewRoomSignaler("", "CODE").(*MQTTClient); !ok {
		t.Error("default rendezvous should be the public MQTT client")
	}
	if _, ok := NewRoomSignaler("relay.example:42425", "CODE").(*SignalingClient); !ok {
		t.Error("code override should use the HTTP relay client")
	}
	t.Setenv("WORMHOLE_RELAY", "https://relay.example:42425")
	if _, ok := NewRoomSignaler("", "CODE").(*SignalingClient); !ok {
		t.Error("WORMHOLE_RELAY should use the HTTP relay client")
	}
	t.Setenv("WORMHOLE_RELAY", "")
	t.Setenv("WORMHOLE_SIGNAL", "local")
	if _, ok := NewRoomSignaler("", "CODE").(*SignalingClient); !ok {
		t.Error("WORMHOLE_SIGNAL=local should use the embedded HTTP relay client")
	}
}
