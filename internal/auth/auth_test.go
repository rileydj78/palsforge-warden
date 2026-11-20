package auth

import (
	"testing"

	"github.com/rileydj78/palsforge-warden/internal/store"
)

func newAuth(t *testing.T) *Auth {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	a, err := New(st.DB)
	if err != nil {
		t.Fatal(err)
	}
	return a
}

func TestOwnerNeedsSetupCode(t *testing.T) {
	a := newAuth(t)
	if a.SetupCode == "" {
		t.Fatal("a fresh install needs a setup code")
	}
	if err := a.CreateOwner("wrong", "moss", "correct horse battery"); err == nil {
		t.Fatal("a wrong setup code must be refused")
	}
	if err := a.CreateOwner(a.SetupCode, "moss", "short"); err == nil {
		t.Fatal("a short password must be refused")
	}
	if err := a.CreateOwner(a.SetupCode, "moss", "correct horse battery"); err != nil {
		t.Fatal(err)
	}
	if a.SetupCode != "" {
		t.Fatal("the setup code must be used up")
	}
	if err := a.CreateOwner("anything", "eve", "correct horse battery"); err == nil {
		t.Fatal("a second owner must be refused")
	}
}

func TestLoginLockout(t *testing.T) {
	a := newAuth(t)
	a.CreateOwner(a.SetupCode, "moss", "correct horse battery")
	tok, s, err := a.Login("moss", "correct horse battery")
	if err != nil || tok == "" || s.User.Role != Owner || s.CSRF == "" {
		t.Fatalf("login: %v", err)
	}
	if a.Session(tok) == nil {
		t.Fatal("session should be live")
	}
	a.Logout(tok)
	if a.Session(tok) != nil {
		t.Fatal("session should be gone")
	}
	for i := 0; i < 5; i++ {
		a.Login("moss", "nope")
	}
	if _, _, err := a.Login("moss", "correct horse battery"); err == nil {
		t.Fatal("sign-in should be locked after five wrong passwords")
	}
}

func TestRoles(t *testing.T) {
	if !Allows(Owner, Admin) || !Allows(Admin, Moderator) || Allows(Moderator, Admin) {
		t.Fatal("role order wrong")
	}
}
