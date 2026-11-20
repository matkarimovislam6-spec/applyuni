// Package storage stores already-encrypted blobs. It never sees plaintext.
package storage

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type Store interface {
	Put(ctx context.Context, key string, data []byte) error
	Get(ctx context.Context, key string) ([]byte, error)
	Delete(ctx context.Context, key string) error
}

// ---------- Local filesystem (development) ----------

type Local struct{ dir string }

func NewLocal(dir string) (*Local, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	return &Local{dir: dir}, nil
}

func (l *Local) path(key string) (string, error) {
	if strings.Contains(key, "..") {
		return "", errors.New("invalid key")
	}
	return filepath.Join(l.dir, filepath.FromSlash(key)), nil
}

func (l *Local) Put(_ context.Context, key string, data []byte) error {
	p, err := l.path(key)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return err
	}
	return os.WriteFile(p, data, 0o600)
}

func (l *Local) Get(_ context.Context, key string) ([]byte, error) {
	p, err := l.path(key)
	if err != nil {
		return nil, err
	}
	return os.ReadFile(p)
}

func (l *Local) Delete(_ context.Context, key string) error {
	p, err := l.path(key)
	if err != nil {
		return err
	}
	if err := os.Remove(p); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

// ---------- S3-compatible (MinIO in an Uzbek data centre) ----------

type S3 struct {
	c      *minio.Client
	bucket string
}

func NewS3(ctx context.Context, endpoint, access, secret, bucket string, useSSL bool) (*S3, error) {
	c, err := minio.New(endpoint, &minio.Options{Creds: credentials.NewStaticV4(access, secret, ""), Secure: useSSL})
	if err != nil {
		return nil, err
	}
	ok, err := c.BucketExists(ctx, bucket)
	if err != nil {
		return nil, err
	}
	if !ok {
		if err := c.MakeBucket(ctx, bucket, minio.MakeBucketOptions{}); err != nil {
			return nil, err
		}
	}
	return &S3{c: c, bucket: bucket}, nil
}

func (s *S3) Put(ctx context.Context, key string, data []byte) error {
	_, err := s.c.PutObject(ctx, s.bucket, key, bytes.NewReader(data), int64(len(data)), minio.PutObjectOptions{ContentType: "application/octet-stream"})
	return err
}

func (s *S3) Get(ctx context.Context, key string) ([]byte, error) {
	obj, err := s.c.GetObject(ctx, s.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, err
	}
	defer obj.Close()
	return io.ReadAll(obj)
}

func (s *S3) Delete(ctx context.Context, key string) error {
	return s.c.RemoveObject(ctx, s.bucket, key, minio.RemoveObjectOptions{})
}
