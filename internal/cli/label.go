package cli

import (
	"github.com/spf13/cobra"

	"github.com/genshibangou16/confluence-cli/internal/cli/output"
)

type labelJSON struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Prefix string `json:"prefix"`
}

func newLabelCmd(d Deps) *cobra.Command {
	c := &cobra.Command{
		Use:   "label",
		Short: "ラベルと、ラベルの付いたページを一覧する",
	}
	c.AddCommand(newLabelListCmd(d), newLabelPagesCmd(d))
	return c
}

func newLabelListCmd(d Deps) *cobra.Command {
	var (
		lf     listFlags
		prefix string
	)
	c := &cobra.Command{
		Use:   "list",
		Short: "ラベルを一覧する",
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
			ls, err := app.Labels.List(cmd.Context(), prefix, limit, all)
			if err != nil {
				return err
			}
			if lf.format == "json" {
				out := make([]labelJSON, 0, len(ls))
				for _, l := range ls {
					out = append(out, labelJSON{ID: l.ID, Name: l.Name, Prefix: l.Prefix})
				}
				return output.JSON(d.Stdout, out)
			}
			rows := make([][]string, 0, len(ls))
			for _, l := range ls {
				rows = append(rows, []string{l.ID, l.Name, l.Prefix})
			}
			return output.Table(d.Stdout, []string{"ID", "NAME", "PREFIX"}, rows)
		},
	}
	c.Flags().StringVar(&prefix, "prefix", "", "ラベルの prefix で絞り込む（my, team, global, system）")
	addListFlags(c, &lf, 25)
	return c
}

func newLabelPagesCmd(d Deps) *cobra.Command {
	var (
		lf                listFlags
		spaceKey, spaceID string
	)
	c := &cobra.Command{
		Use:   "pages <label-id>",
		Short: "ラベルの付いたページを一覧する",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
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
			ps, err := app.Labels.Pages(cmd.Context(), args[0], sid, limit, all)
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
