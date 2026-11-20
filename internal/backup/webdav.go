package backup

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"
)

// WebDAVTarget is a folder on a WebDAV server (Nextcloud, ownCloud, many NAS boxes), reached with
// a user name and an app password over https.
type WebDAVTarget struct {
	URL      string `json:"url"` // https://cloud.example.com/remote.php/dav/files/me/palworld/
	User     string `json:"user"`
	Password string `json:"password"` // an app password; never sent to the browser
}

// Check explains what's wrong with a target, or returns nil.
func (t WebDAVTarget) Check() error {
	u, err := url.Parse(t.URL)
	switch {
	case err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http"):
		return errors.New("a WebDAV place looks like https://cloud.example.com/remote.php/dav/files/me/palworld/")
	case u.Scheme == "http" && !privateHost(u.Hostname()):
		return errors.New("WebDAV over plain http is only allowed to this machine or your own network")
	case u.User != nil:
		return errors.New("put the WebDAV user name and password in their own fields, not in the address")
	case t.User == "" || t.Password == "":
		return errors.New("a WebDAV place needs a user name and an (app) password")
	}
	return nil
}

// Name is the place for messages (no password).
func (t WebDAVTarget) Name() string {
	if u, err := url.Parse(t.URL); err == nil {
		return "webdav://" + u.Host + u.Path
	}
	return "WebDAV"
}

func (t WebDAVTarget) fileURL(name string) string {
	u, _ := url.Parse(t.URL)
	u.Path = path.Join(u.Path, name)
	return u.String()
}

func (t WebDAVTarget) req(ctx context.Context, method, target string, body io.Reader, size int64, hdr map[string]string) (*http.Response, error) {
	r, err := http.NewRequestWithContext(ctx, method, target, body)
	if err != nil {
		return nil, err
	}
	if body != nil {
		r.ContentLength = size
	}
	r.SetBasicAuth(t.User, t.Password)
	for k, v := range hdr {
		r.Header.Set(k, v)
	}
	resp, err := s3Client.Do(r)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 300 && resp.StatusCode != 207 {
		resp.Body.Close()
		if resp.StatusCode == 401 {
			return nil, fmt.Errorf("%s refused the user name or password", t.Name())
		}
		return nil, fmt.Errorf("%s %s: %s", method, t.Name(), resp.Status)
	}
	return resp, nil
}

// SendVerified uploads an archive (as .part, then moved into place) and reads it back to check.
func (t WebDAVTarget) SendVerified(ctx context.Context, a Archive) error {
	if err := t.Check(); err != nil {
		return err
	}
	if resp, err := t.req(ctx, "MKCOL", t.URL, nil, 0, nil); err == nil { // the folder may already exist
		resp.Body.Close()
	}
	want, err := fileSHA(a.Path)
	if err != nil {
		return err
	}
	f, err := os.Open(a.Path)
	if err != nil {
		return err
	}
	st, _ := f.Stat()
	part := t.fileURL(a.Name + ".part")
	resp, err := t.req(ctx, http.MethodPut, part, f, st.Size(), nil)
	f.Close()
	if err != nil {
		return fmt.Errorf("sending %s failed: %w", a.Name, err)
	}
	resp.Body.Close()
	resp, err = t.req(ctx, http.MethodGet, part, nil, 0, nil)
	if err != nil {
		return fmt.Errorf("reading back %s failed: %w", a.Name, err)
	}
	h := sha256.New()
	_, err = io.Copy(h, resp.Body)
	resp.Body.Close()
	if err != nil || hex.EncodeToString(h.Sum(nil)) != want {
		if r, e := t.req(ctx, http.MethodDelete, part, nil, 0, nil); e == nil {
			r.Body.Close()
		}
		return fmt.Errorf("the copy of %s on %s doesn't match (read back and checked); removed", a.Name, t.Name())
	}
	resp, err = t.req(ctx, "MOVE", part, nil, 0, map[string]string{"Destination": t.fileURL(a.Name), "Overwrite": "T"})
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

// List returns the file names in the folder.
func (t WebDAVTarget) List(ctx context.Context) ([]string, error) {
	resp, err := t.req(ctx, "PROPFIND", t.URL, strings.NewReader(`<?xml version="1.0"?><d:propfind xmlns:d="DAV:"><d:prop><d:resourcetype/></d:prop></d:propfind>`), -1, map[string]string{"Depth": "1", "Content-Type": "application/xml"})
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var ms struct {
		R []struct {
			Href string    `xml:"href"`
			Dir  *struct{} `xml:"propstat>prop>resourcetype>collection"`
		} `xml:"response"`
	}
	if err := xml.NewDecoder(io.LimitReader(resp.Body, 16<<20)).Decode(&ms); err != nil {
		return nil, err
	}
	var out []string
	for _, r := range ms.R {
		if r.Dir != nil {
			continue
		}
		p, err := url.PathUnescape(r.Href)
		if err != nil {
			continue
		}
		out = append(out, path.Base(p))
	}
	return out, nil
}

// Mirror applies retention there: keepFn decides from the place's own listing (only this tag's
// Warden-named files are ever deleted).
func (t WebDAVTarget) Mirror(ctx context.Context, tag string, keepFn func(names []string) map[string]bool) (int, error) {
	names, err := t.List(ctx)
	if err != nil {
		return 0, err
	}
	keep := keepFn(names)
	n := 0
	for _, name := range names {
		if !nameRe.MatchString(name) || !strings.HasPrefix(name, tag+"-") || keep[name] {
			continue
		}
		if resp, err := t.req(ctx, http.MethodDelete, t.fileURL(name), nil, 0, nil); err == nil {
			resp.Body.Close()
			n++
		}
	}
	return n, nil
}

// Remote is a backup place on another machine or in cloud storage.
type Remote interface {
	Name() string
	SendVerified(ctx context.Context, a Archive) error
	Mirror(ctx context.Context, tag string, keepFn func(names []string) map[string]bool) (int, error)
}
