package validation

import "testing"

func TestEmailOK(t *testing.T) {
	ok := []string{"a@b.co", "user.name+tag@example.com"}
	bad := []string{"", "nope", "a@", "@b.com", "a@b", "a@b.", " plain @x.com "}
	for _, s := range ok {
		if !EmailOK(s) {
			t.Fatalf("expected ok: %q", s)
		}
	}
	for _, s := range bad {
		if EmailOK(s) {
			t.Fatalf("expected bad: %q", s)
		}
	}
}

func TestUsernameOK(t *testing.T) {
	if err := UsernameOK("ab"); err == nil {
		t.Fatal("expected min length error")
	}
	if err := UsernameOK("alice"); err != nil {
		t.Fatal(err)
	}
	if err := UsernameOK("bad name"); err == nil {
		t.Fatal("expected charset error")
	}
}
