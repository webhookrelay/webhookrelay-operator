// Command chart-artifact creates reproducible Helm chart archives and verifies
// that a repository index contains the expected immutable artifact.
package main

import (
	"archive/tar"
	"bufio"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

func main() {
	if len(os.Args) < 2 {
		fatalf("usage: chart-artifact package CHART_DIR OUTPUT | verify-index INDEX VERSION DIGEST URL")
	}
	var err error
	switch os.Args[1] {
	case "package":
		if len(os.Args) != 4 {
			fatalf("usage: chart-artifact package CHART_DIR OUTPUT")
		}
		err = packageChart(os.Args[2], os.Args[3])
	case "verify-index":
		if len(os.Args) != 6 {
			fatalf("usage: chart-artifact verify-index INDEX VERSION DIGEST URL")
		}
		err = verifyIndex(os.Args[2], os.Args[3], os.Args[4], os.Args[5])
	default:
		fatalf("unknown command %q", os.Args[1])
	}
	if err != nil {
		fatalf("%v", err)
	}
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}

func packageChart(chartDir, output string) error {
	chartDir = filepath.Clean(chartDir)
	rootName := filepath.Base(chartDir)
	var files []string
	err := filepath.WalkDir(chartDir, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if path != chartDir && ignoredDirectory(entry.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.Type().IsRegular() && !ignoredFile(entry.Name()) {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("walk chart: %w", err)
	}
	sort.Strings(files)
	if len(files) == 0 {
		return errors.New("chart contains no packageable files")
	}

	outputFile, err := os.Create(output)
	if err != nil {
		return fmt.Errorf("create archive: %w", err)
	}
	defer outputFile.Close()
	gzipWriter, err := gzip.NewWriterLevel(outputFile, gzip.BestCompression)
	if err != nil {
		return fmt.Errorf("create gzip writer: %w", err)
	}
	gzipWriter.Header.ModTime = time.Unix(0, 0)
	gzipWriter.Header.OS = 255
	tarWriter := tar.NewWriter(gzipWriter)

	for _, path := range files {
		contents, readErr := os.ReadFile(path)
		if readErr != nil {
			return fmt.Errorf("read %s: %w", path, readErr)
		}
		relative, relErr := filepath.Rel(chartDir, path)
		if relErr != nil {
			return fmt.Errorf("resolve %s: %w", path, relErr)
		}
		header := &tar.Header{
			Name:     filepath.ToSlash(filepath.Join(rootName, relative)),
			Mode:     0o644,
			Size:     int64(len(contents)),
			ModTime:  time.Unix(0, 0),
			Typeflag: tar.TypeReg,
			Format:   tar.FormatUSTAR,
		}
		if err := tarWriter.WriteHeader(header); err != nil {
			return fmt.Errorf("write header for %s: %w", path, err)
		}
		if _, err := tarWriter.Write(contents); err != nil {
			return fmt.Errorf("write %s: %w", path, err)
		}
	}
	if err := tarWriter.Close(); err != nil {
		return fmt.Errorf("close tar archive: %w", err)
	}
	if err := gzipWriter.Close(); err != nil {
		return fmt.Errorf("close gzip archive: %w", err)
	}
	return outputFile.Close()
}

func ignoredDirectory(name string) bool {
	switch name {
	case ".git", ".bzr", ".hg", ".svn", ".idea", ".vscode":
		return true
	default:
		return false
	}
}

func ignoredFile(name string) bool {
	if name == ".DS_Store" || name == ".gitignore" || name == ".bzrignore" || name == ".hgignore" || name == ".project" {
		return true
	}
	for _, suffix := range []string{".swp", ".bak", ".tmp", ".orig", ".tmproj", "~"} {
		if strings.HasSuffix(name, suffix) {
			return true
		}
	}
	return false
}

type indexEntry struct {
	version string
	digest  string
	urls    []string
}

func verifyIndex(path, version, digest, artifactURL string) error {
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open index: %w", err)
	}
	defer file.Close()
	var entries []indexEntry
	var current *indexEntry
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "  - apiVersion:") {
			entries = append(entries, indexEntry{})
			current = &entries[len(entries)-1]
			continue
		}
		if current == nil {
			continue
		}
		trimmed := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(trimmed, "version:"):
			current.version = strings.Trim(strings.TrimSpace(strings.TrimPrefix(trimmed, "version:")), "\"")
		case strings.HasPrefix(trimmed, "digest:"):
			current.digest = strings.Trim(strings.TrimSpace(strings.TrimPrefix(trimmed, "digest:")), "\"")
		case strings.HasPrefix(line, "    - "):
			current.urls = append(current.urls, strings.Trim(strings.TrimSpace(strings.TrimPrefix(line, "    - ")), "\""))
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read index: %w", err)
	}
	for _, entry := range entries {
		if entry.version != version || entry.digest != digest {
			continue
		}
		for _, url := range entry.urls {
			if url == artifactURL {
				return nil
			}
		}
	}
	return fmt.Errorf("index has no version %s with digest %s and URL %s", version, digest, artifactURL)
}

func fileSHA256(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}
