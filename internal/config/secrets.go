package config

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
	m.Backups.S3 = append(m.Backups.S3[:0:0], w.Backups.S3...)
	for i := range m.Backups.S3 {
		mask(&m.Backups.S3[i].SecretKey)
	}
	return m
}

// KeepSecrets puts saved secrets back wherever the browser sent the placeholder. An S3 place
// keeps its key only if it's the same place (endpoint, bucket and access key).
func (w *Warden) KeepSecrets(old Warden) {
	keep(&w.PalDef.Token, old.PalDef.Token)
	keep(&w.Notify.DiscordWebhook, old.Notify.DiscordWebhook)
	keep(&w.Notify.NtfyToken, old.Notify.NtfyToken)
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
}
