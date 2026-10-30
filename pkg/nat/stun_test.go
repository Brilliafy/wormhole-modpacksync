package nat

import (
	"context"
	"testing"
	"time"
)

func TestCharacterizeNATLive(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	report, err := CharacterizeNAT(ctx, nil)
	if err != nil {
		t.Fatalf("CharacterizeNAT failed: %v", err)
	}

	t.Logf("NAT Report: MappingType=%s, PublicIP=%s, Port1=%d, Port2=%d, HasStride=%v, Stride=%d",
		report.MappingType, report.PublicIP, report.Port1, report.Port2, report.HasStride, report.Stride)

	if report.PublicIP == nil {
		t.Errorf("expected non-nil PublicIP")
	}
}
