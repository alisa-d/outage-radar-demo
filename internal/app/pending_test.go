package app_test

import "testing"

func TestRepeatSignupForAPendingEmailKeepsThePendingDataAndResendsVerification(t *testing.T) {
	s, st := started(t)
	signup(t, s, "victim@example.com", "en", addr{st, "5"})
	first := waitMail(t, s, 1)[0]
	// Anyone can submit the victim's email again; it must not reset what is pending.
	if c, _ := signup(t, s, "victim@example.com", "en", addr{st, "9"}); c != 202 {
		t.Fatalf("repeat signup: %d", c)
	}
	waitMail(t, s, 2)
	if n := count(t, s, "SELECT count(*) FROM subscribers"); n != 1 {
		t.Errorf("subscribers = %d", n)
	}
	if c := verify(t, s, token(t, first)); c != 204 {
		t.Errorf("the first verification link was destroyed: %d", c)
	}
}
