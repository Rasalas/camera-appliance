package auth

import (
	"encoding/hex"
	"testing"
)

func TestPBKDF2KnownVectors(t *testing.T) {
	for _, v := range []struct {
		rounds int
		want   string
	}{
		{1, "120fb6cffcf8b32c43e7225256c4f837a86548c92ccc35480805987cb70be17b"},
		{2, "ae4d0c95af6b46d32d0adff928f06dd02a303f8ef3c251dfd6e2d85a95474c43"},
		{4096, "c5e478d59288c841aa530db6845c4c8d962893a001ce4e11a4963873aa98134a"},
	} {
		if got := hex.EncodeToString(pbkdf2SHA256([]byte("password"), []byte("salt"), v.rounds, 32)); got != v.want {
			t.Fatalf("round %d: %s", v.rounds, got)
		}
	}
}
func TestPasswordCompatibilityAndBounds(t *testing.T) {
	// Existing PBKDF2 encoding, generated independently with Python hashlib.
	const old = "pbkdf2_sha256$210000$MDEyMzQ1Njc4OWFiY2RlZg$AwqCaxvQwn7NEOs/S/+UedRqk+7rp55KztDk01x3DQc"
	if !VerifyPassword(old, "password") || VerifyPassword(old, "wrong") {
		t.Fatal("legacy hash verification")
	}
	hash, err := HashPassword("password")
	if err != nil {
		t.Fatal(err)
	}
	if !VerifyPassword(hash, "password") || VerifyPassword(hash, "wrong") {
		t.Fatal("roundtrip")
	}
	for _, bad := range []string{"pbkdf2_sha256$1000001$MDEyMzQ1Njc4OWFiY2RlZg$eA", "pbkdf2_sha256$210000$eA$eA"} {
		if VerifyPassword(bad, "password") {
			t.Fatal("accepted invalid hash")
		}
	}
}
func BenchmarkPasswordHash(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		if _, err := HashPassword("benchmark-password"); err != nil {
			b.Fatal(err)
		}
	}
}

func TestPasswordWorkDoesNotAllocatePerRound(t *testing.T) {
	allocations := testing.AllocsPerRun(1, func() { pbkdf2SHA256([]byte("password"), []byte("0123456789abcdef"), 1000, 32) })
	if allocations > 50 {
		t.Fatalf("allocations grow with rounds: %.0f", allocations)
	}
}
