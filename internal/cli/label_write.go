package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/genshibangou16/confluence-cli/internal/cli/output"
	"github.com/genshibangou16/confluence-cli/internal/domain"
)

type labelsAddedJSON struct {
	ID     string   `json:"id"`
	Labels []string `json:"labels"`
}

type labelRemovedJSON struct {
	ID      string `json:"id"`
	Removed string `json:"removed"`
}

func newLabelAddCmd(d Deps) *cobra.Command {
	var format string
	c := &cobra.Command{
		Use:   "add <page> <label>...",
		Short: "ページにラベルを付ける",
		Args:  cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := checkFormat(format, "text", "json"); err != nil {
				return err
			}
			id, err := domain.ParsePageRef(args[0])
			if err != nil {
				return err
			}
			names := args[1:]
			app, err := buildApp(cmd.Context(), d)
			if err != nil {
				return err
			}
			if err := app.LabelCmd.Add(cmd.Context(), id, names); err != nil {
				return err
			}
			if format == "json" {
				return output.JSON(d.Stdout, labelsAddedJSON{ID: string(id), Labels: names})
			}
			out := &printer{w: d.Stdout}
			out.println("labels:")
			for _, n := range names {
				out.printf("- %s\n", output.SanitizeText(n))
			}
			return out.err
		},
	}
	c.Flags().StringVar(&format, "format", "text", "出力形式（text, json）")
	return c
}

func newLabelRemoveCmd(d Deps) *cobra.Command {
	var format string
	c := &cobra.Command{
		Use:   "remove <page> <label>",
		Short: "ページからラベルを外す",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := checkFormat(format, "text", "json"); err != nil {
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
			if err := app.LabelCmd.Remove(cmd.Context(), id, args[1]); err != nil {
				return err
			}
			if format == "json" {
				return output.JSON(d.Stdout, labelRemovedJSON{ID: string(id), Removed: args[1]})
			}
			_, err = fmt.Fprintf(d.Stdout, "removed: %s\n", output.SanitizeText(args[1]))
			return err
		},
	}
	c.Flags().StringVar(&format, "format", "text", "出力形式（text, json）")
	return c
}
