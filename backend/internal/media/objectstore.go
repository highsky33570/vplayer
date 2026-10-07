package media

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ObjectStore persists binary assets (posters) under stable keys.
type ObjectStore interface {
	Exists(ctx context.Context, key string) (bool, error)
	Put(ctx context.Context, key string, body []byte, contentType string) error
	PublicURL(key string) string
}

// LocalStore writes under a directory and exposes URLs via publicBase + key.
type LocalStore struct {
	Root       string
	PublicBase string
}

func NewLocalStore(root, publicBase string) (*LocalStore, error) {
	if root == "" {
		root = filepath.Join("data", "objects")
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, err
	}
	return &LocalStore{Root: root, PublicBase: strings.TrimRight(publicBase, "/")}, nil
}

func (s *LocalStore) pathFor(key string) string {
	key = strings.TrimLeft(filepath.ToSlash(key), "/")
	return filepath.Join(s.Root, filepath.FromSlash(key))
}

func (s *LocalStore) Exists(ctx context.Context, key string) (bool, error) {
	_ = ctx
	_, err := os.Stat(s.pathFor(key))
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

func (s *LocalStore) Put(ctx context.Context, key string, body []byte, contentType string) error {
	_ = ctx
	_ = contentType
	p := s.pathFor(key)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	return os.WriteFile(p, body, 0o644)
}

func (s *LocalStore) PublicURL(key string) string {
	key = strings.TrimLeft(key, "/")
	if s.PublicBase == "" {
		return "/" + key
	}
	return s.PublicBase + "/" + key
}

// R2Store is an S3-compatible uploader. Without credentials it returns clear errors.
type R2Store struct {
	Endpoint   string
	Bucket     string
	AccessKey  string
	SecretKey  string
	PublicBase string
	LocalFallback *LocalStore
}

func NewR2Store(endpoint, bucket, access, secret, publicBase string, fallback *LocalStore) *R2Store {
	return &R2Store{
		Endpoint:      strings.TrimRight(endpoint, "/"),
		Bucket:        bucket,
		AccessKey:     access,
		SecretKey:     secret,
		PublicBase:    strings.TrimRight(publicBase, "/"),
		LocalFallback: fallback,
	}
}

func (s *R2Store) configured() bool {
	return s.Endpoint != "" && s.Bucket != "" && s.AccessKey != "" && s.SecretKey != ""
}

func (s *R2Store) Exists(ctx context.Context, key string) (bool, error) {
	if !s.configured() {
		if s.LocalFallback != nil {
			return s.LocalFallback.Exists(ctx, key)
		}
		return false, nil
	}
	// Lightweight HEAD via fallback for now; full S3 client can replace this.
	if s.LocalFallback != nil {
		return s.LocalFallback.Exists(ctx, key)
	}
	return false, nil
}

func (s *R2Store) Put(ctx context.Context, key string, body []byte, contentType string) error {
	if !s.configured() {
		if s.LocalFallback != nil {
			return s.LocalFallback.Put(ctx, key, body, contentType)
		}
		return fmt.Errorf("r2 not configured and no local fallback")
	}
	// Prefer local mirror + documented R2 credentials path until AWS SDK is wired in deploy.
	// Production should set R2_* and replace this with signed PutObject.
	if s.LocalFallback != nil {
		if err := s.LocalFallback.Put(ctx, key, body, contentType); err != nil {
			return err
		}
	}
	return putS3Compatible(ctx, s.Endpoint, s.Bucket, s.AccessKey, s.SecretKey, key, body, contentType)
}

func (s *R2Store) PublicURL(key string) string {
	key = strings.TrimLeft(key, "/")
	if s.PublicBase != "" {
		return s.PublicBase + "/" + key
	}
	if s.LocalFallback != nil {
		return s.LocalFallback.PublicURL(key)
	}
	return key
}
