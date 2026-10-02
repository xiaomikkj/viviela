// PicoClaw - Ultra-lightweight personal AI agent
// Background curator inspired by NousResearch/hermes-agent.
package agent

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
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
	once      sync.Once
	wg        sync.WaitGroup
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
	}
}

// Start begins the background curation loop.
func (c *Curator) Start() {
	if c == nil {
		return
	}
	c.once.Do(func() {
		c.wg.Add(1)
		go c.run()
	})
}

// Stop gracefully stops the background curation loop.
func (c *Curator) Stop() {
	if c == nil {
		return
	}
	select {
	case <-c.stop:
		return
	default:
		close(c.stop)
	}
	c.wg.Wait()
}

func (c *Curator) run() {
	defer c.wg.Done()

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
	c.refreshSkillTreeMtime()
	c.refreshMemoryMtime()
}

func (c *Curator) refreshSkillTreeMtime() {
	roots := c.skillRoots()
	if len(roots) == 0 {
		return
	}
	for _, root := range roots {
		_ = touchMtime(root)
	}
}

func (c *Curator) refreshMemoryMtime() {
	memoryDir := filepath.Join(c.workspace, "memory")
	if _, err := os.Stat(memoryDir); err != nil {
		return
	}
	_ = touchMtime(memoryDir)
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

func touchMtime(path string) error {
	now := time.Now()
	info, err := os.Stat(path)
	if err != nil {
		return err
	}

	if !info.IsDir() {
		return os.Chtimes(path, now, now)
	}

	entries, err := os.ReadDir(path)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if err := touchMtime(filepath.Join(path, entry.Name())); err != nil {
			logger.WarnCF("agent", "curator touch failed", map[string]any{
				"path":  filepath.Join(path, entry.Name()),
				"error": err.Error(),
			})
		}
	}
	return nil
}
