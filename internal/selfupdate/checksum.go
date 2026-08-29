package selfupdate

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
)

// ChecksumFor finds the hex SHA-256 for assetName in a goreleaser checksums.txt
// body (lines of "hash  filename").
func ChecksumFor(checksumsBody, assetName string) (string, error) {
	for _, line := range strings.Split(checksumsBody, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		hash, name := fields[0], fields[len(fields)-1]
		if name == assetName || strings.HasSuffix(name, "/"+assetName) {
			if len(hash) != 64 {
				return "", fmt.Errorf("invalid checksum length for %s", assetName)
			}
			return strings.ToLower(hash), nil
		}
	}
	return "", fmt.Errorf("no checksum for %s", assetName)
}

// VerifySHA256 ensures data matches the expected hex digest.
func VerifySHA256(data []byte, wantHex string) error {
	sum := sha256.Sum256(data)
	got := hex.EncodeToString(sum[:])
	if !strings.EqualFold(got, wantHex) {
		return fmt.Errorf("checksum mismatch: got %s, want %s", got, wantHex)
	}
	return nil
}
