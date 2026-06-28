package storage

import (
	"os"
	"path"
	"strings"
	"time"

	"github.com/spf13/afero"
)

// rootFs is an afero.Fs backed by *os.Root. Unlike afero.BasePathFs (which only
// cleans path strings), os.Root enforces containment at the OS level: every
// operation is confined beneath the root directory, and symbolic links may not
// escape it. This is the real protection against directory traversal — both for
// the HTTP handlers and for the FTP driver, which share this filesystem.
type rootFs struct {
	root *os.Root
}

// newRootFs wraps an open *os.Root as an afero.Fs.
func newRootFs(root *os.Root) afero.Fs {

	return &rootFs{root: root}
}

// rel converts a slash-rooted path ("/a/b", "/") into the relative form os.Root
// expects ("a/b", "."). os.Root itself rejects anything that escapes the root.
func rel(name string) string {

	var normalized string = strings.ReplaceAll(name, "\\", "/")

	var cleaned string = path.Clean("/" + normalized)

	cleaned = strings.TrimPrefix(cleaned, "/")

	if "" == cleaned {
		return "."
	}

	return cleaned
}

func (fs *rootFs) Create(name string) (afero.File, error) {

	var file *os.File
	var err error

	file, err = fs.root.Create(rel(name))
	if nil != err {
		return nil, err
	}

	return file, nil
}

func (fs *rootFs) Open(name string) (afero.File, error) {

	var file *os.File
	var err error

	file, err = fs.root.Open(rel(name))
	if nil != err {
		return nil, err
	}

	return file, nil
}

func (fs *rootFs) OpenFile(name string, flag int, perm os.FileMode) (afero.File, error) {

	var file *os.File
	var err error

	file, err = fs.root.OpenFile(rel(name), flag, perm)
	if nil != err {
		return nil, err
	}

	return file, nil
}

func (fs *rootFs) Mkdir(name string, perm os.FileMode) error {

	return fs.root.Mkdir(rel(name), perm)
}

func (fs *rootFs) MkdirAll(name string, perm os.FileMode) error {

	return fs.root.MkdirAll(rel(name), perm)
}

func (fs *rootFs) Remove(name string) error {

	return fs.root.Remove(rel(name))
}

func (fs *rootFs) RemoveAll(name string) error {

	return fs.root.RemoveAll(rel(name))
}

func (fs *rootFs) Rename(oldName string, newName string) error {

	return fs.root.Rename(rel(oldName), rel(newName))
}

func (fs *rootFs) Stat(name string) (os.FileInfo, error) {

	return fs.root.Stat(rel(name))
}

func (fs *rootFs) Name() string {

	return "rootFs"
}

func (fs *rootFs) Close() error {

	return fs.root.Close()
}

func (fs *rootFs) Chmod(name string, mode os.FileMode) error {

	return fs.root.Chmod(rel(name), mode)
}

func (fs *rootFs) Chown(name string, uid int, gid int) error {

	return fs.root.Chown(rel(name), uid, gid)
}

func (fs *rootFs) Chtimes(name string, atime time.Time, mtime time.Time) error {

	return fs.root.Chtimes(rel(name), atime, mtime)
}
