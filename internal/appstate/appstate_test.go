package appstate

import (
	"path/filepath"
	"runtime"
	"testing"
)

// redirect the OS user-config dir into a temp dir for the test
func isolateConfigDir(t *testing.T) {
	t.Helper()
	tmp := t.TempDir()
	if runtime.GOOS == "windows" {
		t.Setenv("AppData", tmp)
	} else {
		t.Setenv("XDG_CONFIG_HOME", tmp)
	}
}

func TestSaveLoadRoundtrip(t *testing.T) {
	isolateConfigDir(t)
	if _, ok := Load(); ok {
		t.Fatal("Load should miss before any Save")
	}
	dir := t.TempDir()
	if err := Save(dir); err != nil {
		t.Fatal(err)
	}
	got, ok := Load()
	if !ok || got != dir {
		t.Errorf("Load() = %q, %v — want %q, true", got, ok, dir)
	}
}

// a remembered folder that no longer exists is ignored, not an error
func TestLoadIgnoresMissingDir(t *testing.T) {
	isolateConfigDir(t)
	gone := filepath.Join(t.TempDir(), "since-deleted")
	if err := Save(gone); err != nil {
		t.Fatal(err)
	}
	if _, ok := Load(); ok {
		t.Error("Load should ignore a vanished data folder")
	}
}
