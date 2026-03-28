package server

import (
	"testing"
)

func TestNew(t *testing.T) {
	s := New()
	if s == nil {
		t.Fatal("expected non-nil server")
	}
	if s.mux == nil {
		t.Error("expected non-nil mux")
	}
}

func TestServer_Handler(t *testing.T) {
	s := New()
	h := s.Handler()
	if h == nil {
		t.Error("expected non-nil handler")
	}
}
