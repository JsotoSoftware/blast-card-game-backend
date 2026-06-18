package transport

import "testing"

func TestProtocolVersion(t *testing.T) {
	if ProtocolVersion != 1 {
		t.Fatalf("got %d, want 1", ProtocolVersion)
	}
}
