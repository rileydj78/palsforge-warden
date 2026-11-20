package backup

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSFTPGivesUpOnAServerThatNeverAnswers(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			defer c.Close() // accept, then say nothing
		}
	}()
	key := filepath.Join(t.TempDir(), "k")
	os.WriteFile(key, []byte(testKey), 0o600)
	tg := SFTPTarget{URL: "sftp://u@" + ln.Addr().String() + "/b", KeyFile: key, HostKey: "SHA256:x"}
	handshakeTimeout = time.Second
	defer func() { handshakeTimeout = 30 * time.Second }()
	start := time.Now()
	_, _, _, err = tg.dial(context.Background())
	if err == nil || time.Since(start) > 10*time.Second {
		t.Fatalf("must give up within the deadline: %v after %v", err, time.Since(start))
	}
}

const testKey = `-----BEGIN OPENSSH PRIVATE KEY-----
b3BlbnNzaC1rZXktdjEAAAAABG5vbmUAAAAEbm9uZQAAAAAAAAABAAAAMwAAAAtzc2gtZW
QyNTUxOQAAACDKKpk9t5sO3q2Rt1dmgbNWHQi8KUtJfvZFPXexbpP0kAAAAJjQgEaV0IBG
lQAAAAtzc2gtZWQyNTUxOQAAACDKKpk9t5sO3q2Rt1dmgbNWHQi8KUtJfvZFPXexbpP0kA
AAAEAZ3c2ZPsNJyx7yTsoVJLAl0XDnNVc0ZJuvzO6vjRq0K8oqmT23mw7erZG3V2aBs1Yd
CLwpS0l+9kU9d7Fuk/SQAAAAEHBhbHNmb3JnZS13YXJkZW4BAgMEBQ==
-----END OPENSSH PRIVATE KEY-----
`
