package selfupdate

import "testing"

func TestAssetName(t *testing.T) {
	t.Parallel()
	cases := []struct {
		ver, goos, arch, want string
	}{
		{"0.7.1", "darwin", "arm64", "pine_0.7.1_darwin_arm64.tar.gz"},
		{"v0.7.1", "linux", "amd64", "pine_0.7.1_linux_amd64.tar.gz"},
		{"0.7.1", "windows", "amd64", "pine_0.7.1_windows_amd64.zip"},
		{"0.7.1", "windows", "arm64", "pine_0.7.1_windows_arm64.zip"},
	}
	for _, tc := range cases {
		got, err := AssetName(tc.ver, tc.goos, tc.arch)
		if err != nil {
			t.Fatalf("AssetName(%q,%q,%q): %v", tc.ver, tc.goos, tc.arch, err)
		}
		if got != tc.want {
			t.Errorf("got %q want %q", got, tc.want)
		}
	}
	if _, err := AssetName("1.0.0", "solaris", "amd64"); err == nil {
		t.Fatal("expected unsupported OS error")
	}
	if _, err := AssetName("1.0.0", "linux", "386"); err == nil {
		t.Fatal("expected unsupported arch error")
	}
}

func TestBinaryName(t *testing.T) {
	t.Parallel()
	if BinaryName("darwin") != "pine" {
		t.Fatal(BinaryName("darwin"))
	}
	if BinaryName("windows") != "pine.exe" {
		t.Fatal(BinaryName("windows"))
	}
}
