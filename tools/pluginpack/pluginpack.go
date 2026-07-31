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
	"syscall"
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
	absSource, err := filepath.Abs(sourceDir)
	if err != nil {
		return "", err
	}
	absOutput, err := filepath.Abs(outputPath)
	if err != nil {
		return "", err
	}
	repoRoot, err := findTemplateRoot(filepath.Dir(absOutput))
	if err != nil {
		return "", err
	}
	if err := validateOutputPath(repoRoot, absSource, absOutput); err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(absOutput), 0o700); err != nil {
		return "", err
	}
	if err := rejectExistingOutputAlias(absOutput); err != nil {
		return "", err
	}

	target, err := os.CreateTemp(filepath.Dir(absOutput), "."+filepath.Base(absOutput)+".tmp-*")
	if err != nil {
		return "", err
	}
	tmpPath := target.Name()
	defer os.Remove(tmpPath)

	hash := sha256.New()
	if err := Pack(sourceDir, io.MultiWriter(target, hash)); err != nil {
		_ = target.Close()
		return "", err
	}
	if err := target.Close(); err != nil {
		return "", err
	}
	if err := os.Rename(tmpPath, absOutput); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func findTemplateRoot(start string) (string, error) {
	dir := filepath.Clean(start)
	for {
		if fileExists(filepath.Join(dir, "manifest.json")) &&
			fileExists(filepath.Join(dir, "tools", "pluginpack", "go.mod")) {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("could not find template root above %s", start)
		}
		dir = parent
	}
}

func validateOutputPath(repoRoot, sourceDir, outputPath string) error {
	devDir := filepath.Join(repoRoot, ".lattice-dev")
	if rel, err := filepath.Rel(devDir, outputPath); err != nil || rel == "." || rel == ".." ||
		strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return fmt.Errorf("output must be a file under %s", devDir)
	}
	if rel, err := filepath.Rel(sourceDir, outputPath); err == nil && (rel == "." || (rel != ".." &&
		!strings.HasPrefix(rel, ".."+string(os.PathSeparator)))) {
		return fmt.Errorf("output %s must not be inside source directory %s", outputPath, sourceDir)
	}
	return rejectSymlinkAncestors(devDir, filepath.Dir(outputPath))
}

func rejectSymlinkAncestors(root, targetDir string) error {
	if info, err := os.Lstat(root); err == nil && info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("%s is a symlink", root)
	} else if err != nil && !os.IsNotExist(err) {
		return err
	}

	rel, err := filepath.Rel(root, targetDir)
	if err != nil || rel == "." {
		return nil
	}
	current := root
	for _, part := range strings.Split(rel, string(os.PathSeparator)) {
		if part == "" || part == "." {
			continue
		}
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if os.IsNotExist(err) {
			return nil
		}
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("%s is a symlink", current)
		}
	}
	return nil
}

func rejectExistingOutputAlias(outputPath string) error {
	info, err := os.Lstat(outputPath)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("output %s is a symlink", outputPath)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("output %s is not a regular file", outputPath)
	}
	if hasMultipleLinks(info) {
		return fmt.Errorf("output %s has multiple hard links", outputPath)
	}
	return nil
}

func hasMultipleLinks(info fs.FileInfo) bool {
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && stat.Nlink > 1
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
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
