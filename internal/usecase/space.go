package usecase

import (
	"context"
	"fmt"

	"github.com/genshibangou16/confluence-cli/internal/domain"
)

type SpaceService struct {
	f SpaceKeyFinder
}

func NewSpaceService(f SpaceKeyFinder) *SpaceService {
	return &SpaceService{f: f}
}

// ResolveID は --space-key と --space-id から space-id を決める。
// 両方空なら "" を返し、呼び出し側はスペースで絞り込まない。
func (s *SpaceService) ResolveID(ctx context.Context, key, id string) (string, error) {
	if key == "" {
		return id, nil
	}
	sp, err := s.f.SpaceByKey(ctx, key)
	if err != nil {
		return "", err
	}
	if id != "" && id != sp.ID {
		return "", &domain.Error{
			Kind:    domain.ErrValidation,
			Message: fmt.Sprintf("--space-key %q の space-id は %s で、--space-id %s と一致しない", key, sp.ID, id),
		}
	}
	return sp.ID, nil
}
