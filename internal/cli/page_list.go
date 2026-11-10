package cli

import (
	"github.com/spf13/cobra"

	"github.com/genshibangou16/confluence-cli/internal/cli/output"
	"github.com/genshibangou16/confluence-cli/internal/domain"
)

type pageSummaryJSON struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Status  string `json:"status"`
	SpaceID string `json:"space_id"`
}

// writeSummaries はページの概要の一覧を、format（table か json）に従って書き出す。
func writeSummaries(d Deps, format string, ps []domain.PageSummary) error {
	if format == "json" {
		out := make([]pageSummaryJSON, 0, len(ps))
		for _, p := range ps {
			out = append(out, pageSummaryJSON{ID: string(p.ID), Title: p.Title, Status: p.Status, SpaceID: p.SpaceID})
		}
		return output.JSON(d.Stdout, out)
	}
	rows := make([][]string, 0, len(ps))
	for _, p := range ps {
		rows = append(rows, []string{string(p.ID), p.Title, p.Status, p.SpaceID})
	}
	return output.Table(d.Stdout, []string{"ID", "TITLE", "STATUS", "SPACE ID"}, rows)
}

func addSpaceFlags(c *cobra.Command, key, id *string) {
	c.Flags().StringVar(key, "space-key", "", "スペースキーで絞り込む")
	c.Flags().StringVar(id, "space-id", "", "space-id で絞り込む")
}

func newPageListCmd(d Deps) *cobra.Command {
	var (
		lf                listFlags
		spaceKey, spaceID string
	)
	c := &cobra.Command{
		Use:   "list",
		Short: "ページを一覧する（本文は取得しない）",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			limit, all, err := lf.args()
			if err != nil {
				return err
			}
			app, err := buildApp(cmd.Context(), d)
			if err != nil {
				return err
			}
			sid, err := app.Spaces.ResolveID(cmd.Context(), spaceKey, spaceID)
			if err != nil {
				return err
			}
			ps, err := app.Tree.List(cmd.Context(), sid, limit, all)
			if err != nil {
				return err
			}
			return writeSummaries(d, lf.format, ps)
		},
	}
	addSpaceFlags(c, &spaceKey, &spaceID)
	addListFlags(c, &lf, 25)
	return c
}
