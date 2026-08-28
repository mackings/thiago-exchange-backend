// Package storage handles payment-proof and KYC document uploads. Local is
// a disk-backed implementation for dev/self-hosting; swap in an S3-compatible
// implementation of the same Storage interface for production without
// touching callers.
package storage

import (
	"fmt"
	"io"
	"mime/multipart"
	"os"
	"path/filepath"

	"github.com/google/uuid"
)

type Storage interface {
	Save(file multipart.File, header *multipart.FileHeader) (url string, err error)
}

type Local struct {
	dir       string
	publicURL string
}

func NewLocal(dir, publicURL string) (*Local, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	return &Local{dir: dir, publicURL: publicURL}, nil
}

func (l *Local) Save(file multipart.File, header *multipart.FileHeader) (string, error) {
	ext := filepath.Ext(header.Filename)
	name := uuid.New().String() + ext
	dst, err := os.Create(filepath.Join(l.dir, name))
	if err != nil {
		return "", err
	}
	defer dst.Close()
	if _, err := io.Copy(dst, file); err != nil {
		return "", err
	}
	return fmt.Sprintf("%s/%s", l.publicURL, name), nil
}
