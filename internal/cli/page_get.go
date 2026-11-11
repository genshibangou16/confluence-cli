package cli

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/genshibangou16/confluence-cli/internal/cli/output"
	"github.com/genshibangou16/confluence-cli/internal/domain"
	"github.com/genshibangou16/confluence-cli/internal/usecase"
)

type versionJSON struct {
	Number    int    `json:"number"`
	Message   string `json:"message,omitempty"`
	CreatedAt string `json:"createdAt,omitempty"`
	AuthorID  string `json:"authorId,omitempty"`
	MinorEdit bool   `json:"minorEdit"`
}

func toVersionJSON(v domain.Version) versionJSON {
	j := versionJSON{Number: v.Number, Message: v.Message, AuthorID: v.AuthorID, MinorEdit: v.MinorEdit}
	if !v.CreatedAt.IsZero() {
		j.CreatedAt = v.CreatedAt.Format(time.RFC3339)
	}
	return j
}

type outlineJSON struct {
	Level int    `json:"level"`
	Text  string `json:"text"`
}

type pageGetJSON struct {
	ID      string         `json:"id"`
	Title   string         `json:"title"`
	Status  string         `json:"status"`
	SpaceID string         `json:"spaceId"`
	Version int            `json:"version"`
	URL     string         `json:"url"`
	Content *string        `json:"content,omitempty"`
	Outline *[]outlineJSON `json:"outline,omitempty"`
}

func newPageGetCmd(d Deps) *cobra.Command {
	var (
		format        string
		contentFormat string
		version       int
		outline       bool
		section       string
	)
	c := &cobra.Command{
		Use:   "get <page>",
		Short: "ページを取得する",
		Long: `ページを取得する。

--format markdown（既定）は本文の Markdown を標準出力に出し、id・title・version・url の 4 行と区切りの --- を標準エラーに出す。
--outline は本文の代わりに、トップレベルの見出しをレベルに応じてインデントした一覧を標準出力に出す。
--section は指定した見出しの節の中身だけを出す。--outline とは同時に指定できない。
--format storage は本文（storage 形式）だけを標準出力に、版番号を標準エラーに出す。
--format json はページの情報と content を 1 行の JSON で出す。content の形式は --content-format（markdown, storage）で選ぶ。`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := checkFormat(format, string(usecase.FormatMarkdown), string(usecase.FormatStorage), string(usecase.FormatJSON)); err != nil {
				return err
			}
			if version < 0 {
				return validationf("--version には正の数を指定してください")
			}
			if cmd.Flags().Changed("section") && strings.TrimSpace(section) == "" {
				return validationf("--section には見出しを指定してください")
			}
			id, err := domain.ParsePageRef(args[0])
			if err != nil {
				return err
			}
			app, err := buildApp(cmd.Context(), d)
			if err != nil {
				return err
			}
			v, err := app.Pages.Get(cmd.Context(), id, usecase.GetRequest{
				Format: usecase.Format(format), ContentFormat: usecase.Format(contentFormat),
				Version: version, Outline: outline, Section: section,
			})
			if err != nil {
				return err
			}
			p := v.Page
			switch usecase.Format(format) {
			case usecase.FormatJSON:
				out := pageGetJSON{
					ID: string(p.ID), Title: p.Title, Status: p.Status, SpaceID: p.SpaceID,
					Version: p.Version.Number, URL: p.WebURL,
				}
				if outline {
					items := make([]outlineJSON, 0, len(v.Outline))
					for _, o := range v.Outline {
						items = append(items, outlineJSON{Level: o.Level, Text: output.SanitizeText(o.Text)})
					}
					out.Outline = &items
				} else {
					out.Content = &v.Content
				}
				return output.JSON(d.Stdout, out)
			case usecase.FormatStorage:
				fmt.Fprintf(d.Stderr, "version: %d\n", p.Version.Number)
				// 出力を page update --body-file - に渡したときに本文がバイト単位で一致するよう、
				// SanitizeText も末尾の改行の追加も行わない。
				_, err = io.WriteString(d.Stdout, v.Content)
				return err
			}
			// 標準出力を page update --input-format markdown に渡したときにページの情報が本文に入らないよう、ページの情報は標準エラーに出す。
			fmt.Fprintf(d.Stderr, "id: %s\ntitle: %s\nversion: %d\nurl: %s\n---\n",
				p.ID, output.SanitizeText(p.Title), p.Version.Number, output.SanitizeText(p.WebURL))
			if outline {
				for _, o := range v.Outline {
					fmt.Fprintf(d.Stdout, "%s- %s\n", strings.Repeat("  ", o.Level-1), output.SanitizeText(o.Text))
				}
				return nil
			}
			_, err = io.WriteString(d.Stdout, output.SanitizeMultiline(v.Content))
			return err
		},
	}
	c.Flags().StringVar(&format, "format", string(usecase.FormatMarkdown), "出力形式（markdown, storage, json）")
	c.Flags().StringVar(&contentFormat, "content-format", "", "json の content の形式（markdown, storage。既定は markdown）")
	c.Flags().IntVar(&version, "version", 0, "取得する版番号（省略すると最新版）")
	c.Flags().BoolVar(&outline, "outline", false, "本文の代わりにトップレベルの見出しの一覧を出す")
	c.Flags().StringVar(&section, "section", "", "指定した見出しの節の中身だけを出す")
	return c
}
