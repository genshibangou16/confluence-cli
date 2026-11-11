package domain

import (
	"errors"
	"fmt"
	"testing"
)

func TestKindOf(t *testing.T) {
	if got := KindOf(fmt.Errorf("wrap: %w", &Error{Kind: ErrNotFound})); got != ErrNotFound {
		t.Errorf("wrapped *Error: got %q", got)
	}
	if got := KindOf(errors.New("x")); got != ErrAPI {
		t.Errorf("plain error: got %q", got)
	}
}

func TestErrorMessageAndUnwrap(t *testing.T) {
	cause := errors.New("cause")
	e := &Error{Kind: ErrAPI, Message: "msg", Err: cause}
	if e.Error() != "msg" {
		t.Errorf("Error() = %q", e.Error())
	}
	if !errors.Is(e, cause) {
		t.Error("Unwrap does not expose cause")
	}
	if (&Error{Kind: ErrAuth}).Error() != "auth" {
		t.Error("empty Message should fall back to Kind")
	}
}
