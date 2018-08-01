package fscache

import (
	"errors"
	"io/ioutil"
	"log"
	"path/filepath"
	"strings"
	"sync"

	"gopkg.in/fsnotify.v1"
)

type Watcher struct {
	dir       string
	fswatcher *fsnotify.Watcher
	Cache     *sync.Map
}

// NewWatch creates a directory watcher and
// updates the cache when any file changes in that dir
func NewWatch(dir string) (*Watcher, error) {
	if len(dir) < 1 {
		return nil, errors.New("directory is empty")
	}

	fw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	w := &Watcher{
		dir:       dir,
		fswatcher: fw,
		Cache:     new(sync.Map),
	}

	err = w.fswatcher.Add(w.dir)
	if err != nil {
		return nil, err
	}

	// initial read
	err = w.updateCache()
	if err != nil {
		return nil, err
	}

	return w, nil
}

// Watch watches for when kubelet updates the volume mount content
func (w *Watcher) Watch() {
	go func() {
		for {
			select {
			// it can take up to a 2 minutes for kubelet to recreate the ..data symlink
			case event := <-w.fswatcher.Events:
				if event.Op&fsnotify.Create == fsnotify.Create {
					if filepath.Base(event.Name) == "..data" {
						err := w.updateCache()
						if err != nil {
							log.Println("fscache update error", err)
						} else {
							log.Println("fscache sync with", w.dir)
						}
					}
				}
			case err := <-w.fswatcher.Errors:
				log.Println(w.dir, "fswatcher error", err)
			}
		}
	}()
}

// updateCache reads files content and loads them into the cache
func (w *Watcher) updateCache() error {
	fileMap := make(map[string]string)
	files, err := ioutil.ReadDir(w.dir)
	if err != nil {
		return err
	}

	// read files ignoring symlinks and sub directories
	for _, file := range files {
		name := filepath.Base(file.Name())
		if !file.IsDir() && !strings.Contains(name, "..") {
			b, err := ioutil.ReadFile(filepath.Join(w.dir, file.Name()))
			if err != nil {
				return err
			}
			fileMap[name] = string(b)
		}
	}

	// clear cache
	w.Cache.Range(func(key interface{}, value interface{}) bool {
		w.Cache.Delete(key)
		return true
	})

	// load cache
	for k, v := range fileMap {
		w.Cache.Store(k, v)
	}

	return nil
}
