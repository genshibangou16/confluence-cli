# confluence-cli 全体改修 設計書

## 目的

confluence-cli は、利用者本人と Claude（confluence-cli スキル経由）が社内の Confluence Cloudを検索・閲覧・作成・更新するための CLI である。フォーク元のリポジトリに出した PR に反応がないため、利用者のリポジトリ（`genshibangou16/confluence-cli`）で開発を続ける。

今回の改修の目的は次のとおり。

- 検索で目的のページを見つけやすくする
- 出力のトークン量を減らす（主な読み手は Claude）
- 更新で既存の内容を意図せず消さない
- 認証情報の扱いを安全にする
- 依存と Go のバージョンを最新にする

完了の条件は、改修後のバイナリを `~/go/bin/confluence-cli`（スキルが呼び出す実行ファイル）に置き換え、実際の Confluence に対して読み取り系コマンドが動くことである。

## 現状（2026-11-10 時点の調査結果）

- Go 1.27.3 が `/opt/homebrew/bin/go` に入っている。
- `origin/feature/scoped-api-token` は main より 2 コミット進んでおり、fast-forward でマージできる。現在の `~/go/bin/confluence-cli` は、このブランチからビルドしたものである。
- 依存: cobra 1.8.0（最新 1.10.2）、pflag 1.0.5（最新 1.1.0）、yaml.v3 3.0.1。go.mod の `go` ディレクティブは 1.21。
- モジュールパスはフォーク元の `github.com/ryo-imai-bit/confluence-cli` のまま。
- テストは `internal/client/cloud_id_test.go` だけ。

実機で再現した不具合は次のとおり。

- `search --title` は v2 `GET /pages?title=` を使っており、完全一致でしか見つからない。`--space-key` は渡されずに無視される。
- 全文検索（v1 `/rest/api/content/search`）の結果で、SPACE 列と EXCERPT 列が空になる。
- 検索語に `"` を含むと CQL の解析エラーになる（`fmt.Sprintf("text ~ \"%s\"")` でエスケープしていない）。
- `page get` の text 形式は storage 形式の HTML をそのまま出力する（試したページで 55KB）。
- エラー時に cobra の usage が毎回出力される。

コードから読み取った問題は次のとおり。

- `page update` で `--body` を省略すると、本文が空文字で上書きされる。
- `cleanExcerpt` がバイト単位で切り詰めるため、マルチバイト文字が途中で切れる。
- `page list` は表に出さない本文を `body-format=storage` で毎回取得している。
- どのコマンドもページングに対応しておらず、`--limit` の上限（v2 は 250）を超える件数を取得できない。
- `config init` で API トークンの入力が画面に表示される。
- ローカル設定 `.confluence-cli.yaml` は親ディレクトリまでさかのぼって探され、`base_url`・`email`・`api_token` のすべてを読む。clone したリポジトリに置かれた設定ファイルで、認証情報の送信先を任意のホストに変えられる。`http://` の URL も受け付ける。
- 429 応答のリトライがない。
- `_edge/tenant_info`（cloudId の取得）は公式ドキュメントに載っていない API で、コマンドの実行ごとに 1 回呼ばれる。

## Confluence API の前提（公式仕様で確認済み）

- CQL 検索は v2 に代替がなく、v1 の `/wiki/rest/api/search` を使う。v1 仕様で deprecated になっていない。
- ラベルの付与・削除は v2 になく、v1 の `POST /wiki/rest/api/content/{id}/label` と `DELETE /wiki/rest/api/content/{id}/label?name=` を使う。
- Markdown での入出力は公式 API にない。変換は CLI 側で行う。
- `GET /pages/{id}` の `body-format` は `storage`・`atlas_doc_format`・`view`・`export_view` などを受け付ける。`GET /pages`（一覧）は `storage` と `atlas_doc_format` だけ。
- `PUT /pages/{id}` には現在の版番号 +1 を渡す。`version.message` で版コメントを付けられる。承認必須のスペースでは 409 が返る。
- 子ページは `/pages/{id}/direct-children` を使う（`/pages/{id}/children` は deprecated）。祖先は `/pages/{id}/ancestors`、版履歴は `/pages/{id}/versions`。
- v2 のページングは `Link` ヘッダーの next、v1 検索は `_links.next` を使う。
- API トークンには最長 1 年の有効期限がある。スコープ付きトークンは `https://api.atlassian.com/ex/confluence/{cloudId}` に basic 認証で送る。
- API トークンでのアクセスは、ポイント制のレート制限の対象外で、従来のバースト制限がかかる。429 では `Retry-After` が返る。

公式 OpenAPI 仕様は `docs/openapi-v2.v3.json`（v2、リポジトリ同梱、古い版）にある。最新の v2・v1 仕様は調査時に scratchpad に取得したが、リポジトリには入っていない。v2 仕様は最新版に差し替える。

## ゴール

### 1. `feature/scoped-api-token` のマージと作業の進め方

- `feature/scoped-api-token` を main に fast-forward でマージし、`origin/main` に push する（利用者の指示）。
- 以降の改修は main から切ったブランチで行い、PR を作る。PR の分け方はビルダーに任せる。
- 実装の最初に、新しく使う API（`/wiki/rest/api/search` と、ラベル一覧の v1 API）を、利用者の API トークンで読み取りだけ実行して確認する。スコープ不足で失敗したら、その時点で利用者に知らせる。

### 2. 依存と Go の更新

- 直接・間接の依存をすべて最新版に上げる（`go get -u ./... && go mod tidy`）。
- go.mod の `go` ディレクティブを、ローカルの Go に合わせて上げる。
- モジュールパスを `github.com/genshibangou16/confluence-cli` に変え、README のインストール手順と URL も合わせて直す。`go install` で作られる実行ファイル名は `confluence-cli` のまま変わらない。

### 3. リファクタリング

- API 呼び出しの「リクエスト → ステータス確認 → JSON デコード」の重複を、client パッケージの共通関数にまとめる。
- サービスごとに `client.NewClient()` を呼んでいる構造を、コマンド実行時に 1 回だけクライアントを作る形にする。
- 出力処理（table・json・text）の重複をまとめる。
- `SilenceUsage` を設定し、エラー時に usage を出さない。
- 主要な処理（CQL の組み立てとエスケープ、Markdown 変換、URL からの page-id 解決、設定の読み込み、リトライ、ページング）に、`httptest` を使ったテストを付ける。

### 4. 不具合の修正

- タイトル検索は CQL の `title ~ "..."` に変え、部分一致させる。`--space-key` を反映する。
- 全文検索は v1 `/rest/api/search` に変え、結果にスペースキー・抜粋・URL・最終更新日を出す。
- CQL に埋め込む値は、`\` と `"` をエスケープする。
- 抜粋の切り詰めは rune 単位にする。
- `page list` は本文を取得しない。

### 5. セキュリティ

- cloudId は初回に取得してキャッシュファイル（`$XDG_CACHE_HOME/confluence-cli/`、未設定なら `~/.cache/confluence-cli/`、サイトのホスト名ごと）に保存し、以降は API を呼ばない。cloudId の取得に失敗したら、エラーにせずサイト URL（`*.atlassian.net`）に直接リクエストを送る。
- ローカル設定から読む値は `base_url` だけにする。`email` と `api_token` がローカル設定にあれば無視し、標準エラーに警告を出す。
- `base_url` は `https` だけを受け付ける。ローカル設定の `base_url` は、ホストが `*.atlassian.net` のときだけ採用する。ユーザー設定と環境変数の `base_url` は任意のホストを受け付ける。
- `config init` の API トークン入力は、端末に表示しない（`golang.org/x/term` を使う）。
- 429 と 503 には、`Retry-After` の秒数だけ待ってリトライする。`Retry-After` がなければ jitter 付きの指数バックオフでリトライする。リトライの対象は GET・PUT・DELETE とし、POST（ページ作成とラベル付与）はリトライしない。
- 401・403 のエラーメッセージで、API トークンの失効（有効期限は最長 1 年）とスコープ不足を原因の候補として案内する。スコープ付きトークンでは、v1 API（検索・ラベル）に classic スコープ（`search:confluence`、`write:confluence-content` など）が要る。呼び出した API に必要なスコープ名をメッセージに含める。必要なスコープの正確な名前は公式ドキュメントで確認してから実装する。
- 設定に `api_token_command` を追加する。指定すると、そのコマンドをシェルを介さずに実行し、標準出力の 1 行目を API トークンとして使う（例: macOS のキーチェーンから取り出す `security find-generic-password -s confluence-cli -w`）。`api_token` と両方あれば `api_token_command` を優先する。ローカル設定の `api_token_command` は読まない（`api_token` と同じ理由）。
- HTTP のリダイレクトは、`https` で最初のリクエストと同じホスト（ポートを含む）へのものだけをたどる。それ以外はたどらずにエラーにする。`net/http` は、`http` へのリダイレクトや、ポートだけが違う同じホスト名へのリダイレクトでも `Authorization` ヘッダーを送り直すため。
- 警告と、`config show`・`config path` が出す設定ファイルのパスと値は、テキストで出すときに制御文字を除く。設定ファイルとディレクトリの名前に ESC などを含められるため。
- `--format json` とエラーの JSON の出力では、C1 制御文字（U+0080〜U+009F）と双方向制御文字（U+061C、U+200E、U+200F、U+202A〜U+202E、U+2066〜U+2069）を `\uXXXX` にエスケープする。encoding/json はこれらの文字をそのまま出力し、端末に表示すると Confluence 由来の文字列で端末の表示を変えられるため。デコードすると元の文字列に戻る。
- 書き込みを制限する設定を追加する。
  - `read_only: true`（環境変数 `CONFLUENCE_READ_ONLY=1`）: 書き込み系のコマンド（create・update・delete・label add/remove）を、API を呼ばずにエラーで終了させる。
  - `writable_spaces: [<space-key>, ...]`: 指定すると、書き込みの対象ページがこれらのスペースにあるときだけ書き込みを許可する。対象ページのスペースは書き込みの前に API で確認する。
  - ローカル設定でもこの 2 つを読む。ただし、ローカル設定はユーザー設定より制限を緩められない（`read_only` はどちらかが true なら true、`writable_spaces` は両方にあれば共通部分を使う）。

### 6. 機能追加

**検索**

- `search` の結果表に、スペースキー・最終更新日・抜粋を出す。
- siteSearch の検索が 400 で失敗した場合は、同じ検索語で `text ~` を使って 1 回だけ検索し直し、検索し直したことを標準エラーに出す。
- `--sort`（関連度順・更新日の新しい順）を追加する。CQL の `ORDER BY lastmodified DESC` で実現する。
- すべてのコマンドで、スペースの指定に `--space-key` を受け付ける。space-id が必要な API には、`GET /spaces?keys=` で key を ID に変換して渡す。`--space-id` も残す。
- ページングに対応する。`--limit` で指定した件数に達するまで次のページを取得する。`--all` で全件を取得する。
- 引数のクエリでの検索は、既定で CQL の `siteSearch ~ "..."`（Confluence の画面の検索と同じ順位付け）を使う。タイトルと本文の両方が対象になり、1 回の呼び出しで済む。`--title` はタイトルだけの部分一致（`title ~`）として残す。全文検索だけを行う `--text`（`text ~`）も残す。
- CQL を書かずに絞り込むフラグを追加する: `--label`（複数指定は AND）、`--type`（page・blogpost など、既定は page）、`--since`（`30d`・`2w`・`2026-01-01` などを `lastmodified >=` にする）、`--author`（accountId か、`me` で `currentUser()`）。これらは `--cql` と同時に指定でき、`--cql` の条件と AND で結合する。
- タイトル検索の結果表には、スペースキーと space-id の両方を出す。
- `--cql` に ORDER BY があれば、文字列リテラル（`"` か `'` で囲み、`\` でエスケープする）の外にある最後の ORDER BY から後ろを並び順として分け、条件部と絞り込みフラグの条件を AND で結合してから並び順を末尾に付ける。リテラルの中の ORDER BY では分けない。`--cql` の ORDER BY と `--sort` は同時に指定できない。

**トークン効率**

- `page get` に `--format markdown` を追加し、storage 形式を Markdown に変換して出力する。変換規則は次のとおり。
  - 見出し・段落・リスト・リンク・強調・テーブルは、Markdown の対応する記法にする。
  - code マクロはフェンス付きコードブロックにする（言語指定を引き継ぐ）。
  - info・note・warning・tip パネルは、書き込み時と同じ対応表で GitHub Alerts（`> [!NOTE]` など）にする。info→NOTE、tip→TIP、note→WARNING、warning→CAUTION。パネルのタイトルは Alert の 1 行目に残す。
  - 段落の中にあるパネルは `[NOTE: **タイトル** 本文]`、code マクロは `[code title=タイトル]` の後に空白とインラインコードを続けた形（タイトルがなければインラインコードだけ）にする。
  - 見出しの中の `<br/>` は空白にする。`--outline` と `--section` が比べる見出しの文字列でも空白として扱う。
  - タスクリストは `- [ ]` / `- [x]` にする。
  - ユーザーへのメンションは accountId を残した形にする。
  - 画像と添付ファイルはファイル名を残す。
  - 変換規則のないマクロは、マクロ名を残したプレースホルダーにして、内容を捨てたことが読み手に分かるようにする。
- `page get` の既定の形式を `markdown` にする。storage 形式の HTML が必要な場合は `--format storage` を使う。`--format json` も残す。
- `page get` の markdown と `--outline` は、標準出力に本文か見出しの一覧だけを出し、id・title・version・url の 4 行と区切りの `---` を標準エラーに出す。標準出力をそのまま `page update --input-format markdown --body-file -` に渡したときに、ページの情報が本文に入らないようにするため。
- `page get --format storage` は、本文を標準出力に、版番号を標準エラーに出す。標準出力が端末でなければ、取得した storage 形式をバイト単位でそのまま出す。`page update --body-file` に渡して書き戻したときに本文が一致するようにするため。標準出力が端末なら、制御文字を除き、末尾に改行が無ければ足して出す。`--section` と組み合わせたときも同じ。
- `page get --outline` で、見出しの一覧（レベルとテキスト）だけを出力する。
- `page get --section "<見出し>"` で、指定した見出しから、同じかより上位のレベルの次の見出しの直前までを出力する。見出しが複数一致したら、一致した見出しの一覧を示してエラーで終了する。`--format` のすべての形式と組み合わせられる。
- `create`・`update` の成功時の出力を、ID・タイトル・版番号・URL だけにする。`--format json` で全体を出せるようにする。
- `--format json` の出力から、ページング用の `_links.next` など、読み手が使わないフィールドを除く。
- `--format json` を指定したコマンドが失敗したら、標準エラーに JSON を 1 つだけ出す。JSON には、エラーの種類（`auth`・`not_found`・`conflict`・`validation`・`rate_limit`・`read_only` など）、HTTP ステータス、メッセージを含める。
- `--format json` を指定したコマンドは、警告をテキストで出さない。失敗したときはエラーの JSON の `warnings` に入れる。成功したときは、警告があれば `{"warnings":[...]}` の 1 行を標準エラーに出す。

**更新の正確さ**

- `page create` と `page update` で、本文を `--body-file <path>` と標準入力（`--body-file -`）から受け取れるようにする。
- `page update` で `--body` も `--body-file` も指定しなかったら、本文を変えずに残す（タイトルだけの変更）。
- `page update` で `--title` を省略したら、現在のタイトルを残す。
- `page update` に、取得したときの版番号を渡すフラグを追加する。指定した版番号と現在の版番号が違えば、更新せずにエラーで終了する。`page get` の出力には、すべての形式で版番号を含める。
- `page update` に `--message` を追加し、版コメントを付けられるようにする。
- `page update --dry-run` で、現在の本文と新しい本文の差分（unified diff）を表示し、更新しない。
- 409 が返ったら、承認必須のスペースである可能性をエラーメッセージで案内する。
- `page create` と `page update` に `--input-format markdown|storage`（既定は storage）を追加する。markdown を指定すると、CLI が storage 形式に変換して送る。変換規則は、スキルの `~/.claude/skills/confluence-cli/references/markdown-to-confluence.md` を基準にする。ただし GitHub Alerts の対応は、パネルの色が GitHub と合うように変える（下記）。スキル側は利用者が合わせて修正する。主な規則は次のとおり。
  - 見出し・段落・強調・取り消し線・インラインコード・リンク・リスト・テーブル・水平線・引用は、対応する XHTML 要素にする。外部 URL の画像は `ac:image` + `ri:url` にする。
  - フェンス付きコードブロックは code マクロにし、本文を CDATA で囲む。言語指定を `language` パラメータにする。言語名の別名は Confluence の言語名に置き換える（`sh`・`zsh`・`shell`→`bash`、`golang`→`go`、`py`・`python3`→`python`、`js`→`javascript`、`ts`→`typescript`、`yml`→`yaml` など）。Confluence が知らない言語名は `text` にする。本文に `]]>` が含まれる場合は CDATA を分割する。
  - GitHub Alerts は、NOTE→info、IMPORTANT→info、TIP→tip、WARNING→note（黄）、CAUTION→warning（赤）のパネルマクロにする。Confluence の `warning` マクロは赤、`note` マクロは黄色で、GitHub の WARNING（黄）・CAUTION（赤）と色を合わせるため。`[!TYPE]` のない普通の引用は blockquote のまま残す。
  - タスクリストは `ac:task-list` にし、`ac:task-id` に 1 からの連番を振る。`--append`・`--prepend`・`--section` では、現在の本文の `ac:task-id` の最大値の次から振る。
  - 単独行の `[TOC]` は toc マクロにする。
  - 生の HTML は送らずにエスケープする（Markdown 中の HTML タグが storage 形式に混入しない）。
- `page update` に部分更新のオプションを追加する。どれも、指定しなかった部分の storage 形式を一切変えずに残す。
  - `--append`: 新しい本文を既存の本文の末尾に足す。
  - `--prepend`: 新しい本文を既存の本文の先頭に足す。
  - `--section "<見出し>"`: 見出しの範囲（`page get --section` と同じ規則）の中身を新しい本文で置き換える。見出し自体は残す。見出しが見つからない、または複数一致したらエラーで終了する。
  - 3 つは同時に指定できない。どれも `--input-format` と `--dry-run` と組み合わせられる。
  - `--section` の範囲の終わりは、同じかより上位のレベルの次の見出しの直前とする。ただし、その見出しがマクロ（expand・excerpt など）や `ac:layout-section` の中にある場合は、そのマクロや layout の要素の開始位置を範囲の終わりにする。
- 本文を置き換える範囲（全体の置き換えでは本文全体、`--section` では対象の節）に、インラインコメントのマーカー（`ac:inline-comment-marker`）があれば、更新せずにエラーで終了し、コメントが外れることを案内する。`--allow-drop-comments` を付けたときだけ更新する。`--dry-run` ではエラーにせず警告を出す。
- `--input-format markdown` で本文全体を置き換えるとき、既存の本文に Markdown の変換規則で表せない要素（変換規則のないマクロ、`ac:layout`、結合セルのあるテーブルなど）があれば、更新せずにエラーで終了し、失われる要素の一覧を示す。`--allow-lossy` を付けたときだけ更新する。
- 入力を storage 形式にした結果が空（空白だけを含む）なら、API を呼ばずにエラーで終了する。対象は `page update` のすべての更新方法と、本文を指定した `page create`。Markdown のリンク参照定義だけの入力などは空の storage 形式になり、そのまま送ると本文や節の中身が消えるため。
- 更新と作成の後に、書き込みの応答の版番号を指定して本文を取得し直し、送った本文と比べる。応答に版番号がなければ最新版を取得する。テキストが失われていれば、失われたテキストを標準エラーに出し、終了コードを 0 以外にする。Confluence による属性の正規化（`ac:macro-id` の付与など）だけの差は報告しない。
- 送信前に、本文の storage 形式が XML として整形式かを確認する。storage 形式はそのままでは整形式の XML ではない（`ac:`・`ri:` の名前空間が宣言されておらず、`&nbsp;` などの HTML の実体参照を含む）。このため、名前空間を宣言したルート要素で包み、`encoding/xml` の `Decoder.Entity` に `xml.HTMLEntity` を設定してから検証する。整形式でなければ、行と列を示してエラーで終了し、API を呼ばない。`--input-format markdown` で変換した結果にも同じ確認を行う。本文が `</root>` で包んだルート要素を閉じる場合も整形式でないとする。`--append`・`--prepend`・`--section` では、組み立てた本文全体も送る前に確認する。

**新規コマンド**

- `label add <page> <label>...` と `label remove <page> <label>`（v1 API）。
- `page children <page>`（direct-children）と `page ancestors <page>`。
- `page versions <page>`（版の一覧）と、`page get --version <n>` による過去版の取得。
- ページを指定するすべての引数で、page-id に加えて、ページの URL（`/pages/<id>/`、`viewpage.action?pageId=<id>`）と tiny link（`/wiki/x/<code>`）を受け付ける。tiny link は `<code>` を URL-safe base64 でデコードし、4 バイトのリトルエンディアン整数として page-id を得る。

### 7. ビルドとバイナリの置き換え

`go install .` で `~/go/bin/confluence-cli` を置き換える。置き換えた後、実際の Confluence に対して次のコマンドが動くことを確認する。

- `search`（全文・タイトル・CQL）
- `page get`（markdown・storage・json）
- `page children`、`page ancestors`、`page versions`
- `label list`
- `page update --dry-run`

### 8. テスト

テストは次の 2 層で行う。E2E テストは実装中に 1 回だけ実行し、リポジトリにはユニットテストだけを置く。

**ユニットテスト**（`go test ./...`）

- HTTP は `httptest` で置き換え、ネットワークに出ない。
- 対象は、CQL の組み立てとエスケープ、storage から Markdown への変換、Markdown から storage への変換、XML の整形式チェック、見出しによる節の切り出しと置き換え、URL と tiny link からの page-id 解決、設定の読み込みと送信先の制限、cloudId のキャッシュ、リトライ、ページング、版番号の照合、各コマンドの出力形式。
- Markdown から storage への変換は、スキルの変換リファレンスにある例を入力と期待値にしたテーブルテストにする。

**E2E テスト**（実際の Confluence に対して実行した。2026-11-11 時点）

- 実装中に 1 回、利用者が指定したテスト用のページ（利用者の personal space にあるページ。以下「テストルート」）とその子ページに対して、利用者の許可を得て実行した。
- Confluence への書き込みは、今回の実装中のテストに限って利用者が許可した。このため、Confluence に書き込むテストコードはリポジトリに残していない。今後の変更の確認はユニットテストで行う。
- テストはビルドしたバイナリをサブプロセスとして実行し、標準出力・標準エラー・終了コードを確認した。書き込み系コマンドは、対象ページがテストルートかテストルートの子孫であることを API で確認してから実行した。
- 確認した内容は次のとおり。
  - 子ページの作成（storage と markdown の両方の入力）、取得、更新、削除
  - `--expect-version` での版番号の照合、`--dry-run` で本文が変わらないこと、整形式でない storage 形式で API を呼ばないこと
  - ラベルの付与・削除と、`label pages`・`search --label` での確認
  - `page children`・`page ancestors`・`page versions`・`page get --version`
  - ページ URL と tiny link での指定
  - `read_only` と `writable_spaces` の設定で書き込みが拒否され、`--dry-run` は成功すること
  - インラインコメントのマーカーと、Markdown で表せない要素による拒否
  - `--format json` での失敗時に、標準エラーに JSON が 1 つだけ出ること
  - 読み取り系の `search`（siteSearch・`--title`・`--text`・`--cql`・各絞り込みフラグ）

**既存ページの編集でフォーマットを壊さないことの確認**

利用者が最も重視している点である。次の実装上の制約とテストで確認する。

実装上の制約は次のとおり。

- 部分更新（`--append`・`--prepend`・`--section`）と、タイトルだけの更新は、既存の storage 形式を文字列として扱い、対象範囲の外側をバイト単位でそのまま残す。storage 形式を XML として解析して再シリアライズすると、属性の順序・空白・実体参照・CDATA の表記が変わるため、出力には解析結果を使わない。解析は、見出しの位置（バイトオフセット）を求めるためだけに使う。
- 見出しの探索では、マクロの中（`ac:rich-text-body` の中、たとえばパネルや expand の中）にある見出しと、トップレベルの見出しを区別する。`--section` が対象にするのはトップレベルの見出しだけとする。ただし、トップレベルの要素が `ac:layout`（ページレイアウト）の場合は、各 `ac:layout-cell` の直下をトップレベルとして扱う。

E2E テストでは、書式の多い合成フィクスチャ（`testdata/storage/*.xml`）をテストルートの子ページとして作成し、作成直後の storage 形式をベースラインとして、操作のたびにベースラインと比べた。結果は次のとおり。

- タイトルだけの更新、`--message` だけを付けた更新、`page get --format storage` の出力の `page update --body-file` での書き戻し、`--append`、`--prepend`、`--section`、`--input-format markdown` での部分更新のすべてで、既存部分はベースラインとバイト単位で一致した。
- `--section` では、expand の中にある同名の見出しは変更されなかった。
- Confluence は次の正規化を行う。
  - 新しく挿入したマクロに `ac:macro-id` を付ける。既存のマクロの `ac:macro-id` は変えない。
  - 本文の末尾の改行を削除する。

あわせて、実在するページ（テストルート以外、読み取りだけ）の storage 形式を数件取得し、ユニットテストのフィクスチャにする。このフィクスチャで次を確認する。

- XML の整形式チェックが、Confluence が実際に返す storage 形式を拒否しないこと
- 見出しの探索と節の切り出しが、実際のページで正しく動くこと
- storage から Markdown への変換が、実際のページで失敗しないこと

フィクスチャには社内の情報が含まれるため、リポジトリにはコミットせず、`.gitignore` に入れたディレクトリに置く。リポジトリには、自作した合成フィクスチャだけをコミットする。

## confluence-cli スキルの修正で合わせる変更点

スキルは利用者が別途修正する。スキルが前提にしている CLI の挙動のうち、改修で変わったものは次のとおり。

**出力**

- `page get` の既定の形式は Markdown。標準出力には本文だけを出し、id・title・version・url の 4 行と区切りの `---` を標準エラーに出す。
- `page get --format storage` は本文を標準出力に、版番号を標準エラーに出す。標準出力がパイプかファイルならバイト単位でそのまま出し、端末なら制御文字を除いて末尾に改行を足す。書き戻しに使う本文は、端末に表示したものではなく、ファイルかパイプに出したものを使う。
- `--format json` の出力は 1 行の JSON で、キーは camelCase（`spaceId`、`spaceKey`、`lastModified`、`createdAt` など）。C1 制御文字と双方向制御文字は `\uXXXX` にエスケープする。
- `page get` に `--version <n>`（過去版の取得）、`--outline`（見出しの一覧）、`--section "<見出し>"`（指定した節だけ）がある。

**検索**

- `search <query>` は既定で `siteSearch ~`（Confluence の画面の検索と同じ順位付け）を使う。`--title` はタイトルの部分一致、`--text` は本文の全文検索だけ、`--cql` は CQL での検索。
- 絞り込みのフラグは `--label`（複数指定は AND）、`--type`（既定は page）、`--since`（`30d`、`2w`、`2026-01-01`）、`--author`（accountId か `me`）、`--sort relevance|modified`、`--all`。`--cql` と同時に指定すると AND で結合する。
- スペースの指定は、すべてのコマンドで `--space-key` を受け付ける。

**ページの指定**

- `<page>` には、ページ ID、ページの URL、tiny link（`/wiki/x/<code>`）のいずれかを指定できる。

**作成と更新**

- `page create` と `page update` は `--input-format markdown|storage`（既定は storage）を受け付ける。本文は `--body` か `--body-file`（`-` なら標準入力）で渡す。
- `page update` には次のフラグがある。
  - `--append`・`--prepend`・`--section "<見出し>"`: 指定しなかった部分の本文を変えずに残す。同時には指定できない。
  - `--expect-version <n>`: 現在の版番号が `n` でなければ更新しない。
  - `--message`: 版コメント。
  - `--dry-run`: 更新せず、差分を出す。
  - `--title` は省略でき、省略すると現在のタイトルを残す。本文を指定しなければ本文を残す。
- 次の場合は、上書き用のフラグを付けない限り更新しない。
  - Markdown の入力で置き換える範囲に、Markdown で表せない要素がある（`--allow-lossy`）。
  - 置き換える範囲のインラインコメントのマーカーが新しい本文に残らない（`--allow-drop-comments`）。
- GitHub Alerts とパネルマクロの対応は、NOTE・IMPORTANT→info、TIP→tip、WARNING→note、CAUTION→warning。スキルの変換リファレンス（`references/markdown-to-confluence.md`）にある WARNING→warning、CAUTION→note とは異なる。`page get` の Markdown では逆向き（info→NOTE、tip→TIP、note→WARNING、warning→CAUTION）に変換する。
- Markdown から storage 形式への変換では、段落の中の改行の前後がどちらも全角文字なら、空白を入れずにつなげる。

**エラー**

- 失敗時の終了コードは 1。`--format json` のコマンドでは、標準エラーに `{"error":{"kind":...,"status":...,"message":...,"hint":...},"warnings":[...]}` を出す（`warnings` は警告があるときだけ）。成功時に警告があれば、標準エラーに `{"warnings":[...]}` を出す。
- `kind` は `auth`・`forbidden`・`not_found`・`conflict`・`validation`・`rate_limit`・`read_only`・`version_mismatch`・`lossy`・`comment_loss`・`verify_failed`・`api` のいずれか。
- `verify_failed` は、書き込みは完了したが、書き込み後に読み戻した本文の確認で失敗したことを表す。ページは作成・更新されているため、`page create` をやり直すと同じページが 2 つできる。やり直さずに、出力されたページを確認する。

**コマンドと設定**

- `label add <page> <label>...`、`label remove <page> <label>`、`page children <page>`、`page ancestors <page>`、`page versions <page>` がある。
- 設定に次の項目がある。
  - `read_only`（環境変数 `CONFLUENCE_READ_ONLY`）: 書き込み系のコマンドを API を呼ばずに拒否する（kind は `read_only`）。`--dry-run` は拒否しない。
  - `writable_spaces`: 書き込みを許可するスペースキーの一覧。
  - `api_token_command`: API トークンを標準出力に出すコマンドと引数の配列。

## 非ゴール

- 添付ファイルのアップロード・ダウンロード、コメント、ブログ投稿、フォルダの操作。
- `~/.claude/skills/confluence-cli` スキルの修正（利用者が別途行う。合わせる変更点は「confluence-cli スキルの修正で合わせる変更点」の節にまとめた）。改修後の CLI は、古い出力形式との互換を残さなくてよい。
- Markdown を経由した取得から更新までの完全な往復（変換規則のないマクロは失われる。既存ページの編集は部分更新か storage 形式で行う）。
- OAuth 3LO とサービスアカウントの Bearer 認証。
- GitHub Releases でのバイナリ配布の整備。

## 制約

- 設計はクリーンアーキテクチャに沿う（利用者の指示）。特に次の 3 点を重視する。
  - SOLID 原則を守る。
  - 依存の方向は、外側（cobra のコマンド、HTTP クライアント、設定ファイル、キャッシュファイル）から内側（ユースケース、ドメイン）への一方向にする。内側のパッケージは cobra・`net/http`・ファイルシステムを import しない。
  - 抽象（インターフェース）は、使う側（内側）のパッケージに、使う側が必要とするメソッドだけで定義する。実装が 1 つしかなく差し替えの予定もない箇所や、テストで置き換える必要のない箇所には、インターフェースを作らない。
- リポジトリは public である。社内サイトの URL、page-id、社内ページの内容をコミットしない（コード、テスト、ドキュメント、コミットメッセージ、PR 本文のすべて）。
- 既存のサブコマンド名とフラグは、上記で変更すると明記したもの以外は残す。スキルが現在のコマンドを使っているため。
- 外部ライブラリは追加してよい。追加するときは、メンテナンスが続いているものを選ぶ。
- コメントと docstring は、利用者の CLAUDE.md の日本語の書き方の規則に従う。

## ビルダーに任せること

- パッケージ構成と、共通化の具体的な形。
- Markdown 変換を自作するか、ライブラリ（`github.com/JohannesKaufmann/html-to-markdown/v2` など）を土台にするか。Confluence 独自要素（`ac:`・`ri:` 名前空間）の扱いは上記の変換規則を満たすこと。
- 版番号を渡すフラグの名前、リトライ回数とバックオフの上限値。
- 作業の順序とコミットの単位。
