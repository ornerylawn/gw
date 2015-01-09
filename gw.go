// Package gw (ghost writer) makes it easy to write simple build
// systems and other programs that watch for filesystem changes.
//
// For example, let's say that you want to build a static site with
// markdown. Your directory structure looks like this:
//
//   content/
//     index.md
//     foo/
//       bar.md
//       baz.jpg
//   design/
//     header.html
//     footer.html
//     _reset.scss
//     base.scss
//
// To build the site we write a simple watch.go file at the root of
// our tree and use package gw (full example in example/watch.go). We
// can ignore files that we don't want to build:
//
//   const buildDir = "build"
//   gw.Ignore(`^` + buildDir + `.*$`)
//   gw.Ignore(`^watch\.go$`)
//   gw.Ignore(`^.*~$`)
//   gw.Ignore(`^.*#$`)
//   gw.Ignore(`^\..*$`)
//   gw.Ignore(`^.*` + string(filepath.Separator) + `\..*$`)
//
// We'll need a rule for copying images to the build directory.
//
//   gw.Match(`^content/.*\.(jpe?g|png|gif)$`, func(path string, deleted bool) error {
//     if deleted {
//       // Delete corresponding file from build directory.
//     } else {
//       // Copy file at path to build directory
//     }
//   })
//
// And we'll need a rule for converting markdown files using our
// header.html and footer.html templates.
//
//   gw.Match(`^content/.*\.md$`, func(path string, deleted bool) error {
//     if deleted {
//       // Delete corresponding .html file from build directory.
//     } else {
//       // Convert markdown to html and surround with header.html and
//       // footer.html templates. Write output to build directory.
//     }
//   })
//
// But we want to make sure that all of the markdown files are rebuilt
// if any of the template files is changed.
//
//   gw.Match(`^design/.*\.html$`, func(path string, deleted bool) error {
//     return gw.Trigger(`content/.*\.md`, false)
//   })
//
// Then we can clean the build directory and build the whole tree.
//
//   if err := os.RemoveAll(buildDir); err != nil {
//     fmt.Println(err)
//     os.Exit(1)
//   }
//   if err := gw.Trigger(`.*`, false); err != nil {
//     fmt.Println(err)
//     os.Exit(1)
//   }
//
// Finally, we can watch for changes and automatically trigger rules
// for files that change.
//
//   if err := gw.Watch(); err != nil {
//     fmt.Println(err)
//     os.Exit(1)
//   }
//
// Run the new build system with:
//
//   go run watch.go
//
package gw

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	fsnotify "gopkg.in/fsnotify.v1"
)

type buildFunc func(string, bool) error

type matcher struct {
	pattern *regexp.Regexp
	f       buildFunc
}

var (
	matchers = []*matcher{}
	ignores  = []*regexp.Regexp{}
)

// Match registers a func to be called whenever a file matching a
// pattern is changed or deleted.
func Match(pattern string, f func(path string, deleted bool) error) {
	matchers = append(matchers, &matcher{regexp.MustCompile(pattern), buildFunc(f)})
}

// Ignore specifies files that should not cause registered funcs to be
// called.
func Ignore(pattern string) {
	ignores = append(ignores, regexp.MustCompile(pattern))
}

// Trigger calls the registered funcs for any files that match a
// pattern. The deleted bool is passed to the registered funcs.
func Trigger(pattern string, deleted bool) error {
	r, err := regexp.Compile(pattern)
	if err != nil {
		return err
	}
	return filepath.Walk(".", func(path string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !r.MatchString(path) || shouldIgnore(path) {
			return nil
		}
		m := findMatcher(path)
		if m == nil {
			return nil
		}
		fmt.Printf("triggering %s\n", path)
		if err := m.f(path, deleted); err != nil {
			return err
		}
		return nil
	})
}

// Watch listens for file change/deletion events and calls the
// corresponding registered funcs.
func Watch() error {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	defer w.Close()
	if err = watchRecursive(w, "."); err != nil {
		return err
	}
	errCh := make(chan error)
	go func() {
		for {
			select {
			case e := <-w.Events:
				if shouldIgnore(e.Name) {
					continue
				}
				if e.Op&fsnotify.Create != 0 {
					info, err := os.Stat(e.Name)
					if err != nil {
						fmt.Println(err)
						continue
					}
					if info.IsDir() {
						if err = watchRecursive(w, e.Name); err != nil {
							fmt.Println(err)
							continue
						}
					}
				}
				pattern := `^` + e.Name + `$`
				if err := Trigger(pattern, e.Op&fsnotify.Remove != 0); err != nil {
					fmt.Println(err)
				}
			case err := <-w.Errors:
				errCh <- err
				break
			}
		}
	}()
	return <-errCh
}

func watchRecursive(w *fsnotify.Watcher, dir string) error {
	return filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() || shouldIgnore(path) {
			return nil
		}
		fmt.Printf("watching %s...\n", path)
		return w.Add(path)
	})
}

func findMatcher(path string) *matcher {
	for _, m := range matchers {
		if m.pattern.MatchString(path) {
			return m
		}
	}
	return nil
}

func shouldIgnore(path string) bool {
	for _, v := range ignores {
		if v.MatchString(path) {
			return true
		}
	}
	return false
}
