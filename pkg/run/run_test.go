package run

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSaveAndLoad(t *testing.T) {
	// Override HOME to use a temp directory
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	meta := &RunMetadata{
		RunID:         "test1234",
		ProfileName:   "ml-training-mix",
		ProfilePath:   "/path/to/profile.yaml",
		TopologyName:  "my-topo",
		ClusterName:   "my-topo",
		Seed:          42,
		DryRun:        false,
		WorkloadCount: 15,
		StartedAt:     time.Date(2026, 3, 28, 12, 0, 0, 0, time.UTC),
		Duration:      "5m30.123s",
	}

	if err := Save(meta); err != nil {
		t.Fatalf("Save() error: %v", err)
	}

	// Verify file exists
	metaPath := filepath.Join(tmp, metadataDir, "test1234", metadataFilename)
	if _, err := os.Stat(metaPath); err != nil {
		t.Fatalf("metadata file not found: %v", err)
	}

	loaded, err := Load("test1234")
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if loaded.RunID != meta.RunID {
		t.Errorf("RunID = %q, want %q", loaded.RunID, meta.RunID)
	}
	if loaded.Seed != meta.Seed {
		t.Errorf("Seed = %d, want %d", loaded.Seed, meta.Seed)
	}
	if loaded.WorkloadCount != meta.WorkloadCount {
		t.Errorf("WorkloadCount = %d, want %d", loaded.WorkloadCount, meta.WorkloadCount)
	}
	if loaded.ProfileName != meta.ProfileName {
		t.Errorf("ProfileName = %q, want %q", loaded.ProfileName, meta.ProfileName)
	}
	if loaded.TopologyName != meta.TopologyName {
		t.Errorf("TopologyName = %q, want %q", loaded.TopologyName, meta.TopologyName)
	}
	if loaded.Duration != meta.Duration {
		t.Errorf("Duration = %q, want %q", loaded.Duration, meta.Duration)
	}
}

func TestListEmpty(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	runs, err := List()
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
	if len(runs) != 0 {
		t.Errorf("List() returned %d runs, want 0", len(runs))
	}
}

func TestListMultipleSortedByStartedAt(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	older := &RunMetadata{
		RunID:     "run-older",
		StartedAt: time.Date(2026, 3, 27, 10, 0, 0, 0, time.UTC),
		Seed:      1,
	}
	newer := &RunMetadata{
		RunID:     "run-newer",
		StartedAt: time.Date(2026, 3, 28, 10, 0, 0, 0, time.UTC),
		Seed:      2,
	}

	if err := Save(older); err != nil {
		t.Fatalf("Save(older) error: %v", err)
	}
	if err := Save(newer); err != nil {
		t.Fatalf("Save(newer) error: %v", err)
	}

	runs, err := List()
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
	if len(runs) != 2 {
		t.Fatalf("List() returned %d runs, want 2", len(runs))
	}
	if runs[0].RunID != "run-newer" {
		t.Errorf("runs[0].RunID = %q, want %q (newest first)", runs[0].RunID, "run-newer")
	}
	if runs[1].RunID != "run-older" {
		t.Errorf("runs[1].RunID = %q, want %q", runs[1].RunID, "run-older")
	}
}

func TestLoadNonExistent(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	_, err := Load("does-not-exist")
	if err == nil {
		t.Error("Load() should return error for non-existent run")
	}
}
