package cli

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/genshibangou16/confluence-cli/internal/cli/output"
	"github.com/genshibangou16/confluence-cli/internal/cql"
	"github.com/genshibangou16/confluence-cli/internal/domain"
)

const excerptRunes = 60

type searchHitJSON struct {
	ID           string `json:"id"`
	Title        string `json:"title"`
	Type         string `json:"type"`
	SpaceKey     string `json:"space_key"`
	SpaceID      string `json:"space_id,omitempty"`
	Excerpt      string `json:"excerpt"`
	URL          string `json:"url"`
	LastModified string `json:"last_modified,omitempty"`
}

func newSearchCmd(d Deps) *cobra.Command {
	var (
		f       cql.Filter
		title   bool
		text    bool
		sortBy  string
		helpCQL bool
		lf      listFlags
	)
	c := &cobra.Command{
		Use:   "search [query]",
		Short: "ページを検索する",
		Long: `検索語・CQL・絞り込み条件でコンテンツを検索する。

検索語は既定で siteSearch（Confluence の画面の検索と同じ順位付け）で検索する。
--title はタイトルの部分一致、--text は本文の全文検索だけを行う。`,
		Example: `  confluence-cli search "error handling"
  confluence-cli search "API" --space-key DEV --since 30d
  confluence-cli search "setup" --title
  confluence-cli search --cql "type = page AND label = important"`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if helpCQL {
				_, err := fmt.Fprint(d.Stdout, cqlHelp)
				return err
			}
			limit, all, err := lf.args()
			if err != nil {
				return err
			}
			if len(args) == 1 {
				f.Query = args[0]
			}
			if f.Query != "" && f.RawCQL != "" {
				return validationf("検索語と --cql は同時に指定できない")
			}
			// --type は既定値を持つため、cql.Build の「条件が無い」検査に掛からない。既定値だけで全ページを検索しないよう、ここで検査する。
			if f.Query == "" && f.RawCQL == "" && f.SpaceKey == "" && len(f.Labels) == 0 && f.Since == "" && f.Author == "" && !cmd.Flags().Changed("type") {
				return validationf("検索語、--cql、または絞り込み条件を指定する")
			}
			if (title || text) && f.Query == "" {
				return validationf("--title と --text には検索語が必要")
			}
			switch {
			case title:
				f.Mode = cql.ModeTitle
			case text:
				f.Mode = cql.ModeText
			default:
				f.Mode = cql.ModeSite
			}
			switch sortBy {
			case "relevance":
				f.Sort = cql.SortRelevance
			case "modified":
				f.Sort = cql.SortModified
			default:
				return validationf("--sort に指定できない値: %q（relevance, modified のいずれか）", sortBy)
			}
			// --cql の中で type を指定していることがあるため、--cql を使うときは --type を明示した場合だけ連結する。
			if f.RawCQL != "" && !cmd.Flags().Changed("type") {
				f.Type = ""
			}

			app, err := buildApp(cmd.Context(), d)
			if err != nil {
				return err
			}
			hits, warns, err := app.Search.Search(cmd.Context(), f, limit, all)
			if err != nil {
				return err
			}
			for _, w := range warns {
				fmt.Fprintln(d.Stderr, "Warning: "+w)
			}

			// タイトル検索の結果表には space-id も出す。検索 API は space-id を返さないため、ページ API で補う。
			var spaceIDs map[domain.PageID]string
			if title {
				ids := make([]domain.PageID, 0, len(hits))
				for _, h := range hits {
					ids = append(ids, h.ID)
				}
				pages, err := app.Tree.Pages(cmd.Context(), ids)
				if err != nil {
					return err
				}
				spaceIDs = make(map[domain.PageID]string, len(pages))
				for _, p := range pages {
					spaceIDs[p.ID] = p.SpaceID
				}
			}

			if lf.format == "json" {
				out := make([]searchHitJSON, 0, len(hits))
				for _, h := range hits {
					j := searchHitJSON{
						ID: string(h.ID), Title: h.Title, Type: h.Type, SpaceKey: h.SpaceKey,
						SpaceID: spaceIDs[h.ID], Excerpt: output.CleanExcerpt(h.Excerpt), URL: h.URL,
					}
					if !h.LastModified.IsZero() {
						j.LastModified = h.LastModified.Format(time.RFC3339)
					}
					out = append(out, j)
				}
				return output.JSON(d.Stdout, out)
			}

			header := []string{"ID", "TITLE", "SPACE", "MODIFIED", "EXCERPT"}
			if title {
				header = []string{"ID", "TITLE", "SPACE", "SPACE ID", "MODIFIED", "EXCERPT"}
			}
			rows := make([][]string, 0, len(hits))
			for _, h := range hits {
				excerpt := output.Truncate(output.CleanExcerpt(h.Excerpt), excerptRunes)
				if title {
					rows = append(rows, []string{string(h.ID), h.Title, h.SpaceKey, spaceIDs[h.ID], date(h.LastModified), excerpt})
				} else {
					rows = append(rows, []string{string(h.ID), h.Title, h.SpaceKey, date(h.LastModified), excerpt})
				}
			}
			return output.Table(d.Stdout, header, rows)
		},
	}
	fl := c.Flags()
	fl.BoolVar(&title, "title", false, "タイトルの部分一致で検索する")
	fl.BoolVar(&text, "text", false, "本文の全文検索だけを行う")
	c.MarkFlagsMutuallyExclusive("title", "text")
	fl.StringVar(&f.RawCQL, "cql", "", "CQL で検索する（絞り込みフラグと AND で結合する）")
	fl.StringVar(&f.SpaceKey, "space-key", "", "スペースキーで絞り込む")
	fl.StringArrayVar(&f.Labels, "label", nil, "ラベルで絞り込む（複数指定は AND）")
	fl.StringVar(&f.Type, "type", "page", "コンテンツの種類（page, blogpost など）")
	fl.StringVar(&f.Since, "since", "", "この時点以降に更新されたもの（30d, 2w, 2026-01-01 など）")
	fl.StringVar(&f.Author, "author", "", "作成者の accountId（me で自分）")
	fl.StringVar(&sortBy, "sort", "relevance", "並び順（relevance, modified）")
	fl.BoolVar(&helpCQL, "help-cql", false, "CQL の書き方を表示する")
	addListFlags(c, &lf, 10)
	return c
}

const cqlHelp = `CQL (Confluence Query Language)

構文
  field operator value [AND|OR field operator value]

主なフィールド
  text          本文
  title         タイトル
  siteSearch    タイトルと本文（画面の検索と同じ順位付け）
  space         スペースキー
  type          page, blogpost, comment, attachment
  label         ラベル名
  creator       作成者の accountId
  created       作成日時
  lastmodified  最終更新日時

演算子
  =  !=  ~  !~  >  <  >=  <=  IN  NOT IN

例
  confluence-cli search --cql 'text ~ "API"'
  confluence-cli search --cql 'space = DEV AND title ~ "design"'
  confluence-cli search --cql 'label = "api" AND label = "v2"'
  confluence-cli search --cql 'lastmodified >= now("-7d") ORDER BY lastmodified DESC'

日付関数
  now("-1d")  now("-1w")  now("-1M")  now("-1y")

詳細: https://developer.atlassian.com/cloud/confluence/advanced-searching-using-cql/
`
