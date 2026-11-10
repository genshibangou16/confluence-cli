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

type recorded struct {
	method string
	path   string
	query  map[string][]string
	body   map[string]any
}

func recordingClient(t *testing.T, status int, resp string, rec *[]recorded) *Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var b map[string]any
		_ = json.Unmarshal(raw, &b)
		*rec = append(*rec, recorded{r.Method, r.URL.EscapedPath(), r.URL.Query(), b})
		w.WriteHeader(status)
		_, _ = w.Write([]byte(resp))
	}))
	t.Cleanup(srv.Close)
	return New(Options{Root: srv.URL, Email: "a@example.com", Token: "t"})
}

const samplePage = `{"id":"123","status":"current","title":"T","spaceId":"10","parentId":"99",
"version":{"number":3,"message":"m","createdAt":"2024-01-02T03:04:05.000Z","authorId":"u1","minorEdit":true},
"body":{"storage":{"value":"<p>x</p>","representation":"storage"}},
"_links":{"webui":"/spaces/K/pages/123/T","base":"https://example.atlassian.net/wiki"}}`

func TestGetPage_QueryAndMapping(t *testing.T) {
	var rec []recorded
	c := recordingClient(t, 200, samplePage, &rec)
	p, err := c.GetPage(context.Background(), "123", true, 3)
	if err != nil {
		t.Fatal(err)
	}
	r := rec[0]
	if r.method != "GET" || r.path != "/wiki/api/v2/pages/123" {
		t.Errorf("%s %s", r.method, r.path)
	}
	if r.query["body-format"][0] != "storage" || r.query["version"][0] != "3" {
		t.Errorf("query=%v", r.query)
	}
	if p.Body != "<p>x</p>" || p.Version.Number != 3 || p.Version.Message != "m" || !p.Version.MinorEdit ||
		p.Version.AuthorID != "u1" || p.Version.CreatedAt.Year() != 2024 || p.ParentID != "99" ||
		p.SpaceID != "10" || p.WebURL != "https://example.atlassian.net/wiki/spaces/K/pages/123/T" {
		t.Errorf("page=%+v", p)
	}
}

func TestGetPage_NoBodyNoBase(t *testing.T) {
	var rec []recorded
	c := recordingClient(t, 200, `{"id":"1","_links":{"webui":"/x"}}`, &rec)
	p, err := c.GetPage(context.Background(), "a/b", false, 0)
	if err != nil {
		t.Fatal(err)
	}
	if rec[0].path != "/wiki/api/v2/pages/a%2Fb" {
		t.Errorf("path=%s", rec[0].path)
	}
	if _, ok := rec[0].query["body-format"]; ok {
		t.Error("body-format sent")
	}
	if _, ok := rec[0].query["version"]; ok {
		t.Error("version sent")
	}
	if p.WebURL != "/x" || !p.Version.CreatedAt.IsZero() {
		t.Errorf("page=%+v", p)
	}
}

func TestListPages_NoBody(t *testing.T) {
	var rec []recorded
	c := recordingClient(t, 200, `{"results":[{"id":"1","title":"A","status":"current","spaceId":"10"}]}`, &rec)
	got, err := c.ListPages(context.Background(), "10", 5, false)
	if err != nil {
		t.Fatal(err)
	}
	q := rec[0].query
	if _, ok := q["body-format"]; ok {
		t.Error("body-format sent")
	}
	if q["space-id"][0] != "10" || q["limit"][0] != "5" {
		t.Errorf("query=%v", q)
	}
	if !reflect.DeepEqual(got, []domain.PageSummary{{ID: "1", Title: "A", Status: "current", SpaceID: "10"}}) {
		t.Errorf("got=%+v", got)
	}
}

func TestGetPagesByIDs_ChunksAndOrder(t *testing.T) {
	var rec []recorded
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec = append(rec, recorded{query: r.URL.Query()})
		w.Write([]byte(`{"results":[{"id":"b"},{"id":"a"}]}`))
	}))
	defer srv.Close()
	c := New(Options{Root: srv.URL})
	got, err := c.GetPagesByIDs(context.Background(), []domain.PageID{"a", "x", "b"})
	if err != nil {
		t.Fatal(err)
	}
	if rec[0].query["id"][0] != "a,x,b" {
		t.Errorf("id=%v", rec[0].query["id"])
	}
	if len(got) != 2 || got[0].ID != "a" || got[1].ID != "b" {
		t.Errorf("got=%+v", got)
	}

	rec = nil
	ids := make([]domain.PageID, 251)
	for i := range ids {
		ids[i] = domain.PageID("p")
	}
	if _, err := c.GetPagesByIDs(context.Background(), ids); err != nil {
		t.Fatal(err)
	}
	if len(rec) != 2 {
		t.Errorf("requests=%d", len(rec))
	}
	if got, _ := c.GetPagesByIDs(context.Background(), nil); len(got) != 0 || len(rec) != 2 {
		t.Error("empty input must not call the API")
	}
}

func TestUpdatePage_RequestBody(t *testing.T) {
	var rec []recorded
	c := recordingClient(t, 200, samplePage, &rec)
	if _, err := c.UpdatePage(context.Background(), "123", "T", "<p>y</p>", 4, "msg"); err != nil {
		t.Fatal(err)
	}
	want := map[string]any{
		"id": "123", "status": "current", "title": "T",
		"body":    map[string]any{"representation": "storage", "value": "<p>y</p>"},
		"version": map[string]any{"number": float64(4), "message": "msg"},
	}
	if rec[0].method != "PUT" || rec[0].path != "/wiki/api/v2/pages/123" || !reflect.DeepEqual(rec[0].body, want) {
		t.Errorf("%s %s %v", rec[0].method, rec[0].path, rec[0].body)
	}
	rec = nil
	if _, err := c.UpdatePage(context.Background(), "123", "T", "x", 2, ""); err != nil {
		t.Fatal(err)
	}
	if _, ok := rec[0].body["version"].(map[string]any)["message"]; ok {
		t.Error("empty message sent")
	}
}

func TestCreatePage_RequestBody(t *testing.T) {
	var rec []recorded
	c := recordingClient(t, 200, samplePage, &rec)
	if _, err := c.CreatePage(context.Background(), "10", "", "T", "<p/>"); err != nil {
		t.Fatal(err)
	}
	b := rec[0].body
	if rec[0].method != "POST" || rec[0].path != "/wiki/api/v2/pages" {
		t.Errorf("%s %s", rec[0].method, rec[0].path)
	}
	if _, ok := b["parentId"]; ok {
		t.Error("parentId sent")
	}
	if b["spaceId"] != "10" || b["status"] != "current" || b["title"] != "T" ||
		!reflect.DeepEqual(b["body"], map[string]any{"representation": "storage", "value": "<p/>"}) {
		t.Errorf("body=%v", b)
	}
	rec = nil
	if _, err := c.CreatePage(context.Background(), "10", "77", "T", "x"); err != nil {
		t.Fatal(err)
	}
	if rec[0].body["parentId"] != "77" {
		t.Errorf("body=%v", rec[0].body)
	}
}

func TestDeletePage(t *testing.T) {
	var rec []recorded
	c := recordingClient(t, 204, ``, &rec)
	if err := c.DeletePage(context.Background(), "123"); err != nil {
		t.Fatal(err)
	}
	if rec[0].method != "DELETE" || rec[0].path != "/wiki/api/v2/pages/123" {
		t.Errorf("%s %s", rec[0].method, rec[0].path)
	}
}

func TestChildren(t *testing.T) {
	var rec []recorded
	c := recordingClient(t, 200, `{"results":[{"id":"2","title":"C","status":"current","spaceId":"10","type":"folder"}]}`, &rec)
	got, err := c.Children(context.Background(), "1", 0, false)
	if err != nil {
		t.Fatal(err)
	}
	if rec[0].path != "/wiki/api/v2/pages/1/direct-children" || len(got) != 1 || got[0].Title != "C" {
		t.Errorf("%s %+v", rec[0].path, got)
	}
}

func TestAncestors_Order(t *testing.T) {
	var rec []recorded
	c := recordingClient(t, 200, `{"results":[{"id":"1","type":"folder"},{"id":"2","type":"page"}]}`, &rec)
	got, err := c.Ancestors(context.Background(), "3")
	if err != nil {
		t.Fatal(err)
	}
	want := []domain.Ancestor{{ID: "1", Type: "folder"}, {ID: "2", Type: "page"}}
	if rec[0].query["limit"][0] != "250" {
		t.Errorf("limit=%v", rec[0].query["limit"])
	}
	if rec[0].path != "/wiki/api/v2/pages/3/ancestors" || !reflect.DeepEqual(got, want) {
		t.Errorf("%s %+v", rec[0].path, got)
	}
}

func TestVersions(t *testing.T) {
	var rec []recorded
	c := recordingClient(t, 200, `{"results":[{"number":2,"message":"m","minorEdit":true,"authorId":"u","createdAt":"2024-01-02T03:04:05Z"},{"number":1,"createdAt":""}]}`, &rec)
	got, err := c.Versions(context.Background(), "3", 0, true)
	if err != nil {
		t.Fatal(err)
	}
	if rec[0].path != "/wiki/api/v2/pages/3/versions" || len(got) != 2 {
		t.Fatalf("%s %+v", rec[0].path, got)
	}
	if got[0].Number != 2 || got[0].AuthorID != "u" || !got[0].MinorEdit || got[0].CreatedAt.IsZero() || !got[1].CreatedAt.IsZero() {
		t.Errorf("got=%+v", got)
	}
}

func TestGetPage_NotFound(t *testing.T) {
	var rec []recorded
	c := recordingClient(t, 404, `{"message":"nope"}`, &rec)
	_, err := c.GetPage(context.Background(), "1", false, 0)
	if domain.KindOf(err) != domain.ErrNotFound {
		t.Errorf("err=%v", err)
	}
}
