package confluence

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/genshibangou16/confluence-cli/internal/domain"
)

func TestLabels_V2(t *testing.T) {
	var rec []recorded
	c := recordingClient(t, 200, `{"results":[{"id":"1","name":"a","prefix":"global"}]}`, &rec)
	got, err := c.Labels(context.Background(), "my", 5, false)
	if err != nil {
		t.Fatal(err)
	}
	if rec[0].path != "/wiki/api/v2/labels" || rec[0].query["prefix"][0] != "my" || rec[0].query["limit"][0] != "5" ||
		!reflect.DeepEqual(got, []domain.Label{{ID: "1", Name: "a", Prefix: "global"}}) {
		t.Errorf("%s %v %+v", rec[0].path, rec[0].query, got)
	}
}

func TestLabels_NoPrefix(t *testing.T) {
	var rec []recorded
	c := recordingClient(t, 200, `{"results":[]}`, &rec)
	if _, err := c.Labels(context.Background(), "", 0, false); err != nil {
		t.Fatal(err)
	}
	if _, ok := rec[0].query["prefix"]; ok {
		t.Errorf("query=%v", rec[0].query)
	}
}

func TestPageLabels(t *testing.T) {
	var rec []recorded
	c := recordingClient(t, 200, `{"results":[{"id":"1","name":"a","prefix":"global"}]}`, &rec)
	got, err := c.PageLabels(context.Background(), "42", 0, true)
	if err != nil {
		t.Fatal(err)
	}
	if rec[0].path != "/wiki/api/v2/pages/42/labels" || len(got) != 1 {
		t.Errorf("%s %+v", rec[0].path, got)
	}
}

func TestPagesWithLabel(t *testing.T) {
	var rec []recorded
	c := recordingClient(t, 200, `{"results":[{"id":"7","title":"P","status":"current","spaceId":"10"}]}`, &rec)
	got, err := c.PagesWithLabel(context.Background(), "99", "10", 0, false)
	if err != nil {
		t.Fatal(err)
	}
	if rec[0].path != "/wiki/api/v2/labels/99/pages" || rec[0].query["space-id"][0] != "10" ||
		len(got) != 1 || got[0].ID != "7" || got[0].Title != "P" {
		t.Errorf("%s %v %+v", rec[0].path, rec[0].query, got)
	}
	rec = nil
	if _, err := c.PagesWithLabel(context.Background(), "99", "", 0, false); err != nil {
		t.Fatal(err)
	}
	if _, ok := rec[0].query["space-id"]; ok {
		t.Errorf("query=%v", rec[0].query)
	}
}

func TestAddLabels_Body(t *testing.T) {
	var method, path string
	var body []map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method, path = r.Method, r.URL.EscapedPath()
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &body)
		_, _ = w.Write([]byte(`{"results":[]}`))
	}))
	defer srv.Close()
	c := New(Options{Root: srv.URL, Email: "a@example.com", Token: "t"})
	if err := c.AddLabels(context.Background(), "42", []string{"a", "b/c"}); err != nil {
		t.Fatal(err)
	}
	want := []map[string]string{{"prefix": "global", "name": "a"}, {"prefix": "global", "name": "b/c"}}
	if method != "POST" || path != "/wiki/rest/api/content/42/label" || !reflect.DeepEqual(body, want) {
		t.Errorf("%s %s %v", method, path, body)
	}
}

func TestAddLabels_EmptyIsValidationError(t *testing.T) {
	var rec []recorded
	c := recordingClient(t, 200, `{}`, &rec)
	err := c.AddLabels(context.Background(), "42", nil)
	if domain.KindOf(err) != domain.ErrValidation || len(rec) != 0 {
		t.Errorf("err=%v calls=%d", err, len(rec))
	}
}

func TestRemoveLabel_UsesQueryName(t *testing.T) {
	var rec []recorded
	c := recordingClient(t, 204, ``, &rec)
	if err := c.RemoveLabel(context.Background(), "42", "team/a b"); err != nil {
		t.Fatal(err)
	}
	r := rec[0]
	if r.method != "DELETE" || r.path != "/wiki/rest/api/content/42/label" || r.query["name"][0] != "team/a b" {
		t.Errorf("%s %s %v", r.method, r.path, r.query)
	}
}
