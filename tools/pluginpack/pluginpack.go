package pluginpack

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

var epoch = time.Unix(0, 0).UTC()

type entry struct {
	archivePath string
	sourcePath  string
	info        fs.FileInfo
}

func Pack(sourceDir string, out io.Writer) error {
	entries, err := collectEntries(sourceDir)
	if err != nil {
		return err
	}

	gzw, err := gzip.NewWriterLevel(out, gzip.BestCompression)
	if err != nil {
		return err
	}
	gzw.Header.ModTime = epoch

	tw := tar.NewWriter(gzw)
	for _, item := range entries {
		if err := writeEntry(tw, item); err != nil {
			_ = tw.Close()
			_ = gzw.Close()
			return err
		}
	}
	if err := tw.Close(); err != nil {
		_ = gzw.Close()
		return err
	}
	return gzw.Close()
}

func PackFile(sourceDir, outputPath string) (string, error) {
	target, err := os.Create(outputPath)
	if err != nil {
		return "", err
	}
	defer target.Close()

	hash := sha256.New()
	if err := Pack(sourceDir, io.MultiWriter(target, hash)); err != nil {
		_ = os.Remove(outputPath)
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func collectEntries(sourceDir string) ([]entry, error) {
	var entries []entry
	err := filepath.WalkDir(sourceDir, func(current string, dirEntry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if current == sourceDir {
			return nil
		}

		archivePath, err := normalizeArchivePath(sourceDir, current)
		if err != nil {
			return err
		}

		info, err := dirEntry.Info()
		if err != nil {
			return err
		}
		if dirEntry.Type()&fs.ModeSymlink != 0 {
			return fmt.Errorf("unsupported symlink entry %q", archivePath)
		}
		if !info.IsDir() && !info.Mode().IsRegular() {
			return fmt.Errorf("unsupported entry type for %q", archivePath)
		}

		entries = append(entries, entry{
			archivePath: archivePath,
			sourcePath:  current,
			info:        info,
		})
		return nil
	})
	if err != nil {
		return nil, err
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].archivePath < entries[j].archivePath
	})
	return entries, nil
}

func normalizeArchivePath(sourceDir, current string) (string, error) {
	rel, err := filepath.Rel(sourceDir, current)
	if err != nil {
		return "", err
	}
	if rel == "." || rel == "" {
		return "", fmt.Errorf("refusing empty archive path for %q", current)
	}
	if strings.ContainsRune(rel, '\\') {
		return "", fmt.Errorf("unsafe archive path %q: backslashes are not allowed", rel)
	}

	clean := path.Clean(filepath.ToSlash(rel))
	if clean == "." || clean == "" || strings.HasPrefix(clean, "../") || clean == ".." || path.IsAbs(clean) {
		return "", fmt.Errorf("unsafe archive path %q", rel)
	}
	return clean, nil
}

func writeEntry(tw *tar.Writer, item entry) error {
	hdr := &tar.Header{
		Name:    item.archivePath,
		Uid:     0,
		Gid:     0,
		ModTime: epoch,
	}

	mode := int64(0o600)
	if item.info.IsDir() || isRuntimeFile(item.archivePath) {
		mode = 0o700
	}
	hdr.Mode = mode

	if item.info.IsDir() {
		hdr.Typeflag = tar.TypeDir
		hdr.Name += "/"
		return tw.WriteHeader(hdr)
	}

	hdr.Typeflag = tar.TypeReg
	hdr.Size = item.info.Size()
	if err := tw.WriteHeader(hdr); err != nil {
		return err
	}

	file, err := os.Open(item.sourcePath)
	if err != nil {
		return err
	}
	defer file.Close()

	_, err = io.Copy(tw, file)
	return err
}

func isRuntimeFile(archivePath string) bool {
	return strings.HasPrefix(archivePath, "bin/") && path.Base(archivePath) == "plugin"
}
