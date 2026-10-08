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
	Endpoint      string
	Bucket        string
	AccessKey     string
	SecretKey     string
	Region        string
	PublicBase    string
	LocalFallback *LocalStore
	putAttemptLog PutAttemptLogger
}

func NewR2Store(endpoint, bucket, access, secret, publicBase string, fallback *LocalStore) *R2Store {
	return NewR2StoreRegion(endpoint, bucket, access, secret, "auto", publicBase, fallback)
}

func NewR2StoreRegion(endpoint, bucket, access, secret, region, publicBase string, fallback *LocalStore) *R2Store {
	if region == "" {
		region = "auto"
	}
	return &R2Store{
		Endpoint:      strings.TrimRight(endpoint, "/"),
		Bucket:        bucket,
		AccessKey:     access,
		SecretKey:     secret,
		Region:        region,
		PublicBase:    strings.TrimRight(publicBase, "/"),
		LocalFallback: fallback,
	}
}

func (s *R2Store) configured() bool {
	return s.Endpoint != "" && s.Bucket != "" && s.AccessKey != "" && s.SecretKey != ""
}

func (s *R2Store) Exists(ctx context.Context, key string) (bool, error) {
	_, exists, err := s.Head(ctx, key)
	return exists, err
}

// Head returns remote object size when present (R2/S3 HEAD).
func (s *R2Store) Head(ctx context.Context, key string) (size int64, exists bool, err error) {
	if !s.configured() {
		if s.LocalFallback != nil {
			p := s.LocalFallback.pathFor(key)
			st, err := os.Stat(p)
			if err != nil {
				if os.IsNotExist(err) {
					return 0, false, nil
				}
				return 0, false, err
			}
			return st.Size(), true, nil
		}
		return 0, false, nil
	}
	return HeadObjectSize(ctx, s.Endpoint, s.Bucket, s.AccessKey, s.SecretKey, s.Region, key)
}

// SetPutAttemptLogger receives sanitized per-attempt Put failures during retries.
func (s *R2Store) SetPutAttemptLogger(l PutAttemptLogger) {
	s.putAttemptLog = l
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
	return putS3CompatibleRegionWithRetry(ctx, s.Endpoint, s.Bucket, s.AccessKey, s.SecretKey, s.Region, key, body, contentType, defaultS3PutMaxAttempts, s.putAttemptLog)
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
