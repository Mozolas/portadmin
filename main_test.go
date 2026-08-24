package main

import "testing"

func TestNormaliseVersion(t *testing.T) {
	tests := map[string]string{
		"v0.1.0":                             "v0.1.0",
		"v1.2.3":                             "v1.2.3",
		"v0.0.0-20260824203937-cfb63c903f56": "dev",
		"(devel)":                            "dev",
		"":                                   "dev",
	}

	for in, want := range tests {
		if got := normaliseVersion(in); got != want {
			t.Fatalf("normaliseVersion(%q) = %q, want %q", in, got, want)
		}
	}
}
