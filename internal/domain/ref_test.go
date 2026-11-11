package domain

import (
	"errors"
	"testing"
)

func TestParsePageRef(t *testing.T) {
	cases := map[string]PageID{
		"12345":    "12345",
		" 12345\n": "12345",
		"https://example.atlassian.net/wiki/spaces/DEV/pages/12345/Title":       "12345",
		"https://example.atlassian.net/wiki/spaces/~abc/pages/12345":            "12345",
		"https://example.atlassian.net/wiki/pages/viewpage.action?pageId=12345": "12345",
		"https://example.atlassian.net/wiki/x/0gKWSQ":                           "1234567890",
		"https://example.atlassian.net/wiki/x/sWjeOg":                           "987654321",
		"https://example.atlassian.net/wiki/x/AQAA":                             "1",
		"https://example.atlassian.net/wiki/x/AAAAAAE":                          "4294967296",
		"https://example.atlassian.net/wiki/x/__________8":                      "18446744073709551615",
		"https://example.atlassian.net/wiki/spaces/DEV/pages/edit-v2/12345":     "12345",
		"https://example.atlassian.net/wiki/spaces/DEV/pages/edit/12345":        "12345",
	}
	for in, want := range cases {
		got, err := ParsePageRef(in)
		if err != nil {
			t.Errorf("ParsePageRef(%q) error: %v", in, err)
			continue
		}
		if got != want {
			t.Errorf("ParsePageRef(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestParsePageRefRejects(t *testing.T) {
	for _, in := range []string{"", "  ", "abc", "https://example.atlassian.net/wiki/spaces/DEV", "https://example.atlassian.net/wiki/x/", "https://example.atlassian.net/wiki/x/AAAAAAAAAAAA"} {
		_, err := ParsePageRef(in)
		if err == nil {
			t.Errorf("ParsePageRef(%q) = nil error", in)
			continue
		}
		var de *Error
		if !errors.As(err, &de) || de.Kind != ErrValidation {
			t.Errorf("ParsePageRef(%q) error = %v, want *Error with ErrValidation", in, err)
		}
	}
}
