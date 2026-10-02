// PicoClaw - Ultra-lightweight personal AI agent
// Curator tests.
package agent

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCurator_New(t *testing.T) {
	tmpDir := t.TempDir()
	c := NewCurator(tmpDir, 0)
	if c.interval != 30*time.Minute {
		t.Errorf("NewCurator with 0 interval should default to 30 minutes, got %v", c.interval)
	}

	c = NewCurator(tmpDir, 5*time.Second)
	if c.interval != 5*time.Second {
		t.Errorf("NewCurator interval = %v, want 5s", c.interval)
	}
}

func TestCurator_StartStop(t *testing.T) {
	tmpDir := t.TempDir()
	c := NewCurator(tmpDir, 100*time.Millisecond)

	// Start should not block
	c.Start()

	// Wait a bit
	time.Sleep(50 * time.Millisecond)

	// Stop should not block or deadlock
	done := make(chan bool)
	go func() {
		c.Stop()
		done <- true
	}()

	select {
	case <-done:
		// Success
	case <-time.After(2 * time.Second):
		t.Error("Stop() timed out - possible deadlock")
	}
}

func TestCurator_StartMultipleTimes(t *testing.T) {
	tmpDir := t.TempDir()
	c := NewCurator(tmpDir, 100*time.Millisecond)

	// Multiple starts should be safe
	c.Start()
	c.Start()
	c.Start()

	c.Stop()
}

func TestCurator_StopMultipleTimes(t *testing.T) {
	tmpDir := t.TempDir()
	c := NewCurator(tmpDir, 100*time.Millisecond)
	c.Start()

	// Multiple stops should be safe
	done := make(chan bool, 3)
	for i := 0; i < 3; i++ {
		go func() {
			c.Stop()
			done <- true
		}()
	}

	for i := 0; i < 3; i++ {
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatal("Stop() timed out - possible deadlock")
		}
	}
}

func TestCurator_TouchDir(t *testing.T) {
	tmpDir := t.TempDir()
	testDir := filepath.Join(tmpDir, "test")
	os.MkdirAll(testDir, 0o755)

	c := NewCurator(tmpDir, time.Hour)
	c.touchDir(testDir)

	// Check marker file was created
	marker := filepath.Join(testDir, ".curator")
	if _, err := os.Stat(marker); os.IsNotExist(err) {
		t.Error("touchDir did not create .curator marker file")
	}
}

func TestCurator_SkillRoots(t *testing.T) {
	tmpDir := t.TempDir()
	c := NewCurator(tmpDir, time.Hour)

	roots := c.skillRoots()
	if len(roots) == 0 {
		t.Error("skillRoots returned empty slice")
	}

	// Check for duplicates
	seen := make(map[string]bool)
	for _, root := range roots {
		if seen[root] {
			t.Errorf("Duplicate skill root: %s", root)
		}
		seen[root] = true
	}
}
