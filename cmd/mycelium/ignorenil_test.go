package main

import (
	"errors"
	"testing"
)

type closer interface {
	Close() error
}

type fakeCloser struct{}

func (f *fakeCloser) Close() error { return nil }

func TestIgnoreNil_NilInterface(t *testing.T) {
	var c closer // nil interface
	called := false
	err := ignoreNil(c, func(v closer) error {
		called = true
		return v.Close()
	})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if called {
		t.Error("fn should not be called for nil interface")
	}
}

func TestIgnoreNil_NilPointer(t *testing.T) {
	var p *fakeCloser // nil pointer
	called := false
	err := ignoreNil(p, func(v *fakeCloser) error {
		called = true
		return nil
	})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if called {
		t.Error("fn should not be called for nil pointer")
	}
}

func TestIgnoreNil_NonNilValue(t *testing.T) {
	p := &fakeCloser{}
	called := false
	err := ignoreNil(p, func(v *fakeCloser) error {
		called = true
		return nil
	})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if !called {
		t.Error("fn should be called for non-nil value")
	}
}

func TestIgnoreNil_NonNilReturnsError(t *testing.T) {
	p := &fakeCloser{}
	want := errors.New("fail")
	err := ignoreNil(p, func(v *fakeCloser) error {
		return want
	})
	if !errors.Is(err, want) {
		t.Errorf("got %v, want %v", err, want)
	}
}

func TestIgnoreNil_ValueType(t *testing.T) {
	// Value types (int, string) should always call fn
	called := false
	err := ignoreNil(42, func(v int) error {
		called = true
		return nil
	})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if !called {
		t.Error("fn should be called for value type")
	}
}
