package main

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

func readLimited(path string, limit int64) ([]byte, error) {
	f, e := os.Open(path)
	if e != nil {
		return nil, e
	}
	defer f.Close()
	st, e := f.Stat()
	if e != nil {
		return nil, e
	}
	if !st.Mode().IsRegular() || st.Size() > limit {
		return nil, errors.New("Unsupported file type or file too large")
	}
	return io.ReadAll(io.LimitReader(f, limit+1))
}
func canonical(path string) (string, error) {
	p, e := filepath.Abs(path)
	if e != nil {
		return "", e
	}
	return filepath.EvalSymlinks(p)
}
func backup(path string) (string, error) {
	src, e := os.Open(path)
	if e != nil {
		return "", e
	}
	defer src.Close()
	dst, e := os.OpenFile(path+".backup-"+time.Now().Format("20060102-150405.000000000"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if e != nil {
		return "", e
	}
	name := dst.Name()
	_, e = io.Copy(dst, src)
	if e == nil {
		e = dst.Sync()
	}
	ce := dst.Close()
	if e == nil {
		e = ce
	}
	if e != nil {
		os.Remove(name)
		return "", e
	}
	return name, nil
}
func replaceChecked(path string, data []byte, expected [32]byte) error {
	st, e := os.Stat(path)
	if e != nil {
		return e
	}
	tmp, e := os.CreateTemp(filepath.Dir(path), ".dimraeth-*")
	if e != nil {
		return e
	}
	name := tmp.Name()
	defer os.Remove(name)
	if e = tmp.Chmod(st.Mode().Perm()); e == nil {
		_, e = tmp.Write(data)
	}
	if e == nil {
		e = tmp.Sync()
	}
	ce := tmp.Close()
	if e == nil {
		e = ce
	}
	if e != nil {
		return e
	}
	current, e := readLimited(path, 512<<20)
	if e != nil {
		return e
	}
	if sha256.Sum256(current) != expected {
		return errors.New("The file has changed on disk. Reload it before saving")
	}
	if e = replaceFile(name, path); e != nil {
		return fmt.Errorf("Could not replace the file (close the game): %w", e)
	}
	return nil
}
