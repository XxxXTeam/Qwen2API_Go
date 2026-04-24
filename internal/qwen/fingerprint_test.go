package qwen

import "testing"

func TestFingerprintForTokenIsStable(t *testing.T) {
	a := fingerprintForToken("token-a")
	b := fingerprintForToken("token-a")

	if a != b {
		t.Fatalf("fingerprint should be stable for same token: %#v != %#v", a, b)
	}
}

func TestFingerprintForTokenDiffersAcrossTokens(t *testing.T) {
	a := fingerprintForToken("token-a")
	b := fingerprintForToken("token-b")

	if a.UserAgent == b.UserAgent &&
		a.SecChUA == b.SecChUA &&
		a.AcceptLanguage == b.AcceptLanguage &&
		a.Timezone == b.Timezone {
		t.Fatalf("fingerprints should differ across tokens: %#v == %#v", a, b)
	}
}
