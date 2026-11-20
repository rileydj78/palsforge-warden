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

func TestAccountsAndRemoval(t *testing.T) {
	a := newAuth(t)
	if err := a.AddUser("moss", "short", Admin); err == nil {
		t.Fatal("a short password must be refused")
	}
	if err := a.AddUser("moss", "correct horse battery", Owner); err != nil {
		t.Fatal(err)
	}
	tok, _, err := a.Login("moss", "correct horse battery")
	if err != nil {
		t.Fatal(err)
	}
	if err := a.RemoveUser("moss"); err != nil || a.Session(tok) != nil {
		t.Fatal("removing an account must end its sessions")
	}
	if a.RemoveUser("nobody") == nil {
		t.Fatal("removing a missing account is an error")
	}
}

func TestLoginLockout(t *testing.T) {
	a := newAuth(t)
	a.AddUser("moss", "correct horse battery", Owner)
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
