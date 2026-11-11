package usecase

import (
	"context"
	"fmt"
	"strings"

	"github.com/genshibangou16/confluence-cli/internal/domain"
)

// WritePolicy の WritableSpaces は書き込みを許可するスペースキーの一覧。
// nil なら制限せず、空のスライスならどのスペースにも書き込ませない。
type WritePolicy struct {
	ReadOnly       bool
	WritableSpaces []string
}

// WriteGuard は書き込みの API を呼ぶ前に、WritePolicy で書き込みが許可されているかを確認する。
type WriteGuard struct {
	p      WritePolicy
	spaces SpaceIDFinder
}

func NewWriteGuard(p WritePolicy, spaces SpaceIDFinder) *WriteGuard {
	return &WriteGuard{p: p, spaces: spaces}
}

// Check は spaceID のスペースへの書き込みが許可されていなければ ErrReadOnly を返す。
// 設定はスペースキーで書かれ、ページはスペース ID しか持たないため、WritableSpaces があるときだけ API でキーを引く。
// read_only の判定は API を呼ばずにできるため、checkReadOnly に分けてある。
// 対象のスペースを知るために親ページなどを取得するユースケースは、その取得より前に checkReadOnly を呼ぶ。
func (g *WriteGuard) Check(ctx context.Context, spaceID string) error {
	if err := g.checkReadOnly(); err != nil {
		return err
	}
	if g.p.WritableSpaces == nil {
		return nil
	}
	s, err := g.spaces.SpaceByID(ctx, spaceID)
	if err != nil {
		return err
	}
	for _, k := range g.p.WritableSpaces {
		if strings.EqualFold(k, s.Key) {
			return nil
		}
	}
	return &domain.Error{
		Kind:    domain.ErrReadOnly,
		Message: fmt.Sprintf("スペース %s は writable_spaces に含まれないため、書き込みは許可されていません", s.Key),
	}
}

func (g *WriteGuard) checkReadOnly() error {
	if g.p.ReadOnly {
		return &domain.Error{Kind: domain.ErrReadOnly, Message: "read_only の設定により書き込みは許可されていません"}
	}
	return nil
}
