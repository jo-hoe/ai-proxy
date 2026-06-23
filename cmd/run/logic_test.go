package main

import (
	"archive/tar"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"
)

func TestToWindowsPath_AlreadyWindows(t *testing.T) {
	got, err := toWindowsPath(`C:\Users\foo\token`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != `C:\Users\foo\token` {
		t.Errorf("got %q, want C:\\Users\\foo\\token", got)
	}
}

func TestToWindowsPath_MSYSStyle(t *testing.T) {
	got, err := toWindowsPath("/c/Users/foo/token")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != `C:\Users\foo\token` {
		t.Errorf("got %q, want C:\\Users\\foo\\token", got)
	}
}

func TestToWindowsPath_Empty(t *testing.T) {
	_, err := toWindowsPath("")
	if err == nil {
		t.Fatal("expected error for empty path")
	}
}

func TestParseConfigFile_Valid(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "config.yaml")
	os.WriteFile(f, []byte("proxy:\n  port: 9000\n"), 0600)

	cfg, err := parseConfigFile(f)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Proxy.Port != 9000 {
		t.Errorf("port = %d, want 9000", cfg.Proxy.Port)
	}
}

func TestParseConfigFile_DefaultPort(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "config.yaml")
	os.WriteFile(f, []byte("oidc:\n  endpoint: https://x\n"), 0600)

	cfg, err := parseConfigFile(f)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Proxy.Port != 7655 {
		t.Errorf("port = %d, want 7655", cfg.Proxy.Port)
	}
}

func TestExtractFromTar(t *testing.T) {
	dir := t.TempDir()
	archivePath := filepath.Join(dir, "test.tar.gz")
	destPath := filepath.Join(dir, "proxy")

	// Build a tiny .tar.gz with one entry named "proxy"
	if err := buildTestTar(archivePath, "proxy", []byte("binary-content")); err != nil {
		t.Fatalf("build tar: %v", err)
	}

	if err := extractFromTar(archivePath, "proxy", destPath); err != nil {
		t.Fatalf("extractFromTar: %v", err)
	}

	data, _ := os.ReadFile(destPath)
	if string(data) != "binary-content" {
		t.Errorf("content = %q, want binary-content", string(data))
	}
}

func TestExtractFromTar_EntryNotFound(t *testing.T) {
	dir := t.TempDir()
	archivePath := filepath.Join(dir, "test.tar.gz")
	buildTestTar(archivePath, "other", []byte("x"))

	err := extractFromTar(archivePath, "proxy", filepath.Join(dir, "proxy"))
	if err == nil {
		t.Fatal("expected error for missing entry")
	}
}

func buildTestTar(path, name string, content []byte) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	hdr := &tar.Header{Name: name, Mode: 0755, Size: int64(len(content))}
	if err := tw.WriteHeader(hdr); err != nil {
		return err
	}
	if _, err := tw.Write(content); err != nil {
		return err
	}
	tw.Close()
	return gz.Close()
}
