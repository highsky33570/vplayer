package media

import (
	"context"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// HeadObjectSize performs a SigV4 HEAD and returns object size when present.
func HeadObjectSize(ctx context.Context, endpoint, bucket, accessKey, secretKey, region, key string) (size int64, exists bool, err error) {
	key = strings.TrimLeft(key, "/")
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
		"HEAD",
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
	signingKey := aws4SigningKey(secretKey, dateStamp, region, "s3")
	signature := hex.EncodeToString(hmacSHA256(signingKey, []byte(stringToSign)))
	auth := fmt.Sprintf("AWS4-HMAC-SHA256 Credential=%s/%s, SignedHeaders=%s, Signature=%s",
		accessKey, credentialScope, signedHeaders, signature)

	req, err := http.NewRequestWithContext(ctx, http.MethodHead, fullURL, nil)
	if err != nil {
		return 0, false, err
	}
	req.Host = host
	req.Header.Set("x-amz-content-sha256", payloadHash)
	req.Header.Set("x-amz-date", amzDate)
	req.Header.Set("Authorization", auth)

	resp, err := s3HTTPClient().Do(req)
	if err != nil {
		return 0, false, err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 512))

	switch resp.StatusCode {
	case http.StatusOK:
		cl := resp.Header.Get("Content-Length")
		if cl == "" {
			return 0, true, nil
		}
		n, err := strconv.ParseInt(cl, 10, 64)
		if err != nil {
			return 0, true, nil
		}
		return n, true, nil
	case http.StatusNotFound:
		return 0, false, nil
	default:
		return 0, false, &S3APIError{
			Op:         "head",
			Key:        key,
			StatusCode: resp.StatusCode,
			Message:    SanitizeS3Message(resp.Status),
		}
	}
}
