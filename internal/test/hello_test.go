package test

import "testing"

func TestHello(t *testing.T) {
	got := Hello()
	want := "Hello, World!"

	if got != want {
		t.Errorf("Hello() = %q, want %q", got, want)
	}
}
