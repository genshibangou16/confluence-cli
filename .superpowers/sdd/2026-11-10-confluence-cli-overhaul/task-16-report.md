# Task 16 report

## 実装
- usecase.PageQuery.Get: Format markdown/storage/json、ContentFormat（json のときだけ、既定 markdown）、Outline（トップレベルの見出しのみ。空本文は空の Outline）、Section（storage.FindSection の範囲を切り出して変換）。引数の検証は API 呼び出し前。
- CLI page get: 既定 --format markdown。ヘッダー 4 行 + `---` + 本文。--outline は 4 行 + インデント付き一覧（`---` なし）。storage は本文バイト一致、版は stderr。json は {"id","title","status","spaceId","version","url","content"}（--outline 時は "outline" を足し content を省く）。--content-format は json 以外で ErrValidation。
- 判断: `--outline` と `--format storage` の併用は ErrValidation（ブリーフに指定なし）。空の --section "" は CLI で検証エラー。

## TDD
RED/GREEN を分けて記録できていない: usecase のテストと実装を同時に書いてから実行した（初回実行は旧 TestPageQuery_JSONContent が markdown 既定に変わって失敗 → 旧テストを削除し新テストに置換 → GREEN）。
`go test ./internal/usecase` -> ok / `go vet ./... && go test ./...` -> 全パッケージ ok。

## 実機確認（search で見つけた読み取り専用のページ、内容は記載しない）
- `page get <id>`: 125 行、先頭 5 行はヘッダーと `---`、HTML タグ由来の行なし（`<` を含む行はインラインコード等の文字）。
- `--outline`: 17 行（ヘッダー 4 + 13 見出し）、h2 は 2 スペースのインデント。
- `--section <見出し>`: 25 行出力。`--format storage --section`、`--format json`、`--outline --format json`（1 行）も動作。存在しない見出しは exit 1。

## ファイル
internal/usecase/page_query.go, page_query_test.go, internal/cli/page_get.go, cli_test.go
