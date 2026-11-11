// Package cli はコマンドラインの解釈と出力を担い、adapter を usecase に組み込む。
package cli

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/spf13/cobra"

	"github.com/genshibangou16/confluence-cli/internal/adapter/cache"
	"github.com/genshibangou16/confluence-cli/internal/adapter/config"
	"github.com/genshibangou16/confluence-cli/internal/cli/output"
	"github.com/genshibangou16/confluence-cli/internal/domain"
)

// Deps はコマンドが使う外部の資源。テストで設定ファイルの場所・HTTP クライアント・入出力を差し替える。
// Version は --version で表示する値。
type Deps struct {
	Version  string
	Sources  config.Sources
	CacheDir string
	HTTP     *http.Client
	Stdout   io.Writer
	Stderr   io.Writer
	Stdin    io.Reader
}

// Execute は実環境の Deps でコマンドを実行し、終了コードを返す。version は --version で表示する値。
func Execute(version string) int {
	src := config.DefaultSources()
	d := Deps{
		Version:  version,
		Sources:  src,
		CacheDir: cache.DefaultDir(os.Getenv, src.Home),
		HTTP:     &http.Client{Timeout: 30 * time.Second},
		Stdout:   os.Stdout,
		Stderr:   os.Stderr,
		Stdin:    os.Stdin,
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	return run(ctx, d, os.Args[1:])
}

// run はエラーを output.WriteError で 1 回だけ出し、エラーなら 1 を返す。
// 実行したコマンドの --format が json なら、エラーも JSON で出す。
func run(ctx context.Context, d Deps, args []string) int {
	root := NewRootCmd(d)
	root.SetArgs(args)
	root.SetIn(d.Stdin)
	root.SetOut(d.Stdout)
	root.SetErr(d.Stderr)
	c, err := root.ExecuteContextC(ctx)
	if err == nil {
		return 0
	}
	asJSON := false
	if c != nil {
		if f := c.Flags().Lookup("format"); f != nil && f.Value.String() == "json" {
			asJSON = true
		}
	}
	output.WriteError(d.Stderr, err, asJSON)
	return 1
}

func NewRootCmd(d Deps) *cobra.Command {
	root := &cobra.Command{
		Use:   "confluence-cli",
		Short: "Atlassian Confluence の CLI",
		// エラーは run が output.WriteError で出すため、cobra には usage もエラーも出させない。
		SilenceUsage:  true,
		SilenceErrors: true,
		Version:       d.Version,
	}
	root.SetVersionTemplate("confluence-cli {{.Version}}\n")
	root.AddCommand(newSearchCmd(d), newPageCmd(d), newLabelCmd(d), newConfigCmd(d))
	return root
}

func validationf(format string, a ...any) error {
	return &domain.Error{Kind: domain.ErrValidation, Message: fmt.Sprintf(format, a...)}
}

func checkFormat(format string, allowed ...string) error {
	for _, a := range allowed {
		if format == a {
			return nil
		}
	}
	return validationf("--format に指定できない値です: %q（%v のいずれか）", format, allowed)
}

// listFlags は一覧系のコマンドに共通のフラグ。
type listFlags struct {
	limit  int
	all    bool
	format string
}

func addListFlags(c *cobra.Command, f *listFlags, defaultLimit int) {
	c.Flags().IntVar(&f.limit, "limit", defaultLimit, "取得する最大件数")
	c.Flags().BoolVar(&f.all, "all", false, "全件を取得する（--limit を無視する）")
	c.Flags().StringVar(&f.format, "format", "table", "出力形式（table, json）")
}

// args は adapter に渡す limit と all を返す。adapter は limit が正だと all でもその件数で打ち切るため、
// --all のときは limit を 0 にする。
func (f listFlags) args() (limit int, all bool, err error) {
	if err := checkFormat(f.format, "table", "json"); err != nil {
		return 0, false, err
	}
	if f.all {
		return 0, true, nil
	}
	if f.limit <= 0 {
		return 0, false, validationf("--limit には正の数を指定してください")
	}
	return f.limit, false, nil
}

func date(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format("2006-01-02")
}
