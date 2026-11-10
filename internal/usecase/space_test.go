package usecase

import (
	"context"
	"testing"

	"github.com/genshibangou16/confluence-cli/internal/domain"
)

type fakeSpaceFinder struct {
	byKey map[string]domain.Space
}

func (f *fakeSpaceFinder) SpaceByKey(_ context.Context, key string) (domain.Space, error) {
	if s, ok := f.byKey[key]; ok {
		return s, nil
	}
	return domain.Space{}, &domain.Error{Kind: domain.ErrNotFound}
}

func (f *fakeSpaceFinder) SpaceByID(_ context.Context, id string) (domain.Space, error) {
	for _, s := range f.byKey {
		if s.ID == id {
			return s, nil
		}
	}
	return domain.Space{}, &domain.Error{Kind: domain.ErrNotFound}
}

func TestResolveID(t *testing.T) {
	svc := NewSpaceService(&fakeSpaceFinder{byKey: map[string]domain.Space{"DEV": {ID: "100", Key: "DEV"}}})
	ctx := context.Background()

	tests := []struct {
		name     string
		key, id  string
		want     string
		wantKind domain.ErrorKind
	}{
		{name: "key を ID に変換する", key: "DEV", want: "100"},
		{name: "両方空なら空", want: ""},
		{name: "id だけならそのまま", id: "200", want: "200"},
		{name: "key と id が一致", key: "DEV", id: "100", want: "100"},
		{name: "key と id が食い違う", key: "DEV", id: "999", wantKind: domain.ErrValidation},
		{name: "key が見つからない", key: "NONE", wantKind: domain.ErrNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := svc.ResolveID(ctx, tt.key, tt.id)
			if tt.wantKind != "" {
				if domain.KindOf(err) != tt.wantKind {
					t.Fatalf("err = %v", err)
				}
				return
			}
			if err != nil || got != tt.want {
				t.Fatalf("got %q, %v; want %q", got, err, tt.want)
			}
		})
	}
}
