package selfupdate

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

func TestChecksumForAndVerify(t *testing.T) {
	t.Parallel()
	data := []byte("hello pine")
	sum := sha256.Sum256(data)
	hexSum := hex.EncodeToString(sum[:])
	body := hexSum + "  pine_0.7.1_darwin_arm64.tar.gz\n" +
		"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa  other.tar.gz\n"
	got, err := ChecksumFor(body, "pine_0.7.1_darwin_arm64.tar.gz")
	if err != nil {
		t.Fatal(err)
	}
	if got != hexSum {
		t.Fatalf("got %s", got)
	}
	if err := VerifySHA256(data, hexSum); err != nil {
		t.Fatal(err)
	}
	if err := VerifySHA256(data, "deadbeef"); err == nil {
		t.Fatal("expected mismatch")
	}
	if _, err := ChecksumFor(body, "missing.tar.gz"); err == nil {
		t.Fatal("expected missing checksum error")
	}
}
