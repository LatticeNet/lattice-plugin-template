package pluginpack

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestPackNormalizesPathsAndMetadata(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "ui/index.html", []byte("<!doctype html>"))
	writeFile(t, root, "ui/assets/app.js", []byte("console.log('ok')"))
	writeFile(t, root, "bin/linux-arm64/plugin", []byte("arm64"))
	writeFile(t, root, "bin/linux-amd64/plugin", []byte("amd64"))

	var archive bytes.Buffer
	if err := Pack(root, &archive); err != nil {
		t.Fatalf("Pack returned error: %v", err)
	}
	if !bytes.Equal(archive.Bytes()[4:8], []byte{0, 0, 0, 0}) {
		t.Fatalf("gzip mtime bytes = %v, want unix epoch seconds", archive.Bytes()[4:8])
	}

	gz, err := gzip.NewReader(bytes.NewReader(archive.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	var names []string
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		names = append(names, hdr.Name)
		if !hdr.ModTime.Equal(time.Unix(0, 0).UTC()) {
			t.Fatalf("%s mod time = %v, want unix epoch", hdr.Name, hdr.ModTime)
		}
		if hdr.Uid != 0 || hdr.Gid != 0 {
			t.Fatalf("%s uid/gid = %d/%d, want 0/0", hdr.Name, hdr.Uid, hdr.Gid)
		}
		switch hdr.Name {
		case "bin/", "bin/linux-amd64/", "bin/linux-arm64/", "ui/", "ui/assets/":
			if hdr.FileInfo().Mode().Perm() != 0o700 {
				t.Fatalf("%s mode = %o, want 0700", hdr.Name, hdr.FileInfo().Mode().Perm())
			}
		case "bin/linux-amd64/plugin", "bin/linux-arm64/plugin":
			if hdr.FileInfo().Mode().Perm() != 0o700 {
				t.Fatalf("%s mode = %o, want 0700", hdr.Name, hdr.FileInfo().Mode().Perm())
			}
		default:
			if hdr.FileInfo().Mode().Perm() != 0o600 {
				t.Fatalf("%s mode = %o, want 0600", hdr.Name, hdr.FileInfo().Mode().Perm())
			}
		}
	}

	want := []string{
		"bin/",
		"bin/linux-amd64/",
		"bin/linux-amd64/plugin",
		"bin/linux-arm64/",
		"bin/linux-arm64/plugin",
		"ui/",
		"ui/assets/",
		"ui/assets/app.js",
		"ui/index.html",
	}
	if len(names) != len(want) {
		t.Fatalf("entry count = %d, want %d\nentries: %v", len(names), len(want), names)
	}
	for i := range want {
		if names[i] != want[i] {
			t.Fatalf("entry %d = %q, want %q", i, names[i], want[i])
		}
	}
}

func TestPackIgnoresSourceMtimesWhenComputingDigest(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "ui/index.html", []byte("<!doctype html>"))
	writeFile(t, root, "ui/assets/app.js", []byte("console.log('ok')"))
	writeFile(t, root, "bin/linux-amd64/plugin", []byte("amd64"))
	writeFile(t, root, "bin/linux-arm64/plugin", []byte("arm64"))

	var first bytes.Buffer
	if err := Pack(root, &first); err != nil {
		t.Fatalf("first Pack returned error: %v", err)
	}

	now := time.Date(2026, time.July, 13, 11, 15, 0, 0, time.UTC)
	for _, rel := range []string{
		"ui/index.html",
		"ui/assets/app.js",
		"bin/linux-amd64/plugin",
		"bin/linux-arm64/plugin",
	} {
		path := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.Chtimes(path, now, now); err != nil {
			t.Fatal(err)
		}
		now = now.Add(3 * time.Hour)
	}

	var second bytes.Buffer
	if err := Pack(root, &second); err != nil {
		t.Fatalf("second Pack returned error: %v", err)
	}

	if !bytes.Equal(first.Bytes(), second.Bytes()) {
		t.Fatal("deterministic pack output changed after source mtime changes")
	}
	if sum(first.Bytes()) != sum(second.Bytes()) {
		t.Fatalf("digest mismatch: %s vs %s", sum(first.Bytes()), sum(second.Bytes()))
	}
}

func TestPackRejectsUnsupportedTypesAndUnsafeNames(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "ui/index.html", []byte("<!doctype html>"))
	writeFile(t, root, "ui/assets/app.js", []byte("console.log('ok')"))
	writeFile(t, root, "bin/linux-amd64/plugin", []byte("amd64"))
	writeFile(t, root, "bin/linux-arm64/plugin", []byte("arm64"))
	writeFile(t, root, "ui/assets/evil\\name.js", []byte("bad"))

	var archive bytes.Buffer
	if err := Pack(root, &archive); err == nil {
		t.Fatal("Pack succeeded with an unsafe path containing backslashes")
	}
}

func TestPackRejectsSymlinks(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "ui/index.html", []byte("<!doctype html>"))
	writeFile(t, root, "ui/assets/app.js", []byte("console.log('ok')"))
	writeFile(t, root, "bin/linux-amd64/plugin", []byte("amd64"))
	writeFile(t, root, "bin/linux-arm64/plugin", []byte("arm64"))
	if err := os.Symlink("app.js", filepath.Join(root, "ui/assets/link.js")); err != nil {
		t.Fatal(err)
	}

	var archive bytes.Buffer
	if err := Pack(root, &archive); err == nil {
		t.Fatal("Pack succeeded with a symlink entry")
	}
}

func writeFile(t *testing.T, root, rel string, contents []byte) {
	t.Helper()
	target := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, contents, 0o644); err != nil {
		t.Fatal(err)
	}
}

func sum(data []byte) string {
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}
