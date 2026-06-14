package guard

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

const watchDebounce = 500 * time.Millisecond

type changeHandler func(rootPath string)

type Watcher struct {
	handler changeHandler
	watcher *fsnotify.Watcher
	roots   map[string]struct{}
	mu      sync.Mutex
	stop    context.CancelFunc
}

func NewWatcher(handler changeHandler) *Watcher {
	return &Watcher{
		handler: handler,
		roots:   make(map[string]struct{}),
	}
}

func (w *Watcher) AddWorkspace(rootPath string) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.roots[rootPath] = struct{}{}
	if w.watcher != nil {
		return watchTree(w.watcher, rootPath)
	}
	return nil
}

func (w *Watcher) Start(ctx context.Context) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.watcher != nil {
		return nil
	}
	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	for root := range w.roots {
		if err := watchTree(fsw, root); err != nil {
			_ = fsw.Close()
			return err
		}
	}
	runCtx, cancel := context.WithCancel(ctx)
	w.stop = cancel
	w.watcher = fsw
	go w.loop(runCtx)
	return nil
}

func (w *Watcher) Stop() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.stop != nil {
		w.stop()
		w.stop = nil
	}
	if w.watcher != nil {
		_ = w.watcher.Close()
		w.watcher = nil
	}
}

func (w *Watcher) loop(ctx context.Context) {
	w.mu.Lock()
	fsw := w.watcher
	w.mu.Unlock()
	if fsw == nil {
		return
	}

	debounce := make(map[string]*time.Timer)
	var debounceMu sync.Mutex

	schedule := func(root string) {
		debounceMu.Lock()
		defer debounceMu.Unlock()
		if timer, ok := debounce[root]; ok {
			timer.Stop()
		}
		debounce[root] = time.AfterFunc(watchDebounce, func() {
			if w.handler != nil {
				w.handler(root)
			}
		})
	}

	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-fsw.Events:
			if !ok {
				return
			}
			if event.Op&fsnotify.Create != 0 {
				if info, err := os.Stat(event.Name); err == nil && info.IsDir() && !shouldSkipWatchDir(event.Name) {
					_ = watchTree(fsw, event.Name)
				}
			}
			if event.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Rename|fsnotify.Remove) == 0 {
				continue
			}
			root := w.matchRoot(event.Name)
			if root != "" {
				schedule(root)
			}
		case _, ok := <-fsw.Errors:
			if !ok {
				return
			}
		}
	}
}

func (w *Watcher) matchRoot(path string) string {
	w.mu.Lock()
	defer w.mu.Unlock()
	for root := range w.roots {
		if path == root || hasPathPrefix(path, root) {
			return root
		}
	}
	return ""
}

func hasPathPrefix(path, root string) bool {
	if len(path) <= len(root) {
		return false
	}
	if path[:len(root)] != root {
		return false
	}
	next := path[len(root)]
	return next == '/' || next == '\\'
}

func watchTree(fsw *fsnotify.Watcher, root string) error {
	return filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			return nil
		}
		if shouldSkipWatchDir(path) && path != root {
			return filepath.SkipDir
		}
		if err := fsw.Add(path); err != nil {
			return err
		}
		return nil
	})
}

func shouldSkipWatchDir(path string) bool {
	name := filepath.Base(path)
	if name == ".pkv" {
		return true
	}
	sep := string(os.PathSeparator)
	return strings.Contains(path, sep+".pkv"+sep)
}
