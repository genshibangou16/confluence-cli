package confluence

import (
	"context"
	"testing"

	"github.com/genshibangou16/confluence-cli/internal/domain"
)

func TestSpaceByKey(t *testing.T) {
	var rec []recorded
	c := recordingClient(t, 200, `{"results":[{"id":"10","key":"KEY","name":"N"}]}`, &rec)
	s, err := c.SpaceByKey(context.Background(), "KEY")
	if err != nil {
		t.Fatal(err)
	}
	if rec[0].path != "/wiki/api/v2/spaces" || rec[0].query["keys"][0] != "KEY" {
		t.Errorf("%s %v", rec[0].path, rec[0].query)
	}
	if s != (domain.Space{ID: "10", Key: "KEY", Name: "N"}) {
		t.Errorf("s=%+v", s)
	}
}

func TestSpaceByKey_NotFound(t *testing.T) {
	var rec []recorded
	c := recordingClient(t, 200, `{"results":[]}`, &rec)
	_, err := c.SpaceByKey(context.Background(), "NOPE")
	if domain.KindOf(err) != domain.ErrNotFound {
		t.Errorf("err=%v", err)
	}
}

func TestSpaceByID(t *testing.T) {
	var rec []recorded
	c := recordingClient(t, 200, `{"id":"10","key":"KEY","name":"N"}`, &rec)
	s, err := c.SpaceByID(context.Background(), "10")
	if err != nil {
		t.Fatal(err)
	}
	if rec[0].path != "/wiki/api/v2/spaces/10" || s.Key != "KEY" {
		t.Errorf("%s %+v", rec[0].path, s)
	}
}
