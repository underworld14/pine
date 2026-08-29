package selfupdate

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"path"
	"strings"
)

// ExtractBinary pulls the pine executable bytes from a release archive.
func ExtractBinary(archive []byte, archiveName, goos string) ([]byte, error) {
	want := BinaryName(goos)
	switch {
	case strings.HasSuffix(archiveName, ".zip"):
		return extractZip(archive, want)
	case strings.HasSuffix(archiveName, ".tar.gz") || strings.HasSuffix(archiveName, ".tgz"):
		return extractTarGz(archive, want)
	default:
		return nil, fmt.Errorf("unsupported archive format: %s", archiveName)
	}
}

func extractTarGz(archive []byte, want string) ([]byte, error) {
	gr, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return nil, fmt.Errorf("gzip: %w", err)
	}
	defer gr.Close()
	tr := tar.NewReader(gr)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("tar: %w", err)
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		base := path.Base(hdr.Name)
		if base != want {
			continue
		}
		data, err := io.ReadAll(io.LimitReader(tr, 256<<20))
		if err != nil {
			return nil, err
		}
		return data, nil
	}
	return nil, fmt.Errorf("%s not found in archive", want)
}

func extractZip(archive []byte, want string) ([]byte, error) {
	zr, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
	if err != nil {
		return nil, fmt.Errorf("zip: %w", err)
	}
	for _, f := range zr.File {
		if path.Base(f.Name) != want {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return nil, err
		}
		data, err := io.ReadAll(io.LimitReader(rc, 256<<20))
		rc.Close()
		if err != nil {
			return nil, err
		}
		return data, nil
	}
	return nil, fmt.Errorf("%s not found in archive", want)
}
