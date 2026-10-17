package spike

import "testing"

// The same vector is asserted in worker/test/bridge.test.ts so the Go and
// TypeScript signers cannot drift apart.
func TestSignVector(t *testing.T) {
	got := Sign([]byte("test-secret"), "1700000000", "PUT", "/kv/spike/x", []byte("hello"))
	const want = "851cfb134e76d62584811f22cc34d0c64518d7d1e291d3e41f1fc0d2e535c9d7"
	if got != want {
		t.Fatalf("Sign = %s, want %s", got, want)
	}
}
