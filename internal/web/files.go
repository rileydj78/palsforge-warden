package web

import (
	"errors"
	"mime"
	"net/http"
	"path"
	"strings"

	"github.com/rileydj78/palsforge-warden/internal/auth"
	"github.com/rileydj78/palsforge-warden/internal/files"
)

// File manager: the owner only (config files can hold passwords), every change audited.
func (s *Server) fileRoutes(mux *http.ServeMux) {
	if s.Control == nil || s.Settings == nil {
		return
	}
	m := &files.Manager{Roots: files.Roots(s.Control.Cfg.ServerDir), DataDir: s.Settings.DataDir}
	q := func(r *http.Request) (string, string) { return r.URL.Query().Get("root"), r.URL.Query().Get("path") }
	mux.HandleFunc("GET /api/files/roots", s.guard(auth.Owner, func(w http.ResponseWriter, r *http.Request, _ *auth.Session) {
		writeJSON(w, 200, m.Roots)
	}))
	mux.HandleFunc("GET /api/files/list", s.guard(auth.Owner, func(w http.ResponseWriter, r *http.Request, _ *auth.Session) {
		root, p := q(r)
		ls, err := m.List(root, p)
		if err != nil {
			fail(w, 400, err.Error())
			return
		}
		writeJSON(w, 200, ls)
	}))
	mux.HandleFunc("GET /api/files/text", s.guard(auth.Owner, func(w http.ResponseWriter, r *http.Request, _ *auth.Session) {
		root, p := q(r)
		text, mod, err := m.ReadText(root, p)
		if err != nil {
			fail(w, 400, err.Error())
			return
		}
		writeJSON(w, 200, map[string]any{"text": text, "modified": mod})
	}))
	mux.HandleFunc("POST /api/files/text", s.guard(auth.Owner, func(w http.ResponseWriter, r *http.Request, ses *auth.Session) {
		var in struct {
			Root, Path, Text string
			Modified         int64
		}
		if readJSON(r, &in) != nil {
			fail(w, 400, "bad request")
			return
		}
		err := m.WriteText(in.Root, in.Path, in.Text, in.Modified)
		s.Store.Audit(ses.User.Name, "edit file", in.Root+"/"+in.Path, "", err == nil)
		if err != nil {
			fail(w, 409, err.Error())
			return
		}
		writeJSON(w, 200, map[string]bool{"ok": true})
	}))
	mux.HandleFunc("GET /api/files/download", s.guard(auth.Owner, func(w http.ResponseWriter, r *http.Request, ses *auth.Session) {
		root, p := q(r)
		f, fi, err := m.Open(root, p)
		if err != nil {
			fail(w, 400, err.Error())
			return
		}
		defer f.Close()
		s.Store.Audit(ses.User.Name, "download file", root+"/"+p, "", true)
		name := path.Base(strings.ReplaceAll(p, `\`, "/"))
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": name}))
		w.Header().Set("X-Content-Type-Options", "nosniff")
		http.ServeContent(w, r, name, fi.ModTime(), f)
	}))
	mux.HandleFunc("POST /api/files/upload", s.guard(auth.Owner, func(w http.ResponseWriter, r *http.Request, ses *auth.Session) {
		root, p := q(r)
		r.Body = http.MaxBytesReader(w, r.Body, files.MaxUpload+1)
		err := m.Upload(root, p, r.Body)
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			err = errors.New("uploads are limited to 64 MB")
		}
		s.Store.Audit(ses.User.Name, "upload file", root+"/"+p, "", err == nil)
		if err != nil {
			fail(w, 400, err.Error())
			return
		}
		writeJSON(w, 200, map[string]bool{"ok": true})
	}))
	mux.HandleFunc("POST /api/files/trash", s.guard(auth.Owner, func(w http.ResponseWriter, r *http.Request, ses *auth.Session) {
		var in struct{ Root, Path string }
		if readJSON(r, &in) != nil {
			fail(w, 400, "bad request")
			return
		}
		_, err := m.Trash(in.Root, in.Path)
		s.Store.Audit(ses.User.Name, "delete file", in.Root+"/"+in.Path, "moved to Warden's bin", err == nil)
		if err != nil {
			fail(w, 409, err.Error())
			return
		}
		writeJSON(w, 200, map[string]bool{"ok": true})
	}))
	mux.HandleFunc("GET /api/files/versions", s.guard(auth.Owner, func(w http.ResponseWriter, r *http.Request, _ *auth.Session) {
		root, p := q(r)
		vs, err := m.Versions(root, p)
		if err != nil {
			fail(w, 400, err.Error())
			return
		}
		writeJSON(w, 200, vs)
	}))
	mux.HandleFunc("GET /api/files/version", s.guard(auth.Owner, func(w http.ResponseWriter, r *http.Request, _ *auth.Session) {
		root, p := q(r)
		text, err := m.ReadVersion(root, p, r.URL.Query().Get("v"))
		if err != nil {
			fail(w, 400, err.Error())
			return
		}
		writeJSON(w, 200, map[string]string{"text": text})
	}))
}
