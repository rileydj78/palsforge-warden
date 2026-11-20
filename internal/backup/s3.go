package backup

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"
)

// S3Target is a bucket on Amazon S3 or any S3-compatible service (Backblaze B2, Wasabi,
// Cloudflare R2, MinIO). Requests are signed with AWS Signature Version 4; no SDK.
type S3Target struct {
	Endpoint  string `json:"endpoint"` // https://s3.us-east-1.amazonaws.com, https://<account>.r2.cloudflarestorage.com ...
	Region    string `json:"region"`   // us-east-1 (R2: auto)
	Bucket    string `json:"bucket"`
	Prefix    string `json:"prefix"` // folder inside the bucket, e.g. palworld/
	AccessKey string `json:"accessKey"`
	SecretKey string `json:"secretKey"` // never sent to the browser
}

// Check explains what's wrong with a target, or returns nil.
func (t S3Target) Check() error {
	u, err := url.Parse(t.Endpoint)
	switch {
	case err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http"):
		return errors.New("an S3 endpoint looks like https://s3.us-east-1.amazonaws.com")
	case u.Scheme == "http" && !privateHost(u.Hostname()):
		return errors.New("S3 over plain http is only allowed to this machine or your own network")
	case t.Bucket == "" || strings.ContainsAny(t.Bucket, "/ "):
		return errors.New("an S3 place needs a bucket name")
	case t.AccessKey == "" || t.SecretKey == "":
		return errors.New("an S3 place needs an access key and a secret key")
	case t.Region == "":
		return errors.New("an S3 place needs a region (us-east-1; auto for Cloudflare R2)")
	}
	return nil
}

func privateHost(h string) bool {
	return h == "localhost" || strings.HasPrefix(h, "127.") || strings.HasPrefix(h, "10.") || strings.HasPrefix(h, "192.168.") ||
		strings.HasPrefix(h, "100.") || (strings.HasPrefix(h, "172.") && len(h) > 6)
}

// Name is the place for messages (no keys).
func (t S3Target) Name() string {
	u, _ := url.Parse(t.Endpoint)
	host := t.Endpoint
	if u != nil {
		host = u.Host
	}
	return "s3://" + host + "/" + t.Bucket + "/" + t.prefix()
}

func (t S3Target) prefix() string {
	p := strings.Trim(t.Prefix, "/")
	if p != "" {
		p += "/"
	}
	return p
}

var s3Client = &http.Client{Timeout: 0} // transfers can be long; each request has its own context

// do signs and sends one request. body may be nil; bodyHash is its SHA-256 (hex).
func (t S3Target) do(ctx context.Context, method, key string, query url.Values, body io.Reader, size int64, bodyHash string, extra map[string]string) (*http.Response, error) {
	u, err := url.Parse(strings.TrimRight(t.Endpoint, "/"))
	if err != nil {
		return nil, err
	}
	path := "/" + t.Bucket
	if key != "" {
		path += "/" + key
	}
	u.Path = path
	u.RawPath = uriEncode(path, false)
	u.RawQuery = canonicalQuery(query)
	if bodyHash == "" {
		bodyHash = emptySHA
	}
	req, err := http.NewRequestWithContext(ctx, method, u.String(), body)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.ContentLength = size
	}
	now := time.Now().UTC()
	amzDate := now.Format("20060102T150405Z")
	day := now.Format("20060102")
	req.Header.Set("x-amz-date", amzDate)
	req.Header.Set("x-amz-content-sha256", bodyHash)
	for k, v := range extra {
		req.Header.Set(k, v)
	}
	// Sign host and every x-amz-* header.
	names := []string{"host"}
	vals := map[string]string{"host": u.Host}
	for k := range req.Header {
		lk := strings.ToLower(k)
		if strings.HasPrefix(lk, "x-amz-") {
			names = append(names, lk)
			vals[lk] = strings.TrimSpace(req.Header.Get(k))
		}
	}
	sort.Strings(names)
	var ch strings.Builder
	for _, n := range names {
		ch.WriteString(n + ":" + vals[n] + "\n")
	}
	signed := strings.Join(names, ";")
	creq := strings.Join([]string{method, uriEncode(path, false), u.RawQuery, ch.String(), signed, bodyHash}, "\n")
	scope := day + "/" + t.Region + "/s3/aws4_request"
	sts := "AWS4-HMAC-SHA256\n" + amzDate + "\n" + scope + "\n" + hexSHA([]byte(creq))
	k := hmacSHA([]byte("AWS4"+t.SecretKey), day)
	k = hmacSHA(k, t.Region)
	k = hmacSHA(k, "s3")
	k = hmacSHA(k, "aws4_request")
	sig := hex.EncodeToString(hmacSHA(k, sts))
	req.Header.Set("Authorization", fmt.Sprintf("AWS4-HMAC-SHA256 Credential=%s/%s, SignedHeaders=%s, Signature=%s", t.AccessKey, scope, signed, sig))
	resp, err := s3Client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		resp.Body.Close()
		var e struct {
			Code    string `xml:"Code"`
			Message string `xml:"Message"`
		}
		xml.Unmarshal(b, &e)
		if e.Code == "" {
			e.Code = resp.Status
		}
		return nil, fmt.Errorf("%s %s: %s %s", method, t.Name(), e.Code, e.Message)
	}
	return resp, nil
}

const emptySHA = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"

func hexSHA(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }

func hmacSHA(key []byte, s string) []byte {
	m := hmac.New(sha256.New, key)
	m.Write([]byte(s))
	return m.Sum(nil)
}

// uriEncode is SigV4's encoding: unreserved characters kept, everything else %XX (slashes kept
// in paths).
func uriEncode(s string, encodeSlash bool) string {
	var b strings.Builder
	for _, c := range []byte(s) {
		switch {
		case c >= 'A' && c <= 'Z', c >= 'a' && c <= 'z', c >= '0' && c <= '9', c == '-', c == '_', c == '.', c == '~':
			b.WriteByte(c)
		case c == '/' && !encodeSlash:
			b.WriteByte(c)
		default:
			fmt.Fprintf(&b, "%%%02X", c)
		}
	}
	return b.String()
}

func canonicalQuery(q url.Values) string {
	if len(q) == 0 {
		return ""
	}
	keys := make([]string, 0, len(q))
	for k := range q {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var parts []string
	for _, k := range keys {
		for _, v := range q[k] {
			parts = append(parts, uriEncode(k, true)+"="+uriEncode(v, true))
		}
	}
	return strings.Join(parts, "&")
}

// SendVerified uploads an archive with its SHA-256 (the service checks it on arrival), then
// downloads the copy and checks it again.
func (t S3Target) SendVerified(ctx context.Context, a Archive) error {
	if err := t.Check(); err != nil {
		return err
	}
	st, err := os.Stat(a.Path)
	if err != nil {
		return err
	}
	if st.Size() > 5<<30 {
		return fmt.Errorf("%s is over 5 GB, the limit for one S3 upload; use a folder or SFTP place for whole-server images", a.Name)
	}
	sum, err := fileSHA(a.Path)
	if err != nil {
		return err
	}
	raw, _ := hex.DecodeString(sum)
	f, err := os.Open(a.Path)
	if err != nil {
		return err
	}
	defer f.Close()
	key := t.prefix() + a.Name
	resp, err := t.do(ctx, http.MethodPut, key, nil, f, st.Size(), sum, map[string]string{
		"x-amz-checksum-sha256": base64.StdEncoding.EncodeToString(raw), // the service refuses a damaged upload
	})
	if err != nil {
		return fmt.Errorf("sending %s failed: %w", a.Name, err)
	}
	resp.Body.Close()
	resp, err = t.do(ctx, http.MethodGet, key, nil, nil, 0, "", nil)
	if err != nil {
		return fmt.Errorf("reading back %s failed: %w", a.Name, err)
	}
	defer resp.Body.Close()
	h := sha256.New()
	if _, err := io.Copy(h, resp.Body); err != nil {
		return fmt.Errorf("reading back %s failed: %w", a.Name, err)
	}
	if hex.EncodeToString(h.Sum(nil)) != sum {
		t.do(ctx, http.MethodDelete, key, nil, nil, 0, "", nil)
		return fmt.Errorf("the copy of %s in %s doesn't match (read back and checked); removed", a.Name, t.Name())
	}
	return nil
}

// List returns the object names under the prefix (names only, without the prefix).
func (t S3Target) List(ctx context.Context) ([]string, error) {
	var out []string
	token := ""
	for {
		q := url.Values{"list-type": {"2"}, "prefix": {t.prefix()}}
		if token != "" {
			q.Set("continuation-token", token)
		}
		resp, err := t.do(ctx, http.MethodGet, "", q, nil, 0, "", nil)
		if err != nil {
			return nil, err
		}
		var r struct {
			Contents []struct {
				Key string `xml:"Key"`
			} `xml:"Contents"`
			IsTruncated bool   `xml:"IsTruncated"`
			Next        string `xml:"NextContinuationToken"`
		}
		err = xml.NewDecoder(io.LimitReader(resp.Body, 32<<20)).Decode(&r)
		resp.Body.Close()
		if err != nil {
			return nil, err
		}
		for _, c := range r.Contents {
			out = append(out, strings.TrimPrefix(c.Key, t.prefix()))
		}
		if !r.IsTruncated || r.Next == "" {
			return out, nil
		}
		token = r.Next
	}
}

// Mirror deletes this tag's archives in the bucket that are no longer kept here. It only ever
// touches objects named like Warden's backups, directly under the prefix.
func (t S3Target) Mirror(ctx context.Context, tag string, keep map[string]bool) (int, error) {
	names, err := t.List(ctx)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, name := range names {
		if strings.Contains(name, "/") || !nameRe.MatchString(name) || !strings.HasPrefix(name, tag+"-") || keep[name] {
			continue
		}
		if resp, err := t.do(ctx, http.MethodDelete, t.prefix()+name, nil, nil, 0, "", nil); err == nil {
			resp.Body.Close()
			n++
		}
	}
	return n, nil
}
