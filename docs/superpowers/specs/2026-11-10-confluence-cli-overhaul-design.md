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

**トークン効率**

- `page get` に `--format markdown` を追加し、storage 形式を Markdown に変換して出力する。変換規則は次のとおり。
  - 見出し・段落・リスト・リンク・強調・テーブルは、Markdown の対応する記法にする。
  - code マクロはフェンス付きコードブロックにする（言語指定を引き継ぐ）。
  - info・note・warning・tip パネルは、書き込み時と同じ対応表で GitHub Alerts（`> [!NOTE]` など）にする。info→NOTE、tip→TIP、note→WARNING、warning→CAUTION。パネルのタイトルは Alert の 1 行目に残す。
  - タスクリストは `- [ ]` / `- [x]` にする。
  - ユーザーへのメンションは accountId を残した形にする。
  - 画像と添付ファイルはファイル名を残す。
  - 変換規則のないマクロは、マクロ名を残したプレースホルダーにして、内容を捨てたことが読み手に分かるようにする。
- `page get` の既定の形式を `markdown` にする。storage 形式の HTML が必要な場合は `--format storage` を使う。`--format json` も残す。
- `page get --outline` で、見出しの一覧（レベルとテキスト）だけを出力する。
- `page get --section "<見出し>"` で、指定した見出しから、同じかより上位のレベルの次の見出しの直前までを出力する。見出しが複数一致したら、一致した見出しの一覧を示してエラーで終了する。`--format` のすべての形式と組み合わせられる。
- `create`・`update` の成功時の出力を、ID・タイトル・版番号・URL だけにする。`--format json` で全体を出せるようにする。
- `--format json` の出力から、ページング用の `_links.next` など、読み手が使わないフィールドを除く。
- `--format json` を指定したコマンドが失敗したら、標準エラーに JSON を 1 つだけ出す。JSON には、エラーの種類（`auth`・`not_found`・`conflict`・`validation`・`rate_limit`・`read_only` など）、HTTP ステータス、メッセージを含める。

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
  - タスクリストは `ac:task-list` にし、`ac:task-id` に 1 からの連番を振る。
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
- 更新と作成の後に、本文を取得し直して送った本文と比べる。テキストが失われていれば、失われたテキストを標準エラーに出し、終了コードを 0 以外にする。Confluence による属性の正規化（`ac:macro-id` の付与など）だけの差は報告しない。
- 送信前に、本文の storage 形式が XML として整形式かを確認する。storage 形式はそのままでは整形式の XML ではない（`ac:`・`ri:` の名前空間が宣言されておらず、`&nbsp;` などの HTML の実体参照を含む）。このため、名前空間を宣言したルート要素で包み、`encoding/xml` の `Decoder.Entity` に `xml.HTMLEntity` を設定してから検証する。整形式でなければ、行と列を示してエラーで終了し、API を呼ばない。`--input-format markdown` で変換した結果にも同じ確認を行う。

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

テストは次の 2 層で行う。

**ユニットテスト**（`go test ./...`）

- HTTP は `httptest` で置き換え、ネットワークに出ない。
- 対象は、CQL の組み立てとエスケープ、storage から Markdown への変換、Markdown から storage への変換、XML の整形式チェック、見出しによる節の切り出しと置き換え、URL と tiny link からの page-id 解決、設定の読み込みと送信先の制限、cloudId のキャッシュ、リトライ、ページング、版番号の照合、各コマンドの出力形式。
- Markdown から storage への変換は、スキルの変換リファレンスにある例を入力と期待値にしたテーブルテストにする。

**E2E テスト**（実際の Confluence に対して実行する）

- 利用者が渡すテスト用ページ（以下「テストルート」）を使う。テストルートの page-id は環境変数 `CONFLUENCE_E2E_ROOT` で渡し、リポジトリには書かない。テストルートは利用者の personal space にあるページで、2026-11-10 時点では本文が空、子ページはない。
- ビルドタグ `e2e` を付け、`go test -tags e2e ./e2e/...` で実行する。通常の `go test ./...` では実行しない。環境変数がなければスキップする。
- テストはビルドしたバイナリをサブプロセスとして実行し、標準出力・標準エラー・終了コードを確認する。
- 書き込みが許可されているのは、テストルートとその子孫だけである。それ以外のページは読み取りだけ許可されている。これを守るため、次のようにする。
  - E2E テストのヘルパーは、書き込み系コマンドを実行する前に、対象ページがテストルートか、`ancestors` にテストルートを含むページであることを API で確認する。確認できなければテストを失敗させ、コマンドを実行しない。
  - ページの新規作成は、親にテストルートかその子孫を指定する。
  - テストで作った子ページは、テストの終了時（失敗時を含む）に削除する。
  - テストルートの本文とタイトルはテストの開始時に保存し、終了時に元に戻す。
- 確認する内容は次のとおり。
  - 子ページの作成（storage と markdown の両方の入力）、取得（markdown・storage・json・`--outline`・`--section`）、更新（全体・`--append`・`--prepend`・`--section`・タイトルだけ・`--message`）、削除
  - 版番号を渡したときの照合: 別の更新を挟んだあとに古い版番号で更新すると、エラーで終了し、本文が変わらないこと
  - `--dry-run` で本文が変わらないこと
  - 整形式でない storage 形式を渡すと、API を呼ばずにエラーで終了すること
  - ラベルの付与・削除と、`label pages`・`search --label` で付与したラベルが見えること（検索インデックスの反映待ちは、一定時間ポーリングする）
  - `page children`・`page ancestors`・`page versions`・`page get --version`
  - ページ URL と tiny link での指定
  - `read_only` と `writable_spaces` の設定で、書き込みが API を呼ばずに拒否されること（テストルートのスペースを許可リストから外した設定で確認する）
  - インラインコメントのマーカーを含む節を `--section` で置き換えると、`--allow-drop-comments` なしでは拒否されること（マーカーはフィクスチャに含める）
  - 変換規則のないマクロを含むページを `--input-format markdown` で全体を置き換えると、`--allow-lossy` なしでは拒否されること
  - `--format json` での失敗時に、標準エラーに JSON が 1 つだけ出ること
  - 読み取り系: テストルート以外のページに対する `search`（siteSearch・`--title`・`--text`・`--cql`・各絞り込みフラグ）
- 実行には利用者の API トークンを使う。CI では実行しない。

**既存ページの編集でフォーマットを壊さないことの確認**

利用者が最も重視している点である。次の実装上の制約とテストで確認する。

実装上の制約は次のとおり。

- 部分更新（`--append`・`--prepend`・`--section`）と、タイトルだけの更新は、既存の storage 形式を文字列として扱い、対象範囲の外側をバイト単位でそのまま残す。storage 形式を XML として解析して再シリアライズすると、属性の順序・空白・実体参照・CDATA の表記が変わるため、出力には解析結果を使わない。解析は、見出しの位置（バイトオフセット）を求めるためだけに使う。
- 見出しの探索では、マクロの中（`ac:rich-text-body` の中、たとえばパネルや expand の中）にある見出しと、トップレベルの見出しを区別する。`--section` が対象にするのはトップレベルの見出しだけとする。ただし、トップレベルの要素が `ac:layout`（ページレイアウト）の場合は、各 `ac:layout-cell` の直下をトップレベルとして扱う。

E2E テストでは、次の手順で確認する。

1. 書式の多いフィクスチャ（storage 形式）を用意し、テストルートの子ページとして作成する。フィクスチャには少なくとも次を含める。
   - 見出し（h1〜h3）、ネストしたリスト、結合セルを含むテーブル
   - code マクロ（CDATA の中に `<`・`&`・`]]>` の分割を含む）
   - info・note・warning・tip パネル、expand マクロ（中に見出しを含む）、status マクロ、toc マクロ
   - タスクリスト、ユーザーメンション（`ri:user`）、ページリンク（`ri:page`）、絵文字（`ac:emoticon`）、アンカー
   - ページレイアウト（`ac:layout`）、`&nbsp;` などの実体参照、日本語の本文
2. 作成直後に storage 形式を取得し、ベースラインとして保存する。ベースラインは、Confluence が保存時に行う正規化（`ac:macro-id` の付与など）を反映した状態である。
3. 次の操作を 1 つずつ行い、操作のたびに storage 形式を取得してベースラインと比べる。
   - タイトルだけの更新、`--message` だけを付けた更新: 本文がベースラインとバイト単位で一致すること
   - `page get --format storage` の出力をそのまま `page update --body-file` で書き戻す: 本文がベースラインと一致すること
   - `--append`・`--prepend`: ベースライン部分が、追加した本文の前後でバイト単位で一致すること
   - `--section`: 対象の節の外側がベースラインと一致し、置き換えた節に新しい本文が入っていること。expand の中にある同名の見出しが変更されていないこと
   - `--input-format markdown` での部分更新: 既存部分が一致し、追加部分が期待した storage 形式（マクロを含む）になっていること
4. 一致しなかった場合は、差分を出力してテストを失敗させる。
5. Confluence が更新時に既存部分を正規化し直して、CLI の処理と関係なく差分が出る場合がある。その場合は差分の内容をテスト結果に残し、正規化の内容を確認してから比較の方法を決める。比較を緩めるときは、緩めた理由をテストのコメントに書く。

あわせて、実在するページ（テストルート以外、読み取りだけ）の storage 形式を数件取得し、ユニットテストのフィクスチャにする。このフィクスチャで次を確認する。

- XML の整形式チェックが、Confluence が実際に返す storage 形式を拒否しないこと
- 見出しの探索と節の切り出しが、実際のページで正しく動くこと
- storage から Markdown への変換が、実際のページで失敗しないこと

フィクスチャには社内の情報が含まれるため、リポジトリにはコミットせず、`.gitignore` に入れたディレクトリに置く。リポジトリには、自作した合成フィクスチャだけをコミットする。

## 非ゴール

- 添付ファイルのアップロード・ダウンロード、コメント、ブログ投稿、フォルダの操作。
- `~/.claude/skills/confluence-cli` スキルの修正（利用者が別途行う）。改修後の CLI は、古い出力形式との互換を残さなくてよい。
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
