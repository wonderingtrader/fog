package watcher

import (
	"crypto/md5"
	"fmt"
	"io"
	"os"
	"time"
)

type Watcher struct {
	Path     string
	Interval time.Duration
	onChange func()
	stop     chan struct{}
}

func New(path string, interval time.Duration, onChange func()) *Watcher {
	return &Watcher{
		Path:     path,
		Interval: interval,
		onChange: onChange,
		stop:     make(chan struct{}),
	}
}

func (w *Watcher) Start() {
	lastHash := w.hash()
	ticker := time.NewTicker(w.Interval)
	defer ticker.Stop()
	for {
		select {
		case <-w.stop:
			return
		case <-ticker.C:
			h := w.hash()
			if h != lastHash {
				lastHash = h
				w.onChange()
			}
		}
	}
}

func (w *Watcher) Stop() {
	close(w.stop)
}

func (w *Watcher) hash() string {
	f, err := os.Open(w.Path)
	if err != nil {
		return ""
	}
	defer f.Close()
	h := md5.New()
	if _, err := io.Copy(h, f); err != nil {
		return ""
	}
	return fmt.Sprintf("%x", h.Sum(nil))
}
