package cql

import (
	"errors"
	"testing"

	"github.com/genshibangou16/confluence-cli/internal/domain"
)

func TestBuild(t *testing.T) {
	cases := []struct {
		name string
		f    Filter
		want string
	}{
		{"site", Filter{Mode: ModeSite, Query: "議事録", Type: "page"}, `siteSearch ~ "議事録" AND type = "page"`},
		{"title escape", Filter{Mode: ModeTitle, Query: `a"b\c`, Type: "page"}, `title ~ "a\"b\\c" AND type = "page"`},
		{"text space", Filter{Mode: ModeText, Query: "x", SpaceKey: "DEV", Type: "page"}, `text ~ "x" AND space = "DEV" AND type = "page"`},
		{"labels", Filter{Mode: ModeSite, Query: "x", Labels: []string{"a", "b"}, Type: "page"}, `siteSearch ~ "x" AND label = "a" AND label = "b" AND type = "page"`},
		{"since relative", Filter{Mode: ModeSite, Query: "x", Since: "30d", Type: "page"}, `siteSearch ~ "x" AND type = "page" AND lastmodified >= now("-30d")`},
		{"since date", Filter{Mode: ModeSite, Query: "x", Since: "2026-01-01", Type: "page"}, `siteSearch ~ "x" AND type = "page" AND lastmodified >= "2026-01-01"`},
		{"author me", Filter{Mode: ModeSite, Query: "x", Author: "me", Type: "page"}, `siteSearch ~ "x" AND type = "page" AND creator = currentUser()`},
		{"raw", Filter{RawCQL: "label = foo", SpaceKey: "DEV"}, `(label = foo) AND space = "DEV"`},
		{"raw only", Filter{RawCQL: "label = foo"}, `label = foo`},
		{"raw with order by", Filter{RawCQL: "label = foo ORDER BY created", SpaceKey: "DEV"}, `(label = foo) AND space = "DEV" ORDER BY created`},
		{"raw order by in double-quoted literal", Filter{RawCQL: `title ~ "order by"`, SpaceKey: "DEV"}, `(title ~ "order by") AND space = "DEV"`},
		{"raw order by in single-quoted literal", Filter{RawCQL: `title ~ 'a ORDER BY b' ORDER BY lastmodified DESC`, SpaceKey: "DEV"}, `(title ~ 'a ORDER BY b') AND space = "DEV" ORDER BY lastmodified DESC`},
		{"raw escaped quote in literal", Filter{RawCQL: `title ~ "a\" order by b" ORDER BY created`, SpaceKey: "DEV"}, `(title ~ "a\" order by b") AND space = "DEV" ORDER BY created`},
		{"raw last order by", Filter{RawCQL: "title ~ x ORDER  BY a ORDER\n\tby b", SpaceKey: "DEV"}, "(title ~ x ORDER  BY a) AND space = \"DEV\" ORDER\n\tby b"},
		{"raw orderby not word bounded", Filter{RawCQL: "myorder by = x", SpaceKey: "DEV"}, `(myorder by = x) AND space = "DEV"`},
		{"raw literal order by with sort", Filter{RawCQL: `title ~ "order by"`, Sort: SortModified}, `title ~ "order by" ORDER BY lastmodified DESC`},
		{"sort modified", Filter{Mode: ModeSite, Query: "x", Type: "page", Sort: SortModified}, `siteSearch ~ "x" AND type = "page" ORDER BY lastmodified DESC`},
		{"emoji", Filter{Mode: ModeSite, Query: "😀 'q'", Type: "page"}, `siteSearch ~ "😀 'q'" AND type = "page"`},
		{"mode without query", Filter{Mode: ModeSite, Type: "page"}, `type = "page"`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := Build(c.f)
			if err != nil {
				t.Fatal(err)
			}
			if got != c.want {
				t.Errorf("got %s\nwant %s", got, c.want)
			}
		})
	}
}

func TestBuildRejects(t *testing.T) {
	cases := []struct {
		name string
		f    Filter
	}{
		{"since 30x", Filter{Mode: ModeSite, Query: "x", Since: "30x"}},
		{"since yesterday", Filter{Mode: ModeSite, Query: "x", Since: "yesterday"}},
		{"empty", Filter{}},
		{"mode only", Filter{Mode: ModeSite}},
		{"raw order by and sort", Filter{RawCQL: "label = foo order by created", Sort: SortModified}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := Build(c.f)
			var de *domain.Error
			if !errors.As(err, &de) || de.Kind != domain.ErrValidation {
				t.Fatalf("want validation error, got %v", err)
			}
		})
	}
}
