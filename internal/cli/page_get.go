package cli

import (
	"fmt"
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
	CreatedAt string `json:"created_at,omitempty"`
	AuthorID  string `json:"author_id,omitempty"`
	MinorEdit bool   `json:"minor_edit"`
}

func toVersionJSON(v domain.Version) versionJSON {
	j := versionJSON{Number: v.Number, Message: v.Message, AuthorID: v.AuthorID, MinorEdit: v.MinorEdit}
	if !v.CreatedAt.IsZero() {
		j.CreatedAt = v.CreatedAt.Format(time.RFC3339)
	}
	return j
}

type pageJSON struct {
	ID       string      `json:"id"`
	Title    string      `json:"title"`
	Status   string      `json:"status"`
	SpaceID  string      `json:"space_id"`
	ParentID string      `json:"parent_id,omitempty"`
	URL      string      `json:"url,omitempty"`
	Version  versionJSON `json:"version"`
	Body     string      `json:"body"`
}

func newPageGetCmd(d Deps) *cobra.Command {
	var (
		format  string
		version int
	)
	c := &cobra.Command{
		Use:   "get <page>",
		Short: "ページを取得する",
		Long: `ページを取得する。

--format storage は本文（storage 形式）だけを標準出力に、版番号を標準エラーに出す。
--format json はページの情報と本文を 1 行の JSON で出す。`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := checkFormat(format, string(usecase.FormatStorage), string(usecase.FormatJSON)); err != nil {
				return err
			}
			if version < 0 {
				return validationf("--version には正の数を指定する")
			}
			id, err := domain.ParsePageRef(args[0])
			if err != nil {
				return err
			}
			app, err := buildApp(cmd.Context(), d)
			if err != nil {
				return err
			}
			v, err := app.Pages.Get(cmd.Context(), id, usecase.GetRequest{Format: usecase.Format(format), Version: version})
			if err != nil {
				return err
			}
			p := v.Page
			if format == string(usecase.FormatJSON) {
				return output.JSON(d.Stdout, pageJSON{
					ID: string(p.ID), Title: p.Title, Status: p.Status, SpaceID: p.SpaceID, ParentID: p.ParentID,
					URL: p.WebURL, Version: toVersionJSON(p.Version), Body: v.Content,
				})
			}
			fmt.Fprintf(d.Stderr, "version: %d\n", p.Version.Number)
			// 出力を page update --body-file にそのまま渡せるよう、本文は SanitizeText を通さない。
			content := v.Content
			if !strings.HasSuffix(content, "\n") {
				content += "\n"
			}
			_, err = fmt.Fprint(d.Stdout, content)
			return err
		},
	}
	// 既定の形式は、markdown 変換を実装するまで storage にする。
	c.Flags().StringVar(&format, "format", string(usecase.FormatStorage), "出力形式（storage, json）")
	c.Flags().IntVar(&version, "version", 0, "取得する版番号（省略すると最新版）")
	return c
}
