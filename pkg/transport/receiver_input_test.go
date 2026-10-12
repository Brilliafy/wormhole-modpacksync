package transport

import "testing"

func TestParseReceiverInput(t *testing.T) {
	cases := []struct {
		in     string
		room   string
		relay  string
		direct bool
	}{
		{in: "K7X2-M9QF", room: "K7X2-M9QF", relay: "", direct: false},
		{in: "  K7X2-M9QF@192.168.1.5:42425 ", room: "K7X2-M9QF", relay: "192.168.1.5:42425", direct: false},
		{in: "K7X2-M9QF@100.115.92.3:42425", room: "K7X2-M9QF", relay: "100.115.92.3:42425", direct: false},
		{in: "100.115.92.3", room: "100.115.92.3", relay: "", direct: true},
		{in: "100.115.92.3:42424", room: "100.115.92.3:42424", relay: "", direct: true},
		{in: "192.168.1.20@x", room: "192.168.1.20", relay: "x", direct: true},
	}

	for _, tc := range cases {
		room, relay, direct := parseReceiverInput(tc.in)
		if room != tc.room || relay != tc.relay || direct != tc.direct {
			t.Errorf("parseReceiverInput(%q) = (%q, %q, %v), want (%q, %q, %v)", tc.in, room, relay, direct, tc.room, tc.relay, tc.direct)
		}
	}
}
