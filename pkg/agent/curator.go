// PicoClaw - Ultra-lightweight personal AI agent
// Background curator inspired by NousResearch/hermes-agent.
package agent

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/sipeed/picoclaw/pkg/logger"
)

// Curator maintains skills and memory in the background.
// It is intentionally minimal: a single goroutine that periodically
// touches workspace metadata so background refresh is possible later.
type Curator struct {
	workspace string
	interval  time.Duration
	stop      chan struct{}
	stopped   chan struct{}
	startOnce sync.Once
	stopOnce  sync.Once
	running   atomic.Bool
}

// NewCurator creates a new Curator.
func NewCurator(workspace string, interval time.Duration) *Curator {
	if interval <= 0 {
		interval = 30 * time.Minute
	}
	return &Curator{
		workspace: workspace,
		interval:  interval,
		stop:      make(chan struct{}),
		stopped:   make(chan struct{}),
	}
}

// Start begins the background curation loop. Safe to call multiple times.
func (c *Curator) Start() {
	if c == nil {
		return
	}
	c.startOnce.Do(func() {
		c.running.Store(true)
		go c.run()
	})
}

// Stop gracefully stops the background curation loop. Safe to call multiple times.
func (c *Curator) Stop() {
	if c == nil {
		return
	}
	c.stopOnce.Do(func() {
		close(c.stop)
		<-c.stopped
	})
}

func (c *Curator) run() {
	defer close(c.stopped)

	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()

	// Run an initial pass shortly after start so the curator is not silent
	// until the first full interval elapses.
	select {
	case <-time.After(5 * time.Second):
		c.tick()
	case <-c.stop:
		return
	}

	for {
		select {
		case <-ticker.C:
			c.tick()
		case <-c.stop:
			return
		}
	}
}

func (c *Curator) tick() {
	if !c.running.Load() {
		return
	}
	c.refreshSkillTreeMtime()
	c.refreshMemoryMtime()
}

func (c *Curator) refreshSkillTreeMtime() {
	roots := c.skillRoots()
	for _, root := range roots {
		c.touchDir(root)
	}
}

func (c *Curator) refreshMemoryMtime() {
	memoryDir := filepath.Join(c.workspace, "memory")
	if _, err := os.Stat(memoryDir); err == nil {
		c.touchDir(memoryDir)
	}
}

// touchDir touches only the directory itself and a .curator marker file.
// It does not recurse into subdirectories to avoid O(n) file operations.
func (c *Curator) touchDir(dir string) {
	now := time.Now()
	if err := os.Chtimes(dir, now, now); err != nil {
		logger.DebugCF("agent", "curator touch dir failed", map[string]any{
			"path":  dir,
			"error": err.Error(),
		})
	}

	// Also touch a marker file so file watchers can detect changes.
	marker := filepath.Join(dir, ".curator")
	if err := os.WriteFile(marker, []byte(now.Format(time.RFC3339)), 0o644); err != nil {
		logger.DebugCF("agent", "curator write marker failed", map[string]any{
			"path":  marker,
			"error": err.Error(),
		})
	}
}

func (c *Curator) skillRoots() []string {
	seen := make(map[string]struct{})
	var roots []string

	add := func(root string) {
		root = filepath.Clean(root)
		if root == "" {
			return
		}
		if _, ok := seen[root]; ok {
			return
		}
		seen[root] = struct{}{}
		roots = append(roots, root)
	}

	workspace := filepath.Clean(c.workspace)
	if workspace != "" {
		add(filepath.Join(workspace, "skills"))
	}
	if wd, err := os.Getwd(); err == nil {
		add(filepath.Join(wd, "skills"))
	}
	if home, err := os.UserHomeDir(); err == nil && strings.TrimSpace(home) != "" {
		add(filepath.Join(home, ".picoclaw", "skills"))
	}

	return roots
}
