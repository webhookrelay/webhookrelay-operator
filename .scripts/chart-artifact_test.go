package main

import (
	"archive/tar"
	"compress/gzip"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestPackageChartIsReproducible(t *testing.T) {
	chartDir := filepath.Join(t.TempDir(), "example")
	mustWrite(t, filepath.Join(chartDir, "Chart.yaml"), "name: example\nversion: 1.0.0\n")
	mustWrite(t, filepath.Join(chartDir, "templates", "deployment.yaml"), "kind: Deployment\n")
	mustWrite(t, filepath.Join(chartDir, ".DS_Store"), "ignored")

	first := filepath.Join(t.TempDir(), "first.tgz")
	second := filepath.Join(t.TempDir(), "second.tgz")
	if err := packageChart(chartDir, first); err != nil {
		t.Fatal(err)
	}
	if err := packageChart(chartDir, second); err != nil {
		t.Fatal(err)
	}
	firstDigest, err := fileSHA256(first)
	if err != nil {
		t.Fatal(err)
	}
	secondDigest, err := fileSHA256(second)
	if err != nil {
		t.Fatal(err)
	}
	if firstDigest != secondDigest {
		t.Fatalf("archive digests differ: %s != %s", firstDigest, secondDigest)
	}

	file, err := os.Open(first)
	if err != nil {
		t.Fatal(err)
	}
	gzipReader, err := gzip.NewReader(file)
	if err != nil {
		t.Fatal(err)
	}
	tarReader := tar.NewReader(gzipReader)
	var names []string
	for {
		header, err := tarReader.Next()
		if err != nil {
			break
		}
		names = append(names, header.Name)
	}
	want := []string{"example/Chart.yaml", "example/templates/deployment.yaml"}
	if !reflect.DeepEqual(names, want) {
		t.Fatalf("archive files = %v, want %v", names, want)
	}
}

func TestVerifyIndex(t *testing.T) {
	path := filepath.Join(t.TempDir(), "index.yaml")
	mustWrite(t, path, `apiVersion: v1
entries:
  webhookrelay-operator:
  - apiVersion: v2
    digest: abc123
    urls:
    - https://charts.example/operator-0.7.0.tgz
    version: 0.7.0
`)
	if err := verifyIndex(path, "0.7.0", "abc123", "https://charts.example/operator-0.7.0.tgz"); err != nil {
		t.Fatal(err)
	}
	if err := verifyIndex(path, "0.7.0", "wrong", "https://charts.example/operator-0.7.0.tgz"); err == nil {
		t.Fatal("expected digest mismatch")
	}

	newPath := filepath.Join(t.TempDir(), "new-index.yaml")
	mustWrite(t, newPath, `apiVersion: v1
entries:
  webhookrelay-operator:
  - apiVersion: v2
    digest: def456
    urls:
    - https://charts.example/operator-0.8.0.tgz
    version: 0.8.0
  - apiVersion: v2
    digest: abc123
    urls:
    - https://charts.example/operator-0.7.0.tgz
    version: 0.7.0
`)
	if err := verifyRetained(path, newPath); err != nil {
		t.Fatal(err)
	}
	if err := verifyRetained(newPath, path); err == nil {
		t.Fatal("expected dropped entry error")
	}
}

func mustWrite(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}
