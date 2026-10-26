package media

import (
	"context"
	"encoding/hex"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// s3GetClient has no overall timeout so a large object can stream.
// ResponseHeaderTimeout bounds a stalled handshake without cutting the body.
var s3GetClient = &http.Client{
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	},
	Transport: &http.Transport{
		ResponseHeaderTimeout: 30 * time.Second,
		IdleConnTimeout:       90 * time.Second,
		MaxIdleConns:          32,
		MaxIdleConnsPerHost:   8,
	},
}

// ObjectBody is a streamed private object. Caller closes Body.
type ObjectBody struct {
	StatusCode    int
	ContentType   string
	ContentLength int64
	ContentRange  string
	Body          io.ReadCloser
}

// OpenPrivateObject reads one object from R2 when configured, otherwise the local object dir.
// rangeHeader is forwarded as an HTTP Range (for example bytes=0-1023). It is not part of the signature.
func OpenPrivateObject(ctx context.Context, endpoint, bucket, accessKey, secretKey, region, localRoot, key, rangeHeader string) (*ObjectBody, error) {
	key = strings.TrimLeft(key, "/")
	if endpoint != "" && bucket != "" && accessKey != "" && secretKey != "" {
		return getS3Object(ctx, endpoint, bucket, accessKey, secretKey, region, key, rangeHeader)
	}
	if localRoot == "" {
		return nil, &S3APIError{Op: "get", Key: key, StatusCode: http.StatusNotFound, Message: "not_found"}
	}
	return openLocalObject(localRoot, key, rangeHeader)
}

func getS3Object(ctx context.Context, endpoint, bucket, accessKey, secretKey, region, key, rangeHeader string) (*ObjectBody, error) {
	if region == "" {
		region = "auto"
	}
	host := strings.TrimPrefix(strings.TrimPrefix(strings.TrimRight(endpoint, "/"), "https://"), "http://")
	canonicalURI := "/" + bucket + "/" + key
	fullURL := strings.TrimRight(endpoint, "/") + canonicalURI
	now := time.Now().UTC()
	amzDate := now.Format("20060102T150405Z")
	dateStamp := now.Format("20060102")
	payloadHash := sha256Hex(nil)
	canonicalHeaders := fmt.Sprintf("host:%s\nx-amz-content-sha256:%s\nx-amz-date:%s\n", host, payloadHash, amzDate)
	signedHeaders := "host;x-amz-content-sha256;x-amz-date"
	canonicalRequest := strings.Join([]string{
		"GET",
		canonicalURI,
		"",
		canonicalHeaders,
		signedHeaders,
		payloadHash,
	}, "\n")
	credentialScope := fmt.Sprintf("%s/%s/s3/aws4_request", dateStamp, region)
	stringToSign := strings.Join([]string{
		"AWS4-HMAC-SHA256",
		amzDate,
		credentialScope,
		sha256Hex([]byte(canonicalRequest)),
	}, "\n")
	signature := hex.EncodeToString(hmacSHA256(aws4SigningKey(secretKey, dateStamp, region, "s3"), []byte(stringToSign)))
	auth := fmt.Sprintf("AWS4-HMAC-SHA256 Credential=%s/%s, SignedHeaders=%s, Signature=%s",
		accessKey, credentialScope, signedHeaders, signature)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fullURL, nil)
	if err != nil {
		return nil, err
	}
	req.Host = host
	req.Header.Set("x-amz-content-sha256", payloadHash)
	req.Header.Set("x-amz-date", amzDate)
	req.Header.Set("Authorization", auth)
	if rangeHeader != "" {
		req.Header.Set("Range", rangeHeader)
	}
	resp, err := s3GetClient.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		defer resp.Body.Close()
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 512))
		if resp.StatusCode == http.StatusNotFound {
			return nil, &S3APIError{Op: "get", Key: key, StatusCode: http.StatusNotFound, Message: "not_found"}
		}
		return nil, &S3APIError{Op: "get", Key: key, StatusCode: resp.StatusCode, Message: SanitizeS3Message(resp.Status)}
	}
	return &ObjectBody{
		StatusCode:    resp.StatusCode,
		ContentType:   resp.Header.Get("Content-Type"),
		ContentLength: resp.ContentLength,
		ContentRange:  resp.Header.Get("Content-Range"),
		Body:          resp.Body,
	}, nil
}

func openLocalObject(root, key, rangeHeader string) (*ObjectBody, error) {
	store, err := NewLocalStore(root, "")
	if err != nil {
		return nil, err
	}
	path, err := confinedObjectPath(store.Root, key)
	if err != nil {
		return nil, &S3APIError{Op: "get", Key: key, StatusCode: http.StatusNotFound, Message: "not_found"}
	}
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, &S3APIError{Op: "get", Key: key, StatusCode: http.StatusNotFound, Message: "not_found"}
		}
		return nil, err
	}
	st, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, err
	}
	size := st.Size()
	start, end, partial, err := parseByteRange(rangeHeader, size)
	if err != nil {
		f.Close()
		return nil, &S3APIError{Op: "get", Key: key, StatusCode: http.StatusRequestedRangeNotSatisfiable, Message: "bad_range"}
	}
	if _, err := f.Seek(start, io.SeekStart); err != nil {
		f.Close()
		return nil, err
	}
	length := end - start + 1
	status := http.StatusOK
	cr := ""
	if partial {
		status = http.StatusPartialContent
		cr = fmt.Sprintf("bytes %d-%d/%d", start, end, size)
	}
	return &ObjectBody{
		StatusCode:    status,
		ContentLength: length,
		ContentRange:  cr,
		Body:          &limitedFile{File: f, remain: length},
	}, nil
}

type limitedFile struct {
	*os.File
	remain int64
}

func (l *limitedFile) Read(p []byte) (int, error) {
	if l.remain <= 0 {
		return 0, io.EOF
	}
	if int64(len(p)) > l.remain {
		p = p[:l.remain]
	}
	n, err := l.File.Read(p)
	l.remain -= int64(n)
	return n, err
}

func parseByteRange(spec string, size int64) (start, end int64, partial bool, err error) {
	if spec == "" {
		if size == 0 {
			return 0, -1, false, nil
		}
		return 0, size - 1, false, nil
	}
	spec = strings.TrimSpace(spec)
	if !strings.HasPrefix(spec, "bytes=") || strings.Contains(spec, ",") {
		return 0, 0, false, fmt.Errorf("bad range")
	}
	spec = strings.TrimPrefix(spec, "bytes=")
	left, right, ok := strings.Cut(spec, "-")
	if !ok {
		return 0, 0, false, fmt.Errorf("bad range")
	}
	if left == "" {
		n, err := parseNonNeg(right)
		if err != nil || n <= 0 || n > size {
			return 0, 0, false, fmt.Errorf("bad range")
		}
		return size - n, size - 1, true, nil
	}
	start, err = parseNonNeg(left)
	if err != nil || start >= size {
		return 0, 0, false, fmt.Errorf("bad range")
	}
	end = size - 1
	if right != "" {
		end, err = parseNonNeg(right)
		if err != nil || end < start {
			return 0, 0, false, fmt.Errorf("bad range")
		}
		if end >= size {
			end = size - 1
		}
	}
	return start, end, true, nil
}

func confinedObjectPath(root, key string) (string, error) {
	key = strings.TrimLeft(strings.ReplaceAll(key, "\\", "/"), "/")
	if key == "" || strings.Contains(key, "..") {
		return "", fmt.Errorf("bad key")
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	full := filepath.Join(absRoot, filepath.FromSlash(key))
	absFull, err := filepath.Abs(full)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(absRoot, absFull)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("bad key")
	}
	return absFull, nil
}

func parseNonNeg(s string) (int64, error) {
	var n int64
	if s == "" {
		return 0, fmt.Errorf("bad")
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0, fmt.Errorf("bad")
		}
		d := int64(r - '0')
		if n > (math.MaxInt64-d)/10 {
			return 0, fmt.Errorf("bad")
		}
		n = n*10 + d
	}
	return n, nil
}
