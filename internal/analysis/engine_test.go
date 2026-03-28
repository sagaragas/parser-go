package analysis

import (
	"context"
	"testing"
)

func TestNewEngine(t *testing.T) {
	eng := NewEngine()
	if eng == nil {
		t.Fatal("expected non-nil engine")
	}
}

func TestEngine_Analyze_EmptyInput(t *testing.T) {
	eng := NewEngine()
	ctx := context.Background()

	_, err := eng.Analyze(ctx, []byte{})
	if err == nil {
		t.Error("expected error for empty input, got nil")
	}
}

func TestEngine_Analyze_Placeholder(t *testing.T) {
	eng := NewEngine()
	ctx := context.Background()

	result, err := eng.Analyze(ctx, []byte("test log line\n"))
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}
