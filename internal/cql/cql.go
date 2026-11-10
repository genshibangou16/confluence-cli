// Package cql は Confluence の検索条件から CQL 文字列を組み立てる。
package cql

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/genshibangou16/confluence-cli/internal/domain"
)

// Mode は検索語を当てるフィールドを表す。
type Mode int

const (
	ModeSite Mode = iota
	ModeTitle
	ModeText
)

// Sort は検索結果の並び順を表す。SortRelevance は ORDER BY を付けない。
type Sort int

const (
	SortRelevance Sort = iota
	SortModified
)

// Filter は検索条件。RawCQL が空でなければ検索語（Mode と Query）より優先する。
type Filter struct {
	Mode     Mode
	Query    string
	RawCQL   string
	SpaceKey string
	Labels   []string
	Type     string
	Since    string
	Author   string
	Sort     Sort
}

var (
	relativeSince = regexp.MustCompile(`^\d+[hdwMy]$`)
	dateSince     = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
	orderByRe     = regexp.MustCompile(`(?i)\border\s+by\b`)
)

// Quote は CQL の文字列リテラルとして、`\` と `"` をエスケープして `"` で囲む。
func Quote(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return `"` + s + `"`
}

func validation(format string, args ...any) error {
	return &domain.Error{Kind: domain.ErrValidation, Message: fmt.Sprintf(format, args...)}
}

// Build は Filter から CQL を組み立てる。条件は検索語、space、label、type、since、author の順に AND で連結する。
func Build(f Filter) (string, error) {
	var conds []string
	orderBy := ""

	if raw := strings.TrimSpace(f.RawCQL); raw != "" {
		// ORDER BY の後ろに AND を連結すると不正な CQL になるため、条件部と並び順部に分ける。
		if loc := orderByRe.FindStringIndex(raw); loc != nil {
			if f.Sort != SortRelevance {
				return "", validation("RawCQL に ORDER BY が含まれているため、並び順を同時に指定できない")
			}
			orderBy = " " + strings.TrimSpace(raw[loc[0]:])
			raw = strings.TrimSpace(raw[:loc[0]])
		}
		if raw != "" {
			conds = append(conds, raw)
		}
	} else if f.Query != "" {
		field, err := modeField(f.Mode)
		if err != nil {
			return "", err
		}
		conds = append(conds, field+" ~ "+Quote(f.Query))
	}
	hasTerm := len(conds) > 0
	// RawCQL は他の条件と連結するときだけ括弧で囲む。
	rawOnly := len(conds) == 1 && strings.TrimSpace(f.RawCQL) != ""

	var filters []string
	if f.SpaceKey != "" {
		filters = append(filters, "space = "+Quote(f.SpaceKey))
	}
	for _, l := range f.Labels {
		filters = append(filters, "label = "+Quote(l))
	}
	if f.Type != "" {
		filters = append(filters, "type = "+Quote(f.Type))
	}
	if f.Since != "" {
		switch {
		case relativeSince.MatchString(f.Since):
			filters = append(filters, fmt.Sprintf(`lastmodified >= now("-%s")`, f.Since))
		case dateSince.MatchString(f.Since):
			filters = append(filters, "lastmodified >= "+Quote(f.Since))
		default:
			return "", validation("--since の形式が不正: %q（例: 30d、2026-01-01）", f.Since)
		}
	}
	if f.Author != "" {
		if f.Author == "me" {
			filters = append(filters, "creator = currentUser()")
		} else {
			filters = append(filters, "creator = "+Quote(f.Author))
		}
	}

	if !hasTerm && len(filters) == 0 {
		return "", validation("検索語、--cql、または絞り込み条件を指定する必要がある")
	}
	if rawOnly && len(filters) > 0 {
		conds[0] = "(" + conds[0] + ")"
	}
	conds = append(conds, filters...)

	if f.Sort == SortModified {
		orderBy = " ORDER BY lastmodified DESC"
	}
	return strings.Join(conds, " AND ") + orderBy, nil
}

func modeField(m Mode) (string, error) {
	switch m {
	case ModeSite:
		return "siteSearch", nil
	case ModeTitle:
		return "title", nil
	case ModeText:
		return "text", nil
	}
	return "", validation("不明な検索モード: %d", int(m))
}
