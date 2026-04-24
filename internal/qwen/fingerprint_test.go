package qwen

import (
	"strings"
	"testing"
)

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

func TestFingerprintForTokenOnlyAdvertisesSupportedEncodings(t *testing.T) {
	fp := fingerprintForToken("token-a")
	if strings.Contains(fp.AcceptEncoding, "br") || strings.Contains(fp.AcceptEncoding, "zstd") {
		t.Fatalf("unsupported accept-encoding advertised: %q", fp.AcceptEncoding)
	}
	if !strings.Contains(fp.AcceptEncoding, "gzip") && !strings.Contains(fp.AcceptEncoding, "deflate") {
		t.Fatalf("expected gzip/deflate in accept-encoding, got %q", fp.AcceptEncoding)
	}
}
