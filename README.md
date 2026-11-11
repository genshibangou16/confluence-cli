# confluence-cli

Atlassian Confluence Cloud のページの検索・取得・作成・更新・削除と、ラベルの操作を行う CLI。Go で書かれた単一バイナリで動く。

## インストール

### go install

```bash
go install github.com/genshibangou16/confluence-cli@latest
```

バイナリは `$GOPATH/bin`（通常は `~/go/bin`）に入る。

### リリースのバイナリ

[GitHub Releases](https://github.com/genshibangou16/confluence-cli/releases) のアセット名は `confluence-cli-<os>-<arch>` で、Windows だけ末尾に `.exe` が付く。

```bash
# macOS (Apple Silicon)
sudo curl -L https://github.com/genshibangou16/confluence-cli/releases/latest/download/confluence-cli-darwin-arm64 -o /usr/local/bin/confluence-cli
sudo chmod +x /usr/local/bin/confluence-cli
```

ほかのアセットは `confluence-cli-darwin-amd64`、`confluence-cli-linux-amd64`、`confluence-cli-linux-arm64`、`confluence-cli-windows-amd64.exe`。

### ソースからビルド

```bash
git clone https://github.com/genshibangou16/confluence-cli.git
cd confluence-cli
make install        # go install . と同じ。$GOPATH/bin に入る
make build          # ./confluence-cli を作る
make release-build  # 全プラットフォーム向けの confluence-cli-<os>-<arch> を作る
```

## 設定

`confluence-cli config init` で、ユーザー設定ファイルに `base_url`・`email`・`api_token` を書き込む。

### 設定の優先順位

高い順に次のとおり。

1. 環境変数（`CONFLUENCE_BASE_URL`、`CONFLUENCE_EMAIL`、`CONFLUENCE_API_TOKEN`、`CONFLUENCE_READ_ONLY`）
2. ユーザー設定ファイル（`$XDG_CONFIG_HOME/confluence-cli/config.yaml`。`XDG_CONFIG_HOME` が未設定なら `~/.config/confluence-cli/config.yaml`）
3. ローカル設定ファイル（`.confluence-cli.yaml`。カレントディレクトリから親ディレクトリへ向かって探す）

`config path` で設定ファイルのパス、`config show` で読み込んだ設定を確認できる。`config show` は API トークンを先頭 4 文字だけ表示する。

API トークンは、環境変数 `CONFLUENCE_API_TOKEN`、`api_token_command`、`api_token` の順に使う。

### ユーザー設定の例

```yaml
base_url: https://example.atlassian.net/wiki
email: you@example.com
api_token_command: ["security", "find-generic-password", "-s", "confluence-cli", "-w"]
read_only: false
writable_spaces: [DEV, DOCS]
```

- `base_url`: `https` の URL だけを指定できる。認証情報を URL に含められない。
- `api_token_command`: コマンドと引数の配列。シェルを介さずに実行し、標準出力の 1 行目を API トークンとして使う。上の例は macOS のキーチェーンから取り出す。トークンを設定ファイルに平文で置かずに済む。
- `read_only`: `true` にすると、`page create`・`page update`・`page delete`・`label add`・`label remove` を実行しない。環境変数 `CONFLUENCE_READ_ONLY`（`1`、`true`、`yes`、`on`）でも指定できる。認識できない値は `true` として扱い、警告を出す。
- `writable_spaces`: 書き込みを許可するスペースキーの一覧。指定すると、一覧にないスペースのページへの書き込みを拒否する。省略すると制限しない。空の配列にすると、すべてのスペースへの書き込みを拒否する。

### ローカル設定

チームでリポジトリに置く `.confluence-cli.yaml` には、`config init-local` で `base_url` だけを書ける。

```yaml
base_url: https://example.atlassian.net/wiki
```

ローカル設定には次の制限がある。

- 採用するのは `base_url`・`read_only`・`writable_spaces` だけ。`email`・`api_token`・`api_token_command` は読まず、警告を出す。
- `base_url` は `https` の `*.atlassian.net` だけを採用する。環境変数にもユーザー設定にも `base_url` がないときに限り使い、使うときは警告を出す。
- `read_only` はユーザー設定・環境変数のいずれかが `true` なら `true` になり、ローカル設定では書き込み制限を緩められない。`writable_spaces` はユーザー設定と共通するスペースだけが残る。
- シンボリックリンク、所有者が実行ユーザーでないファイル、グループ・その他に書き込み権限があるファイルは読まない。ホームディレクトリより上は探さない。

### API トークン

<https://id.atlassian.com/manage-profile/security/api-tokens> で作る。クラシックの API トークンとスコープ付きの API トークンのどちらも使える。

スコープ付きの API トークンには、使うコマンドに応じて次のスコープを付ける。

| スコープ | 使うコマンド |
| --- | --- |
| `read:page:confluence` | `page get`、`page list`、`page versions`、`label pages`、`page update`・`page create` の事後確認 |
| `write:page:confluence` | `page create`、`page update` |
| `delete:page:confluence` | `page delete` |
| `read:space:confluence` | スペースキーの指定、`writable_spaces` の判定 |
| `read:label:confluence` | `label list`、`label add` |
| `write:label:confluence` | `label add`、`label remove` |
| `read:content.metadata:confluence` | `page ancestors` |
| `read:hierarchical-content:confluence` | `page children` |
| `search:confluence` | `search` |

読み取りだけなら `read:*` 系と `search:confluence` で足りる。

## コマンド

引数の `<page>` には、ページ ID、ページの URL、短縮 URL（`/wiki/x/...`）のいずれかを指定できる。詳しいフラグは各コマンドの `--help` で確認できる。

### search

```bash
confluence-cli search "error handling"                       # siteSearch（Confluence の画面の検索と同じ順位付け）
confluence-cli search "setup" --title                        # タイトルの部分一致
confluence-cli search "keyword" --text                       # 本文の全文検索だけ
confluence-cli search "API" --space-key DEV --since 30d      # スペースと更新日で絞り込む
confluence-cli search --cql "type = page AND label = important"
confluence-cli search --help-cql                             # CQL の書き方
```

主なフラグ: `--space-key`、`--label`（複数指定は AND）、`--author`（accountId。`me` で自分）、`--since`（`30d`、`2w`、`2026-01-01`）、`--type`、`--sort relevance|modified`、`--limit`、`--all`、`--format table|json`。

### page

```bash
confluence-cli page get <page>                      # Markdown で標準出力に出す
confluence-cli page get <page> --outline            # 見出しの一覧
confluence-cli page get <page> --section "見出し"    # 指定した節だけ
confluence-cli page get <page> --format storage     # storage 形式
confluence-cli page get <page> --format json [--content-format markdown|storage] [--version <n>]
confluence-cli page list [--space-key <key> | --space-id <id>] [--limit <n> | --all]
confluence-cli page children <page>
confluence-cli page ancestors <page>
confluence-cli page versions <page>
confluence-cli page create --title <title> --parent <page> --body-file page.md --input-format markdown
confluence-cli page update <page> --title <title> --body "<p>...</p>"
confluence-cli page update <page> --body-file add.md --input-format markdown --append
confluence-cli page update <page> --section "見出し" --body-file part.md --input-format markdown
confluence-cli page update <page> --body "<p>x</p>" --dry-run
confluence-cli page delete <page>
```

- `page get` の Markdown（既定）と `--outline` は、標準出力に本文か見出しの一覧だけを出し、id・title・version・url の 4 行と区切りの `---` を標準エラーに出す。`--format storage` は本文だけを標準出力に、版番号を標準エラーに出す。どちらも標準出力をファイルに保存して編集し、`page update <page> --body-file <file>`（Markdown なら `--input-format markdown` も付ける）で戻せる。
- `page create` と `page update` の本文は `--body` か `--body-file`（`-` なら標準入力）で渡す。形式は `--input-format storage|markdown`（既定は `storage`）。
- `page update` の `--append`・`--prepend`・`--section` は同時に指定できず、指定しなかった部分の本文は変えない。
- `--expect-version <n>` は、現在の版番号が `n` でなければ更新しない。`--message` は版コメント。
- `--dry-run` は更新せず、現在の本文と新しい本文の差分を出す。書き込まないため、`read_only` と `writable_spaces` では拒否しない。
- 本文全体か `--section` の節を置き換えるときに、置き換える範囲のインラインコメントのマーカーが新しい本文に残らなければ、`--allow-drop-comments` を付けない限り更新しない。storage 形式の入力では、同じ `ac:ref` のマーカーを新しい本文に含めればマーカーは残る。Markdown の入力ではマーカーを表せないため、範囲のマーカーはすべて外れる。
- Markdown の入力で本文全体か `--section` の節を置き換えるときに、置き換える範囲に Markdown で表せない要素があれば、`--allow-lossy` を付けない限り更新しない。
- `page create` と `page update` は、更新後に本文を取得し直し、送った本文から失われたテキストがあればページの情報を出してからエラーで終了する。
- `--format json` は、失敗時の標準エラーにも JSON を出す。

### エラー

失敗したときの終了コードは常に 1。標準エラーには `Error: <メッセージ>` を出し、対処方法があれば `Hint: <対処方法>` を続ける。`--format json` のコマンドでは `{"error":{"kind":...,"status":...,"message":...,"hint":...}}` を出す。`kind` は次のいずれか。

| kind | 意味 |
| --- | --- |
| `auth` | 認証の失敗（HTTP 401） |
| `forbidden` | 権限がない（HTTP 403） |
| `not_found` | 対象がない（HTTP 404） |
| `conflict` | 版の競合など（HTTP 409） |
| `validation` | 引数・設定・本文の誤り（HTTP 400 を含む） |
| `rate_limit` | リクエスト数の制限（HTTP 429） |
| `read_only` | `read_only` か `writable_spaces` により書き込みを拒否した |
| `version_mismatch` | `--expect-version` と現在の版番号が異なる |
| `lossy` | Markdown で表せない要素が失われるため更新しなかった |
| `comment_loss` | インラインコメントのマーカーが外れるため更新しなかった |
| `verify_failed` | 書き込みは完了したが、書き込み後の本文の確認で失敗した（テキストが失われたか、読み戻しに失敗した） |
| `api` | 上記以外の API の失敗と通信の失敗 |

### label

```bash
confluence-cli label list [--prefix my|team|global|system]
confluence-cli label pages <label-id> [--space-key <key>]
confluence-cli label add <page> <label>...
confluence-cli label remove <page> <label>
```

### config

```bash
confluence-cli config init        # ユーザー設定を作る・更新する
confluence-cli config init-local  # カレントディレクトリに .confluence-cli.yaml を作る
confluence-cli config show
confluence-cli config path
```

## テスト

```bash
go vet ./... && go test ./...
```

### E2E テスト

実際の Confluence に対して、ビルドしたバイナリを実行する。ビルドタグ `e2e` を付けたときだけ対象になる。

```bash
CONFLUENCE_E2E_ROOT=<テストルートのページ ID> go test -tags e2e -count=1 ./e2e/...
```

- 認証情報は通常の設定（設定ファイルか環境変数）から読む。API トークンにはページの読み書きと削除、ラベルの読み書きのスコープが要る。
- テスト用に空のページを 1 つ用意し、その ID を `CONFLUENCE_E2E_ROOT` に指定する。書き込みはこのページの子孫にだけ行う。環境変数が未設定なら、E2E テストはスキップする。
- テストの開始時に、中断した実行が残した `e2e-` で始まるテストルートの子ページを削除する。終了時に、作ったページを削除し、テストルートが開始時のタイトル・本文・版番号のままかを確認する。
- テストルートの ID は、ファイルやコミットに書かない。

## API

- Confluence REST API v2（ページ・ラベル・スペース）と v1（CQL 検索、ラベルの付け外し）を使う。
- [REST API v2](https://developer.atlassian.com/cloud/confluence/rest/v2/intro#about)
- [CQL](https://developer.atlassian.com/cloud/confluence/advanced-searching-using-cql/)
