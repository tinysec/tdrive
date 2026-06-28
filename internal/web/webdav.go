package web

import (
	"context"
	"net/http"
	"os"

	"github.com/spf13/afero"
	"golang.org/x/net/webdav"
)

// webdavFs adapts the shared afero filesystem to webdav.FileSystem. The mapping
// is one-to-one because afero.File already satisfies webdav.File.
type webdavFs struct {
	fs afero.Fs
}

func newWebdavFs(fs afero.Fs) webdav.FileSystem {

	return webdavFs{fs: fs}
}

func (adapter webdavFs) Mkdir(ctx context.Context, name string, perm os.FileMode) error {

	return adapter.fs.Mkdir(name, perm)
}

func (adapter webdavFs) OpenFile(ctx context.Context, name string, flag int, perm os.FileMode) (webdav.File, error) {

	var file afero.File
	var err error

	file, err = adapter.fs.OpenFile(name, flag, perm)
	if nil != err {
		return nil, err
	}

	// afero.File satisfies the webdav.File interface.
	return file, nil
}

func (adapter webdavFs) RemoveAll(ctx context.Context, name string) error {

	return adapter.fs.RemoveAll(name)
}

func (adapter webdavFs) Rename(ctx context.Context, oldName string, newName string) error {

	return adapter.fs.Rename(oldName, newName)
}

func (adapter webdavFs) Stat(ctx context.Context, name string) (os.FileInfo, error) {

	return adapter.fs.Stat(name)
}

// webdavHandler builds the WebDAV handler mounted under /webdav.
func (server *Server) webdavHandler() http.Handler {

	var handler webdav.Handler

	handler.Prefix = "/webdav"
	handler.FileSystem = newWebdavFs(server.store.Fs())
	handler.LockSystem = webdav.NewMemLS()

	return &handler
}
