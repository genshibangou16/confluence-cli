package cli

import (
	"strconv"

	"github.com/spf13/cobra"

	"github.com/genshibangou16/confluence-cli/internal/cli/output"
	"github.com/genshibangou16/confluence-cli/internal/domain"
)

func newPageChildrenCmd(d Deps) *cobra.Command {
	var lf listFlags
	c := &cobra.Command{
		Use:   "children <page>",
		Short: "直下の子コンテンツを一覧する",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			limit, all, err := lf.args()
			if err != nil {
				return err
			}
			id, err := domain.ParsePageRef(args[0])
			if err != nil {
				return err
			}
			app, err := buildApp(cmd.Context(), d)
			if err != nil {
				return err
			}
			ps, err := app.Tree.Children(cmd.Context(), id, limit, all)
			if err != nil {
				return err
			}
			return writeSummaries(d, lf.format, ps)
		},
	}
	addListFlags(c, &lf, 25)
	return c
}

type ancestorJSON struct {
	ID    string `json:"id"`
	Type  string `json:"type"`
	Title string `json:"title,omitempty"`
}

func newPageAncestorsCmd(d Deps) *cobra.Command {
	var format string
	c := &cobra.Command{
		Use:   "ancestors <page>",
		Short: "祖先をルートに近い順に一覧する",
		Long:  "祖先をルートに近い順に一覧する。フォルダーなどページ以外の祖先は TITLE が空になる。",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := checkFormat(format, "table", "json"); err != nil {
				return err
			}
			id, err := domain.ParsePageRef(args[0])
			if err != nil {
				return err
			}
			app, err := buildApp(cmd.Context(), d)
			if err != nil {
				return err
			}
			as, err := app.Tree.Ancestors(cmd.Context(), id)
			if err != nil {
				return err
			}
			if format == "json" {
				out := make([]ancestorJSON, 0, len(as))
				for _, a := range as {
					out = append(out, ancestorJSON{ID: string(a.ID), Type: a.Type, Title: a.Title})
				}
				return output.JSON(d.Stdout, out)
			}
			rows := make([][]string, 0, len(as))
			for _, a := range as {
				rows = append(rows, []string{string(a.ID), a.Type, a.Title})
			}
			return output.Table(d.Stdout, []string{"ID", "TYPE", "TITLE"}, rows)
		},
	}
	c.Flags().StringVar(&format, "format", "table", "出力形式（table, json）")
	return c
}

func newPageVersionsCmd(d Deps) *cobra.Command {
	var lf listFlags
	c := &cobra.Command{
		Use:   "versions <page>",
		Short: "ページの版を一覧する",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			limit, all, err := lf.args()
			if err != nil {
				return err
			}
			id, err := domain.ParsePageRef(args[0])
			if err != nil {
				return err
			}
			app, err := buildApp(cmd.Context(), d)
			if err != nil {
				return err
			}
			vs, err := app.Tree.Versions(cmd.Context(), id, limit, all)
			if err != nil {
				return err
			}
			if lf.format == "json" {
				out := make([]versionJSON, 0, len(vs))
				for _, v := range vs {
					out = append(out, toVersionJSON(v))
				}
				return output.JSON(d.Stdout, out)
			}
			rows := make([][]string, 0, len(vs))
			for _, v := range vs {
				rows = append(rows, []string{strconv.Itoa(v.Number), date(v.CreatedAt), v.AuthorID, strconv.FormatBool(v.MinorEdit), v.Message})
			}
			return output.Table(d.Stdout, []string{"VERSION", "CREATED", "AUTHOR ID", "MINOR", "MESSAGE"}, rows)
		},
	}
	addListFlags(c, &lf, 25)
	return c
}
