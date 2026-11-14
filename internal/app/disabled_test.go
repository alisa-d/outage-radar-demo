package app_test

import "testing"

func TestNoManagementLinkIsSentToADisabledChannel(t *testing.T) {
	s, _ := bounceWatcher(t)
	if c := brevoEvent(t, s, "Bearer "+brevoToken, "hard_bounce", "ivan@example.com"); c != 200 {
		t.Fatalf("webhook %d", c)
	}
	if c, _ := requestLink(t, s, "ivan@example.com"); c != 202 {
		t.Fatalf("request: %d", c)
	}
	waitMail(t, s, 1) // only the verification mail from setup
}
