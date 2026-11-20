package api

import (
	"testing"
	"time"
)

func TestLimiter(t *testing.T) {
	l := newLimiter(3, time.Minute)
	for i := 0; i < 3; i++ {
		if !l.allowed("u") {
			t.Fatalf("attempt %d should be allowed", i)
		}
		l.fail("u")
	}
	if l.allowed("u") {
		t.Fatal("4th attempt should be blocked")
	}
	if !l.allowed("other") {
		t.Fatal("other keys are unaffected")
	}
	l.reset("u")
	if !l.allowed("u") {
		t.Fatal("reset should unblock")
	}
}
