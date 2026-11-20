package config

import (
	"net/url"
	"strings"
)

// Secrets never go to the browser: the settings page gets a placeholder for each one that is set,
// and a placeholder sent back means "keep what's saved".

// Unchanged is the placeholder shown for a secret that is set.
const Unchanged = "•••• (saved; type to replace)"

func mask(s *string) {
	if *s != "" {
		*s = Unchanged
	}
}

func keep(s *string, old string) {
	if *s == Unchanged {
		*s = old
	}
}

// Masked returns a copy safe to send to a browser.
func (w Warden) Masked() Warden {
	m := w
	mask(&m.PalDef.Token)
	mask(&m.Notify.DiscordWebhook)
	mask(&m.Notify.NtfyToken)
	mask(&m.DiscordBot.Token)
	m.Backups.S3 = append(m.Backups.S3[:0:0], w.Backups.S3...)
	for i := range m.Backups.S3 {
		mask(&m.Backups.S3[i].SecretKey)
	}
	m.Backups.WebDAV = append(m.Backups.WebDAV[:0:0], w.Backups.WebDAV...)
	for i := range m.Backups.WebDAV {
		mask(&m.Backups.WebDAV[i].Password)
	}
	return m
}

// KeepSecrets puts saved secrets back wherever the browser sent the placeholder. An S3 place
// keeps its key only if it's the same place (endpoint, bucket and access key).
func (w *Warden) KeepSecrets(old Warden) {
	if sameHost(w.PalDef.URL, old.PalDef.URL) { // a token only ever goes back where it came from
		keep(&w.PalDef.Token, old.PalDef.Token)
	} else if w.PalDef.Token == Unchanged {
		w.PalDef.Token = ""
	}
	keep(&w.Notify.DiscordWebhook, old.Notify.DiscordWebhook) // the webhook is its own address
	keep(&w.DiscordBot.Token, old.DiscordBot.Token)           // the bot token only ever goes to Discord
	if sameHost(w.Notify.NtfyURL, old.Notify.NtfyURL) {
		keep(&w.Notify.NtfyToken, old.Notify.NtfyToken)
	} else if w.Notify.NtfyToken == Unchanged {
		w.Notify.NtfyToken = ""
	}
	for i := range w.Backups.S3 {
		t := &w.Backups.S3[i]
		if t.SecretKey != Unchanged {
			continue
		}
		t.SecretKey = ""
		for _, o := range old.Backups.S3 {
			if o.Endpoint == t.Endpoint && o.Bucket == t.Bucket && o.AccessKey == t.AccessKey {
				t.SecretKey = o.SecretKey
			}
		}
	}
	for i := range w.Backups.WebDAV { // a saved password only ever goes back to the same address and user
		t := &w.Backups.WebDAV[i]
		if t.Password != Unchanged {
			continue
		}
		t.Password = ""
		for _, o := range old.Backups.WebDAV {
			if o.URL == t.URL && o.User == t.User {
				t.Password = o.Password
			}
		}
	}
}

// sameHost: same scheme and host (an empty address means the default, which can't change).
func sameHost(a, b string) bool {
	ua, ea := url.Parse(a)
	ub, eb := url.Parse(b)
	return ea == nil && eb == nil && ua.Scheme == ub.Scheme && strings.EqualFold(ua.Host, ub.Host)
}
