//go:build windows && !cgo

package mountwin

import (
	"os"
	"time"

	"github.com/winfsp/cgofuse/fuse"
)

func (fs *FileSystem) Create(p string, flags int, mode uint32) (int, uint64) {
	abs, rel, errc := fs.realPath(p)
	if errc != 0 {
		return errc, ^uint64(0)
	}
	if abs, errc = fs.writePath(p, false); errc != 0 {
		return errc, ^uint64(0)
	}
	f, err := os.OpenFile(abs, osFlags(flags)|os.O_CREATE, os.FileMode(mode).Perm())
	if err != nil {
		return errno(err), ^uint64(0)
	}
	fh, errc := fs.addHandle(f, rel, true)
	if errc != 0 {
		_ = f.Close()
		return errc, ^uint64(0)
	}
	return 0, fh
}

func (fs *FileSystem) Write(p string, buff []byte, off int64, fh uint64) int {
	h := fs.handle(fh)
	if h == nil {
		return -fuse.EBADF
	}
	n, err := h.file.WriteAt(buff, off)
	if err != nil && n == 0 {
		return -fuse.EIO
	}
	fs.mu.Lock()
	h.bytes += n
	fs.mu.Unlock()
	return n
}

func (fs *FileSystem) Truncate(p string, size int64, fh uint64) int {
	abs, rel, errc := fs.realPath(p)
	if errc != 0 {
		return errc
	}
	if abs, errc = fs.writePath(p, false); errc != 0 {
		return errc
	}
	if h := fs.handle(fh); h != nil {
		errc = errno(h.file.Truncate(size))
	} else {
		errc = errno(os.Truncate(abs, size))
	}
	if errc == 0 {
		fs.journalWrite(rel, int(size))
	}
	return errc
}

func (fs *FileSystem) Mkdir(p string, mode uint32) int {
	_, rel, _ := fs.realPath(p)
	abs, errc := fs.writePath(p, true)
	if errc != 0 {
		return errc
	}
	if err := os.Mkdir(abs, os.FileMode(mode).Perm()); err != nil {
		return errno(err)
	}
	fs.journalKind("write", rel, "mkdir")
	return 0
}

func (fs *FileSystem) Rmdir(p string) int {
	_, rel, _ := fs.realPath(p)
	abs, errc := fs.writePath(p, true)
	if errc != 0 {
		return errc
	}
	if err := os.Remove(abs); err != nil {
		return errno(err)
	}
	fs.journalKind("unlink", rel, "")
	return 0
}

func (fs *FileSystem) Unlink(p string) int {
	_, rel, _ := fs.realPath(p)
	abs, errc := fs.writePath(p, false)
	if errc != 0 {
		return errc
	}
	if err := os.Remove(abs); err != nil {
		return errno(err)
	}
	fs.journalKind("unlink", rel, "")
	return 0
}

func (fs *FileSystem) Rename(oldp, newp string) int {
	_, oldRel, _ := fs.realPath(oldp)
	_, newRel, _ := fs.realPath(newp)
	oldAbs, errc := fs.writePath(oldp, false)
	if errc != 0 {
		return errc
	}
	newAbs, errc := fs.writePath(newp, false)
	if errc != 0 {
		return errc
	}
	if err := os.Rename(oldAbs, newAbs); err != nil {
		return errno(err)
	}
	fs.journalKind("write", newRel, "moved from "+oldRel)
	return 0
}

func (fs *FileSystem) Chmod(p string, mode uint32) int {
	abs, errc := fs.writePath(p, false)
	if errc != 0 {
		return errc
	}
	return errno(os.Chmod(abs, os.FileMode(mode).Perm()))
}

func (fs *FileSystem) Chown(p string, uid, gid uint32) int { return 0 }

func (fs *FileSystem) Utimens(p string, tmsp []fuse.Timespec) int {
	abs, errc := fs.writePath(p, false)
	if errc != 0 {
		return errc
	}
	if len(tmsp) < 2 {
		now := time.Now()
		return errno(os.Chtimes(abs, now, now))
	}
	return errno(os.Chtimes(abs, tmsp[0].Time(), tmsp[1].Time()))
}

func (fs *FileSystem) Link(oldp, newp string) int {
	oldAbs, errc := fs.writePath(oldp, false)
	if errc != 0 {
		return errc
	}
	newAbs, errc := fs.writePath(newp, false)
	if errc != 0 {
		return errc
	}
	return errno(os.Link(oldAbs, newAbs))
}

func (fs *FileSystem) Symlink(target, newp string) int {
	newAbs, errc := fs.writePath(newp, false)
	if errc != 0 {
		return errc
	}
	return errno(os.Symlink(target, newAbs))
}

func (fs *FileSystem) Flush(p string, fh uint64) int {
	if fs.handle(fh) == nil {
		return -fuse.EBADF
	}
	return 0
}

func (fs *FileSystem) Fsync(p string, datasync bool, fh uint64) int {
	if h := fs.handle(fh); h != nil {
		return errno(h.file.Sync())
	}
	return 0
}
