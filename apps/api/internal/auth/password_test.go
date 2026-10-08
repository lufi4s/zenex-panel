package auth

import (
	"strings"
	"testing"
)

func TestHashAndVerify(t *testing.T) {
	h, err := HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(h, "$argon2id$v=19$") {
		t.Fatalf("unexpected format: %s", h)
	}
	ok, err := VerifyPassword("correct horse battery staple", h)
	if err != nil || !ok {
		t.Fatalf("correct password rejected: ok=%v err=%v", ok, err)
	}
	ok, err = VerifyPassword("wrong", h)
	if err != nil || ok {
		t.Fatalf("wrong password accepted: ok=%v err=%v", ok, err)
	}
}

func TestHashesAreSalted(t *testing.T) {
	a, _ := HashPassword("same")
	b, _ := HashPassword("same")
	if a == b {
		t.Fatal("two hashes of the same password are identical; salt missing")
	}
}

func TestVerifyRejectsMalformed(t *testing.T) {
	for _, bad := range []string{"", "plain", "$bcrypt$x$y$z$w", "$argon2id$v=19$m=bad$salt$key"} {
		if _, err := VerifyPassword("x", bad); err == nil {
			t.Errorf("malformed hash %q accepted", bad)
		}
	}
}

func TestNewTokenHashMatches(t *testing.T) {
	tok, hash, err := NewToken()
	if err != nil {
		t.Fatal(err)
	}
	if len(tok) < 40 {
		t.Fatalf("token too short: %d", len(tok))
	}
	if string(HashToken(tok)) != string(hash) {
		t.Fatal("HashToken does not match NewToken hash")
	}
}

func TestDummyHashIsVerifiable(t *testing.T) {
	ok, err := VerifyPassword("anything", DummyHash())
	if err != nil || ok {
		t.Fatalf("dummy hash: ok=%v err=%v", ok, err)
	}
}
