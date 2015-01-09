// This is an example of how to use package gw. It is a build system
// for a static site written in markdown. The directory structure is:
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
// And it builds to:
//
//   build/
//     index.html
//     foo/
//       bar.html
//       baz.jpg
//     css/
//       base.css
//
package main

import (
	"bufio"
	"errors"
	"fmt"
	"html/template"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/rynlbrwn/gw"
)

func main() {
	const buildDir = "build"

	gw.Ignore(`^` + buildDir + `.*$`)
	gw.Ignore(`^watch\.go$`)
	gw.Ignore(`^.*~$`)
	gw.Ignore(`^.*#$`)
	gw.Ignore(`^\..*$`)
	gw.Ignore(`^.*` + string(filepath.Separator) + `\..*$`)

	gw.Match(`^content/.*\.(jpe?g|png|gif)$`, func(path string, deleted bool) error {
		outPath := filepath.Join(buildDir, rest(path))
		if deleted {
			return os.Remove(outPath)
		}
		in, err := os.Open(path)
		if err != nil {
			return err
		}
		defer in.Close()
		if err = os.MkdirAll(filepath.Dir(outPath), 0755); err != nil {
			return err
		}
		out, err := os.Create(outPath)
		if err != nil {
			return err
		}
		defer out.Close()
		_, err = io.Copy(out, in)
		return err
	})

	gw.Match(`^content/.*\.md$`, func(path string, deleted bool) error {
		r := rest(path)
		ext := filepath.Ext(r)
		outPath := filepath.Join(buildDir, r[:len(r)-len(ext)]+".html")
		if deleted {
			return os.Remove(outPath)
		}
		files, err := filepath.Glob("design/*.html")
		if err != nil {
			return err
		}
		t := template.Must(template.ParseFiles(files...))
		header := t.Lookup("header")
		if header == nil {
			return errors.New("header template not found")
		}
		footer := t.Lookup("footer")
		if footer == nil {
			return errors.New("footer template not found")
		}
		in, err := os.Open(path)
		if err != nil {
			return err
		}
		defer in.Close()
		title, err := parseTitle(in)
		if err != nil {
			return err
		}
		_, err = in.Seek(0, 0)
		if err != nil {
			return err
		}
		if err = os.MkdirAll(filepath.Dir(outPath), 0755); err != nil {
			return err
		}
		out, err := os.Create(outPath)
		if err != nil {
			return err
		}
		defer out.Close()
		cssPath, err := filepath.Rel(filepath.Dir(outPath), filepath.Join(buildDir, "css"))
		if err != nil {
			return err
		}
		err = header.Execute(out, map[string]interface{}{
			"Title": title,
			"CSS":   cssPath,
		})
		c := exec.Command("pandoc", "--to=html")
		c.Stdin = in
		c.Stdout = out
		c.Stderr = os.Stderr
		return c.Run()
	})

	gw.Match(`^design/.*\.scss$`, func(path string, deleted bool) error {
		if filepath.Base(path)[0] == '_' {
			return nil
		}
		r := rest(path)
		ext := filepath.Ext(r)
		outPath := filepath.Join(buildDir, "css", r[:len(r)-len(ext)]+".css")
		if deleted {
			return os.Remove(outPath)
		}
		if err := os.MkdirAll(filepath.Dir(outPath), 0755); err != nil {
			return err
		}
		c := exec.Command("sass", path, outPath)
		c.Stdout = os.Stdout
		c.Stderr = os.Stderr
		return c.Run()
	})

	gw.Match(`^design/.*\.html$`, func(path string, deleted bool) error {
		return gw.Trigger(`content/.*\.md`, false)
	})

	if err := os.RemoveAll(buildDir); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	if err := gw.Trigger(`.*`, false); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	if err := gw.Watch(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func rest(path string) string {
	i := strings.Index(path, string(filepath.Separator))
	if i == -1 {
		return ""
	}
	return path[i+1:]
}

var titleExp = regexp.MustCompile(`# (.*)`)

func parseTitle(r io.Reader) (string, error) {
	s := bufio.NewScanner(r)
	for s.Scan() {
		line := s.Text()
		if matches := titleExp.FindStringSubmatch(line); matches != nil && len(matches) > 1 {
			return matches[1], nil
		}
	}
	return "Ryan Brown", s.Err()
}
