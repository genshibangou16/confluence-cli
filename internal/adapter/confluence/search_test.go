package confluence

import (
	"context"
	"testing"
	"time"
)

const sampleSearch = `{"results":[
{"content":{"id":"123","type":"page","status":"current","title":"T","_expandable":{"space":"/rest/api/space/DEV"}},
 "title":"T","excerpt":"a @@@hl@@@x@@@endhl@@@","url":"/spaces/DEV/pages/123/T",
 "resultGlobalContainer":{"title":"Dev space","displayUrl":"/spaces/OTHER"},"lastModified":"2026-06-10T01:03:37.000Z"},
{"content":null,"title":"user","url":"/people/1"},
{"content":{"id":"456","type":"blogpost","title":"B"},"title":"B","excerpt":"","url":"/spaces/OPS/blog/456",
 "resultGlobalContainer":{"displayUrl":"/spaces/OPS"},"lastModified":"broken"}
],"_links":{"base":"https://example.atlassian.net/wiki"}}`

func TestSearch_MapsFields(t *testing.T) {
	var rec []recorded
	c := recordingClient(t, 200, sampleSearch, &rec)
	got, err := c.Search(context.Background(), "type=page", 10, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("len=%d %+v", len(got), got)
	}
	h := got[0]
	if h.ID != "123" || h.Title != "T" || h.Type != "page" || h.SpaceKey != "DEV" ||
		h.Excerpt != "a @@@hl@@@x@@@endhl@@@" ||
		h.URL != "https://example.atlassian.net/wiki/spaces/DEV/pages/123/T" ||
		!h.LastModified.Equal(time.Date(2026, 6, 10, 1, 3, 37, 0, time.UTC)) {
		t.Errorf("h=%+v", h)
	}
	if got[1].SpaceKey != "OPS" || got[1].Type != "blogpost" || !got[1].LastModified.IsZero() {
		t.Errorf("got[1]=%+v", got[1])
	}
	r := rec[0]
	if r.path != "/wiki/rest/api/search" || r.query["excerpt"][0] != "highlight_unescaped" || r.query["limit"][0] != "10" {
		t.Errorf("%s %v", r.path, r.query)
	}
}

func TestSearch_EncodesCQL(t *testing.T) {
	var rec []recorded
	c := recordingClient(t, 200, `{"results":[]}`, &rec)
	cql := `title ~ "日本語 \"引用\"" AND text ~ "a&b=c"`
	if _, err := c.Search(context.Background(), cql, 0, false); err != nil {
		t.Fatal(err)
	}
	if rec[0].query["cql"][0] != cql {
		t.Errorf("cql=%q", rec[0].query["cql"][0])
	}
}

func TestSearch_StripsHighlightFromTitle(t *testing.T) {
	var rec []recorded
	c := recordingClient(t, 200, `{"results":[{"content":{"id":"1","type":"page"},"title":"a@@@hl@@@b@@@endhl@@@c","excerpt":""}]}`, &rec)
	got, err := c.Search(context.Background(), "title ~ b", 10, false)
	if err != nil {
		t.Fatal(err)
	}
	if got[0].Title != "abc" {
		t.Fatalf("title = %q", got[0].Title)
	}
}
