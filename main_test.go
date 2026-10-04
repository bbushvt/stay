package main

import "testing"

func TestRequireLoopback(t *testing.T) {
	ok := []string{"127.0.0.1:7681", "localhost:1", "[::1]:7681", "127.0.0.2:80"}
	bad := []string{"0.0.0.0:7681", ":7681", "10.0.0.5:7681", "example.com:80", "[::]:7681", "nonsense"}
	for _, a := range ok {
		if err := requireLoopback(a); err != nil {
			t.Errorf("%q should be allowed: %v", a, err)
		}
	}
	for _, a := range bad {
		if err := requireLoopback(a); err == nil {
			t.Errorf("%q should be rejected", a)
		}
	}
}
