package config

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/rileydj78/palsforge-warden/internal/backup"
)

func TestSecretsNeverLeaveAndSurviveASave(t *testing.T) {
	w := Defaults(t.TempDir())
	w.PalDef.Token = "pd-secret"
	w.Notify.DiscordWebhook = "https://discord.com/api/webhooks/1/abc"
	w.Backups.S3 = []backup.S3Target{{Endpoint: "https://s3.example.com", Region: "us-east-1", Bucket: "b", AccessKey: "AK", SecretKey: "s3-secret"}}
	b, _ := json.Marshal(w.Masked())
	for _, s := range []string{"pd-secret", "webhooks/1/abc", "s3-secret"} {
		if strings.Contains(string(b), s) {
			t.Fatalf("%s reached the browser", s)
		}
	}
	if w.PalDef.Token != "pd-secret" || w.Backups.S3[0].SecretKey != "s3-secret" {
		t.Fatal("masking must not change the saved settings")
	}
	var back Warden
	json.Unmarshal(b, &back)
	back.KeepSecrets(w)
	if back.PalDef.Token != "pd-secret" || back.Notify.DiscordWebhook != w.Notify.DiscordWebhook || back.Backups.S3[0].SecretKey != "s3-secret" {
		t.Fatal("an unchanged placeholder keeps the saved secret")
	}
	w.Notify.NtfyURL, w.Notify.NtfyToken = "https://ntfy.sh/mine", "ntfy-secret"
	moved := w.Masked()
	moved.Notify.NtfyURL = "http://attacker.example/x"
	moved.KeepSecrets(w)
	if moved.Notify.NtfyToken != "" {
		t.Fatal("a saved token must never follow a changed address")
	}
	back2 := w.Masked()
	back2.Backups.S3[0].Endpoint = "https://evil.example.com"
	back2.KeepSecrets(w)
	if back2.Backups.S3[0].SecretKey != "" {
		t.Fatal("a saved key must never be handed to a different endpoint")
	}
}
