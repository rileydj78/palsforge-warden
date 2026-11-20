package nightly

import "testing"

func TestGamePort(t *testing.T) {
	if GamePort([]string{"Pal", "-port=8221", "-queryport=27025"}) != 8221 || GamePort([]string{"Pal"}) != 8211 || GamePort([]string{"-PORT=9000"}) != 9000 {
		t.Fatal("game port")
	}
}
