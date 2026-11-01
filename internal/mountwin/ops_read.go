//go:build windows && !cgo

package mountwin

import (
	"io"
	"os"

	"github.com/winfsp/cgofuse/fuse"
)

func (fs *FileSystem) Getattr(p string, stat *fuse.Stat_t, fh uint64) int {
	abs, _, errc := fs.realPath(p)
	if errc != 0 {
		return errc
	}
	info, err := os.Lstat(abs)
	if err != nil {
		return errno(err)
	}
	*stat = *statOf(info)
	return 0
}

func (fs *FileSystem) Readdir(p string, fill func(name string, stat *fuse.Stat_t, ofst int64) bool, ofst int64, fh uint64) int {
	abs, _, errc := fs.realPath(p)
	if errc != 0 {
		return errc
	}
	entries, err := os.ReadDir(abs)
	if err != nil {
		return errno(err)
	}
	fill(".", nil, 0)
	fill("..", nil, 0)
	for _, entry := range entries {
		if !fill(entry.Name(), nil, 0) {
			break
		}
	}
	return 0
}

func (fs *FileSystem) Open(p string, flags int) (int, uint64) {
	abs, rel, errc := fs.realPath(p)
	if errc != 0 {
		return errc, ^uint64(0)
	}
	write := flags&fuse.O_ACCMODE != fuse.O_RDONLY
	if write {
		if abs, errc = fs.writePath(p, false); errc != 0 {
			return errc, ^uint64(0)
		}
	}
	f, err := os.OpenFile(abs, osFlags(flags), 0)
	if err != nil {
		return errno(err), ^uint64(0)
	}
	fh, errc := fs.addHandle(f, rel, write)
	if errc != 0 {
		_ = f.Close()
		return errc, ^uint64(0)
	}
	return 0, fh
}

func (fs *FileSystem) Read(p string, buff []byte, off int64, fh uint64) int {
	h := fs.handle(fh)
	if h == nil {
		return -fuse.EBADF
	}
	n, err := h.file.ReadAt(buff, off)
	if err != nil && err != io.EOF {
		return -fuse.EIO
	}
	return n
}

func (fs *FileSystem) Release(p string, fh uint64) int {
	h := fs.dropHandle(fh)
	if h == nil {
		return 0
	}
	err := h.file.Close()
	if h.write {
		fs.journalWrite(h.rel, h.bytes)
	}
	return errno(err)
}

func (fs *FileSystem) Readlink(p string) (int, string) {
	abs, _, errc := fs.realPath(p)
	if errc != 0 {
		return errc, ""
	}
	target, err := os.Readlink(abs)
	if err != nil {
		return errno(err), ""
	}
	return 0, target
}

func (fs *FileSystem) Access(p string, mask uint32) int {
	abs, rel, errc := fs.realPath(p)
	if errc != 0 {
		return errc
	}
	info, err := os.Lstat(abs)
	if err != nil {
		return errno(err)
	}
	if mask&2 != 0 && !info.IsDir() { // W_OK
		return fs.allowWrite(rel, false)
	}
	return 0
}

func (fs *FileSystem) Statfs(p string, stat *fuse.Statfs_t) int {
	stat.Bsize, stat.Frsize = 4096, 4096
	stat.Blocks, stat.Bfree, stat.Bavail = 1<<40, 1<<39, 1<<39
	stat.Files, stat.Ffree, stat.Favail, stat.Namemax = 1<<32, 1<<31, 1<<31, 255
	return 0
}

func (fs *FileSystem) Opendir(p string) (int, uint64) {
	abs, _, errc := fs.realPath(p)
	if errc != 0 {
		return errc, ^uint64(0)
	}
	info, err := os.Stat(abs)
	if err != nil {
		return errno(err), ^uint64(0)
	}
	if !info.IsDir() {
		return -fuse.ENOTDIR, ^uint64(0)
	}
	return 0, 0
}

func (fs *FileSystem) Releasedir(p string, fh uint64) int { return 0 }

func (fs *FileSystem) Fsyncdir(p string, datasync bool, fh uint64) int { return 0 }
