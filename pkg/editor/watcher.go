package editor

import (
	"log"
	"os"
	"sync"
	"time"
)

// FileWatcher polls a file for changes and calls a callback.
// Uses polling rather than inotify for simplicity and cross-platform support.
type FileWatcher struct {
	path     string
	callback func()
	done     chan struct{}
	once     sync.Once
}

// NewFileWatcher creates and starts a file watcher.
func NewFileWatcher(path string, callback func()) (*FileWatcher, error) {
	w := &FileWatcher{
		path:     path,
		callback: callback,
		done:     make(chan struct{}),
	}

	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}

	go w.poll(info.ModTime())
	return w, nil
}

func (w *FileWatcher) poll(lastMod time.Time) {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-w.done:
			return
		case <-ticker.C:
			info, err := os.Stat(w.path)
			if err != nil {
				log.Printf("watcher stat: %v", err)
				continue
			}
			if info.ModTime().After(lastMod) {
				lastMod = info.ModTime()
				w.callback()
			}
		}
	}
}

// Close stops the file watcher.
func (w *FileWatcher) Close() {
	w.once.Do(func() {
		close(w.done)
	})
}
