// Package gw (ghost writer) makes it easy to write simple build
// systems and other programs that watch for filesystem changes.
//
// Say that you want to build a static site. All you need to do is
// register handlers for the file paths you care about:
//
//   gw.Match(`^content/.*\.md$`, func(path string, state gw.FileState) error {
//     if state == gw.Deleted {
//       // delete corresponding .html file from build directory
//       return nil
//     }
//     // convert markdown to html write it to build directory
//   })
//
//   gw.Match(`^content/.*\.(jpe?g|png|gif)$`, func(path string, state gw.FileState) error {
//     if state == gw.Deleted {
//       // delete image from build directory
//       return nil
//     }
//     // build final image and write it to the build directory
//   })
//
// If a change in one file should cause another file to rebuild, just
// mark the other file as needing to be rebuilt.
//
//   gw.Match(`^design/.*\.html$`, func(path string, state gw.FileState) error {
//     gw.SetStates(`content/.*\.md`, gw.Changed)
//     return nil
//   })
//
// You can also specify which paths to ignore:
//
//   const buildDir = "build"
//   gw.Ignore(`^` + buildDir + `.*$`)
//   gw.Ignore(`^watch\.go$`)
//   gw.Ignore(`^.*~$`) // emacs backup
//   gw.Ignore(`^.*#$`) // emacs auto-save
//
// Then we can clean the build directory and watch for changes/deletes.
//
//   if err := os.RemoveAll(buildDir); err != nil {
//     log.Fatal(err)
//   }
//   gw.SetStates(`.*`, gw.Changed)
//   if err := gw.Watch(); err != nil {
//     log.Fatal(err)
//   }
//
// If you put all of this in `watch.go` at the root of the tree, you
// can now build the site with:
//
//   go run watch.go
//
package gw

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"time"

	fsnotify "gopkg.in/fsnotify.v1"
)

type FileState int

const (
	Clean FileState = iota
	Changed
	Deleted
)

type Handler func(string, FileState) error

// Log is called for every state change. By default, it prints the
// event to os.Stdout.
var Log = logEvent

type matcher struct {
	pattern *regexp.Regexp
	f       Handler
}

var (
	matchers = []*matcher{}
	ignores  = []*regexp.Regexp{}

	states = map[string]FileState{}
)

// Match registers a handler to be called whenever a file matching a
// pattern is changed or deleted.
func Match(pattern string, f Handler) {
	matchers = append(matchers, &matcher{regexp.MustCompile(pattern), f})
}

// Ignore specifies files that should never be matched.
func Ignore(pattern string) {
	ignores = append(ignores, regexp.MustCompile(pattern))
}

// SetState marks a path as having a certain state, overriding the
// current state. If state is Clean, it will prevent handlers from
// being called on the next Dispatch, otherwise it will cause handlers
// to be called on the next Dispatch.
func SetState(path string, state FileState) {
	if !hasMatcher(path) {
		return
	}
	if state == Clean {
		delete(states, path)
	} else {
		states[path] = state
	}
	Log(path, state)
}

// SetStates calls SetState with the given state on each path that
// matches the pattern.
func SetStates(pattern string, state FileState) error {
	r, err := regexp.Compile(pattern)
	if err != nil {
		return err
	}
	return filepath.Walk(".", func(path string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !r.MatchString(path) || hasIgnore(path) {
			return nil
		}
		SetState(path, state)
		return nil
	})
}

// Dispatch calls registered handlers for each path marked as Changed
// or Deleted.
func Dispatch() error {
	for {
		path, state, ok := nextDirty()
		if !ok {
			return nil
		}
		SetState(path, Clean)
		for _, m := range matchers {
			if m.pattern.MatchString(path) {
				if err := m.f(path, state); err != nil {
					return err
				}
			}
		}
	}
}

// Watch listens for file change/deletion events and calls the
// corresponding handlers.
func Watch() error {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	defer w.Close()
	if err = watchRecursive(w, "."); err != nil {
		return err
	}
	for {
		select {
		case <-time.After(250 * time.Millisecond):
			Dispatch()
		case e := <-w.Events:
			if hasIgnore(e.Name) {
				continue
			}
			if e.Op&fsnotify.Create != 0 {
				info, err := os.Stat(e.Name)
				if err != nil {
					return err
				}
				if info.IsDir() {
					if err = watchRecursive(w, e.Name); err != nil {
						return err
					}
				}
			}
			if e.Op&fsnotify.Remove != 0 {
				SetState(e.Name, Deleted)
			} else {
				SetState(e.Name, Changed)
			}
		case err := <-w.Errors:
			return err
		}
	}
}

func watchRecursive(w *fsnotify.Watcher, dir string) error {
	return filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() || hasIgnore(path) {
			return nil
		}
		SetState(path, Clean)
		return w.Add(path)
	})
}

func hasMatcher(path string) bool {
	for _, m := range matchers {
		if m.pattern.MatchString(path) {
			return true
		}
	}
	return false
}

func hasIgnore(path string) bool {
	for _, v := range ignores {
		if v.MatchString(path) {
			return true
		}
	}
	return false
}

func nextDirty() (path string, state FileState, ok bool) {
	for k, v := range states {
		return k, v, true
	}
	return "", Clean, false
}

func logEvent(path string, state FileState) {
	var s string
	switch state {
	case Clean:
		s = "clean"
	case Changed:
		s = "changed"
	case Deleted:
		s = "deleted"
	}
	fmt.Println(s, path)
}
