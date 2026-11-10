package cli

import "github.com/spf13/cobra"

func newPageCmd(d Deps) *cobra.Command {
	c := &cobra.Command{
		Use:   "page",
		Short: "ページを取得・一覧する",
		Long:  "ページの取得・一覧・階層・版を扱う。<page> にはページ ID、ページの URL、短縮 URL（/wiki/x/...）を指定できる。",
	}
	c.AddCommand(newPageGetCmd(d), newPageListCmd(d), newPageChildrenCmd(d), newPageAncestorsCmd(d), newPageVersionsCmd(d))
	return c
}
