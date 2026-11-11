package cli

import (
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/genshibangou16/confluence-cli/internal/cli/output"
	"github.com/genshibangou16/confluence-cli/internal/domain"
	"github.com/genshibangou16/confluence-cli/internal/usecase"
)

// bodyFlags は page create と page update に共通の、本文と出力形式のフラグ。
type bodyFlags struct {
	body, bodyFile string
	inputFormat    string
	format         string
}

func addBodyFlags(c *cobra.Command, f *bodyFlags) {
	c.Flags().StringVar(&f.body, "body", "", "本文")
	c.Flags().StringVar(&f.bodyFile, "body-file", "", "本文を読むファイル（- なら標準入力）")
	c.Flags().StringVar(&f.inputFormat, "input-format", string(usecase.InputStorage), "本文の形式（storage, markdown）")
	c.Flags().StringVar(&f.format, "format", "text", "出力形式（text, json）")
}

// check は API を呼ぶ前に判定できるフラグの誤りを返す。
func (f bodyFlags) check(cmd *cobra.Command) error {
	if err := checkFormat(f.format, "text", "json"); err != nil {
		return err
	}
	if f.inputFormat != string(usecase.InputStorage) && f.inputFormat != string(usecase.InputMarkdown) {
		return validationf("--input-format に指定できない値です: %q（storage, markdown のいずれか）", f.inputFormat)
	}
	if cmd.Flags().Changed("body") && cmd.Flags().Changed("body-file") {
		return validationf("--body と --body-file は同時に指定できません")
	}
	return nil
}

// maxBodyBytes は --body-file から読む本文の上限。誤って巨大なファイルやストリームを渡したときに、全体をメモリに読み込まないようにする。
// テストで小さな値に差し替えるため変数にしている。
var maxBodyBytes int64 = 32 << 20

// read は指定された本文を返す。--body も --body-file も無ければ nil を返す。
func (f bodyFlags) read(cmd *cobra.Command, stdin io.Reader) (*string, error) {
	switch {
	case cmd.Flags().Changed("body"):
		return &f.body, nil
	case !cmd.Flags().Changed("body-file"):
		return nil, nil
	}
	r := stdin
	if f.bodyFile != "-" {
		file, err := os.Open(f.bodyFile)
		if err != nil {
			return nil, validationf("--body-file を読めません: %v", err)
		}
		defer file.Close()
		r = file
	}
	b, err := io.ReadAll(io.LimitReader(r, maxBodyBytes+1))
	if err != nil {
		return nil, validationf("--body-file を読めません: %v", err)
	}
	if int64(len(b)) > maxBodyBytes {
		return nil, validationf("--body-file の本文が %d バイトを超えています", maxBodyBytes)
	}
	s := string(b)
	return &s, nil
}

type writeResultJSON struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Version int    `json:"version"`
	URL     string `json:"url"`
}

type dryRunJSON struct {
	DryRun   bool     `json:"dryRun"`
	Diff     string   `json:"diff"`
	Warnings []string `json:"warnings"`
}

// writeResult は create と update の結果を出し、err をそのまま返す。
// 書き込み後の確認で失敗したときも、書き込んだページを利用者が特定できるように、ページの情報を出してから err を返す。
func writeResult(d Deps, format string, res usecase.WriteResult, err error) error {
	if res.DryRun {
		if format == "json" {
			w := res.Warnings
			if w == nil {
				w = []string{}
			}
			return output.JSON(d.Stdout, dryRunJSON{DryRun: true, Diff: res.Diff, Warnings: w})
		}
		for _, w := range res.Warnings {
			fmt.Fprintln(d.Stderr, "warning: "+output.SanitizeText(w))
		}
		_, werr := io.WriteString(d.Stdout, output.SanitizeMultiline(res.Diff))
		return werr
	}
	p := res.Page
	if p.ID == "" {
		return err
	}
	if format == "json" {
		if jerr := output.JSON(d.Stdout, writeResultJSON{ID: string(p.ID), Title: p.Title, Version: p.Version.Number, URL: p.WebURL}); jerr != nil && err == nil {
			return jerr
		}
		return err
	}
	fmt.Fprintf(d.Stdout, "id: %s\ntitle: %s\nversion: %d\nurl: %s\n",
		p.ID, output.SanitizeText(p.Title), p.Version.Number, output.SanitizeText(p.WebURL))
	return err
}

func newPageCreateCmd(d Deps) *cobra.Command {
	var (
		bf                      bodyFlags
		spaceKey, spaceID       string
		parent, parentID, title string
	)
	c := &cobra.Command{
		Use:   "create",
		Short: "ページを作成する",
		Long: `ページを作成する。

--parent を指定すると親ページと同じスペースに作るため、スペースの指定は省略できる。
--format text（既定）は id・title・version・url の 4 行を出す。--format json は同じ項目を 1 行の JSON で出す。
作成後に本文を取得し直し、送った本文から失われたテキストがあれば、ページの情報を出してからエラーで終了する。`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := bf.check(cmd); err != nil {
				return err
			}
			if cmd.Flags().Changed("parent") && cmd.Flags().Changed("parent-id") {
				return validationf("--parent と --parent-id は同時に指定できません")
			}
			if cmd.Flags().Changed("parent-id") {
				parent = parentID
			}
			var pid domain.PageID
			if parent != "" {
				var err error
				if pid, err = domain.ParsePageRef(parent); err != nil {
					return err
				}
			}
			body, err := bf.read(cmd, d.Stdin)
			if err != nil {
				return err
			}
			req := usecase.CreateRequest{ParentID: pid, Title: title, Input: usecase.InputFormat(bf.inputFormat)}
			if body != nil {
				req.Body = *body
			}
			app, err := buildApp(cmd.Context(), d)
			if err != nil {
				return err
			}
			// --space-key をスペース ID に変換する API の呼び出しより前に、read_only を確認する。
			if err := app.Guard.CheckReadOnly(); err != nil {
				return err
			}
			if req.SpaceID, err = app.Spaces.ResolveID(cmd.Context(), spaceKey, spaceID); err != nil {
				return err
			}
			res, err := app.Commands.Create(cmd.Context(), req)
			return writeResult(d, bf.format, res, err)
		},
	}
	c.Flags().StringVar(&spaceKey, "space-key", "", "作成先のスペースキー")
	c.Flags().StringVar(&spaceID, "space-id", "", "作成先の space-id")
	c.Flags().StringVar(&parent, "parent", "", "親ページ（ページ ID・URL・短縮 URL）")
	c.Flags().StringVar(&parentID, "parent-id", "", "--parent の別名")
	_ = c.Flags().MarkHidden("parent-id")
	c.Flags().StringVar(&title, "title", "", "タイトル（必須）")
	addBodyFlags(c, &bf)
	return c
}

func newPageUpdateCmd(d Deps) *cobra.Command {
	var (
		bf                                    bodyFlags
		title, section, message               string
		appendBody, prependBody               bool
		expectVersion                         int
		dryRun, allowLossy, allowDropComments bool
	)
	c := &cobra.Command{
		Use:   "update <page>",
		Short: "ページを更新する",
		Long: `ページのタイトルと本文を更新する。

--title を省略すると現在のタイトルを、--body と --body-file を省略すると現在の本文をそのまま残す。
--append・--prepend・--section は同時に指定できず、どれも本文の指定が要る。指定しなかった部分の本文は変えない。
--dry-run は更新せず、現在の本文と新しい本文の差分を標準出力に、警告を標準エラーに出す。
--format text（既定）は更新後の id・title・version・url の 4 行を出す。--format json は同じ項目を 1 行の JSON で出す。
更新後に本文を取得し直し、送った本文から失われたテキストがあれば、ページの情報を出してからエラーで終了する。`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := bf.check(cmd); err != nil {
				return err
			}
			req := usecase.UpdateRequest{
				Input: usecase.InputFormat(bf.inputFormat), Section: section,
				ExpectedVersion: expectVersion, Message: message, DryRun: dryRun,
				AllowLossy: allowLossy, AllowDropComments: allowDropComments,
			}
			modes := 0
			if appendBody {
				modes++
				req.Mode = usecase.ModeAppend
			}
			if prependBody {
				modes++
				req.Mode = usecase.ModePrepend
			}
			if cmd.Flags().Changed("section") {
				modes++
				req.Mode = usecase.ModeSection
			}
			if modes > 1 {
				return validationf("--append・--prepend・--section は同時に指定できません")
			}
			// usecase は ExpectedVersion が 0 なら版を確認しないため、明示した 0 を受け付けると確認がされない。
			if cmd.Flags().Changed("expect-version") && expectVersion < 1 {
				return validationf("--expect-version には 1 以上の版番号を指定してください")
			}
			if cmd.Flags().Changed("title") {
				req.Title = &title
			}
			id, err := domain.ParsePageRef(args[0])
			if err != nil {
				return err
			}
			req.ID = id
			if req.Body, err = bf.read(cmd, d.Stdin); err != nil {
				return err
			}
			app, err := buildApp(cmd.Context(), d)
			if err != nil {
				return err
			}
			res, err := app.Commands.Update(cmd.Context(), req)
			return writeResult(d, bf.format, res, err)
		},
	}
	c.Flags().StringVar(&title, "title", "", "新しいタイトル")
	c.Flags().BoolVar(&appendBody, "append", false, "本文を現在の本文の末尾に足す")
	c.Flags().BoolVar(&prependBody, "prepend", false, "本文を現在の本文の先頭に足す")
	c.Flags().StringVar(&section, "section", "", "指定した見出しの節の中身を本文で置き換える")
	c.Flags().IntVar(&expectVersion, "expect-version", 0, "現在の版番号がこの値でなければ更新しない")
	c.Flags().StringVar(&message, "message", "", "版コメント")
	c.Flags().BoolVar(&dryRun, "dry-run", false, "更新せずに差分を表示する")
	c.Flags().BoolVar(&allowLossy, "allow-lossy", false, "Markdown で表せない要素が失われても本文全体を置き換える")
	c.Flags().BoolVar(&allowDropComments, "allow-drop-comments", false, "インラインコメントのマーカーが外れても更新する")
	addBodyFlags(c, &bf)
	return c
}

type deletedJSON struct {
	Deleted string `json:"deleted"`
}

func newPageDeleteCmd(d Deps) *cobra.Command {
	var format string
	c := &cobra.Command{
		Use:   "delete <page>",
		Short: "ページを削除する",
		Args:  cobra.ExactArgs(1),
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
			if err := app.Commands.Delete(cmd.Context(), id); err != nil {
				return err
			}
			if format == "json" {
				return output.JSON(d.Stdout, deletedJSON{Deleted: string(id)})
			}
			fmt.Fprintf(d.Stdout, "deleted: %s\n", id)
			return nil
		},
	}
	c.Flags().StringVar(&format, "format", "text", "出力形式（text, json）")
	return c
}
