# confluence-cli 全体改修 実装計画

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** confluence-cli を、検索の精度、出力のトークン量、更新の正確さ、認証情報の安全性の 4 点で改修し、クリーンアーキテクチャの構成に作り直す。

**Architecture:** 内側から順に、`domain`（型とエラー）、`cql`・`storage`・`convert`（外部 I/O を持たない純粋な処理）、`usecase`（アプリケーションの処理と、使う側が定義するポート）、`adapter`（Confluence の HTTP、設定ファイル、キャッシュファイル）、`cli`（cobra のコマンドと出力、依存の組み立て）を置く。依存は `cli → usecase → domain` と `adapter → domain` の向きだけにする。adapter は usecase のポートを Go の構造的型付けで満たし、usecase を import しない。

**Tech Stack:** Go（ローカルの 1.27.3）、cobra、pflag、`gopkg.in/yaml.v3`、`golang.org/x/term`、`github.com/yuin/goldmark`（Markdown の解析）、`github.com/aymanbagabas/go-udiff`（unified diff）、標準ライブラリの `encoding/xml`・`net/http/httptest`。

**Spec:** `docs/superpowers/specs/2026-11-10-confluence-cli-overhaul-design.md`

## Global Constraints

- リポジトリは public。社内サイトの URL、page-id、社内ページの内容を、コード・テスト・ドキュメント・コミットメッセージ・PR 本文に入れない。
- テストルート（書き込みが許可されたページ）の page-id は環境変数 `CONFLUENCE_E2E_ROOT` で渡す。書き込みはテストルートとその子孫だけに行う。それ以外のページは読み取りだけ。
- 実在のページから作るフィクスチャは `testdata/private/`（`.gitignore` 対象）に置く。
- 依存の向き: `internal/domain`・`internal/cql`・`internal/storage`・`internal/convert/...`・`internal/usecase` は、cobra・`net/http`・`os`（ファイル・環境変数）を import しない。`internal/adapter/...` は `internal/usecase` を import しない。
- インターフェースは使う側のパッケージに、使う側が呼ぶメソッドだけで定義する。テストで置き換えない箇所と実装が 1 つしかない箇所には作らない。
- パッケージ変数でのフラグ保持をやめ、コマンドは関数で生成する（テストで並行に生成できるようにする）。
- コメントと docstring は日本語で、利用者の CLAUDE.md「日本語の書き方」に従う。変更の経緯は書かない。
- push は利用者が指示した範囲で行う。`git push --force` 系と push 済みブランチの rebase は行わない（gh-stack の手順を除く）。
- コミットメッセージの末尾に次の 2 行を付ける。
  `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`
  `Claude-Session: https://claude.ai/code/session_01F2RsbXKVGhKixEXjYU6bmh`

## PR の構成

gh-stack でスタックにする（下の PR ほど上に積む）。各 PR の終わりで `go test ./...` が通ること。

| PR | ブランチ | タスク |
|----|---------|-------|
| — | main に直接 | Task 0 |
| 1 | `chore/deps-module` | Task 1 |
| 2 | `refactor/clean-architecture` | Task 2〜11 |
| 3 | `feat/storage-read` | Task 12〜16 |
| 4 | `feat/safe-write` | Task 17〜20 |
| 5 | `test/e2e` | Task 21〜22 |

## パッケージ構成

```
main.go                          cli.Execute() を呼ぶだけ
internal/domain/                 Page・Version・Space・Label・SearchHit・PageID・Error（種類つき）・ParsePageRef
internal/cql/                    CQL の組み立て・エスケープ・--since の解釈
internal/storage/                storage 形式の文字列操作（整形式チェック、見出しと節、切り貼り、コメントマーカー、損失検出、失われたテキスト）
internal/convert/tomd/           storage → Markdown
internal/convert/frommd/         Markdown → storage
internal/usecase/                ports.go（ポート定義）、search.go、space.go、page_query.go、page_tree.go、page_command.go、label.go、guard.go
internal/adapter/confluence/     HTTP クライアント（認証・リトライ・ページング・エラー変換）、pages.go、spaces.go、search.go、labels.go、cloudid.go
internal/adapter/config/         設定の読み込みと保存、トークンコマンドの実行
internal/adapter/cache/          cloudId のキャッシュファイル
internal/cli/                    root.go（依存の組み立て）、search.go、page_*.go、label.go、config.go
internal/cli/output/             table・json・エラー JSON の出力
e2e/                             ビルドタグ e2e の E2E テスト
testdata/                        合成フィクスチャ（コミットする）
testdata/private/                実在ページのフィクスチャ（コミットしない）
```

## Review Focus

- 本文が空のページ（テストルートは空）に対する `--outline`・`--section`・`--append`・`--prepend`・整形式チェック: 空の outline、`--section` は「見出しがない」エラー、append/prepend は追加分だけの本文になること。Task 13 と Task 19 にテストを入れる。
- code マクロの CDATA の中にある `<h2>` などの文字列: 見出しとして扱わないこと。Task 13 にテストを入れる。
- 見出しの照合で、見出しの中のインライン要素（`<strong>`）、前後の空白、`&nbsp;`: 正規化したテキスト（タグ除去、`&nbsp;` を空白に、空白の連続を 1 つに、前後の空白を除去）で比較すること。Task 13 にテストを入れる。
- 検索語に `"`・`'`・`\`・日本語・絵文字が入る: CQL が壊れないこと。Task 3 にテストを入れる。
- 版番号を指定しない更新で、取得と更新の間に別の更新が入る: Confluence が 409 を返し、版の競合として案内されること。Task 19 にテストを入れる。

---

### Task 0: `feature/scoped-api-token` の main へのマージと API の事前確認

**Files:** なし（git 操作と読み取りの確認だけ）

- [ ] **Step 1: remote を取得し、main が `origin/main` と一致していることを確認する**

Run: `git fetch origin && git status -sb && git rev-parse main origin/main`
Expected: 2 つのハッシュが一致する。

- [ ] **Step 2: fast-forward でマージして push する**

Run: `git checkout main && git merge --ff-only origin/feature/scoped-api-token && go test ./... && git push origin main`
Expected: テストが PASS し、push が fast-forward で成功する。

- [ ] **Step 3: 新しく使う API を読み取りだけで確認する**

設定ファイル `~/.config/confluence-cli/config.yaml` の認証情報で、次の 2 つの GET を `python3`（標準ライブラリの `urllib`）から実行する。送信先は `https://api.atlassian.com/ex/confluence/<cloudId>/wiki`（cloudId は `/_edge/tenant_info` で取得）。トークンをコマンドライン引数に置かず、スクリプトの中でファイルから読む。

- `GET /rest/api/search?cql=siteSearch%20~%20%22test%22&limit=1`
- `GET /rest/api/content/<CONFLUENCE_E2E_ROOT>/label`

Expected: どちらも 200。401・403 なら作業を止め、レスポンス本文を添えて利用者に報告する。

### Task 1: 依存・Go・モジュールパスの更新（PR 1）

**Files:**
- Modify: `go.mod`、`go.sum`、すべての `*.go` の import、`README.md`、`Makefile`
- Modify: `.gitignore`（既存。`testdata/private/` と `/confluence-cli` を追加）
- Add: `docs/superpowers/specs/2026-11-10-confluence-cli-overhaul-design.md`、本計画

- [ ] **Step 1: ブランチを作る**

Run: `gh stack init`（gh-stack スキルの手順に従い、main の上に `chore/deps-module` を作る）

- [ ] **Step 2: モジュールパスを変える**

Run: `go mod edit -module github.com/genshibangou16/confluence-cli && grep -rl 'github.com/ryo-imai-bit/confluence-cli' --include='*.go' . | xargs sed -i '' 's#github.com/ryo-imai-bit/confluence-cli#github.com/genshibangou16/confluence-cli#g'`

- [ ] **Step 3: Go と依存を上げる**

Run: `go mod edit -go=1.27 && go get -u ./... && go mod tidy`
Expected: `go.mod` の cobra が v1.10.2 以上、pflag が v1.1.0 以上。

- [ ] **Step 4: README と Makefile を直す**

README のインストール手順・Releases の URL を `genshibangou16` に変える。Makefile の `BINARY_NAME` を `confluence-cli` にし、`install` ターゲットを `go install .` にする。

- [ ] **Step 5: ビルドとテスト**

Run: `go build ./... && go vet ./... && go test ./...`
Expected: すべて成功。

- [ ] **Step 6: コミットして PR を作る**

```bash
git add -A && git commit -m "chore: モジュールパスを変更し、依存と Go を最新にする"
```
`gh stack submit` で PR を作る。PR 本文に、更新した依存の版を書く。

### Task 2: domain パッケージ

**Files:**
- Create: `internal/domain/page.go`、`internal/domain/errors.go`、`internal/domain/ref.go`
- Test: `internal/domain/ref_test.go`、`internal/domain/errors_test.go`

**Interfaces:**
- Produces:
  - `type PageID string`
  - `type Version struct { Number int; Message string; CreatedAt time.Time; AuthorID string; MinorEdit bool }`
  - `type Page struct { ID PageID; Title, Status, SpaceID, ParentID, WebURL string; Version Version; Body string }`（Body は storage 形式。取得しなかった場合は空）
  - `type PageSummary struct { ID PageID; Title, Status, SpaceID string }`
  - `type Ancestor struct { ID PageID; Type string; Title string }`（Type は `page`・`folder` など。祖先にはフォルダーも含まれる）
  - `type Space struct { ID, Key, Name string }`
  - `type Label struct { ID, Name, Prefix string }`
  - `type SearchHit struct { ID PageID; Title, Type, SpaceKey, Excerpt, URL string; LastModified time.Time }`
  - `type ErrorKind string` と定数 `ErrAuth`・`ErrForbidden`・`ErrNotFound`・`ErrConflict`・`ErrValidation`・`ErrRateLimit`・`ErrReadOnly`・`ErrVersionMismatch`・`ErrLossy`・`ErrCommentLoss`・`ErrAPI`（JSON での値は `auth`・`forbidden`・`not_found`・`conflict`・`validation`・`rate_limit`・`read_only`・`version_mismatch`・`lossy`・`comment_loss`・`api`）
  - `type Error struct { Kind ErrorKind; Status int; Message string; Hint string; Err error }`（`Error()` と `Unwrap()` を持つ）
  - `func KindOf(err error) ErrorKind`（`errors.As` で取り出し、`*Error` でなければ `ErrAPI`）
  - `func ParsePageRef(s string) (PageID, error)`

- [ ] **Step 1: 失敗するテストを書く**

```go
func TestParsePageRef(t *testing.T) {
	cases := map[string]PageID{
		"12345": "12345",
		"https://example.atlassian.net/wiki/spaces/DEV/pages/12345/Title": "12345",
		"https://example.atlassian.net/wiki/spaces/~abc/pages/12345":      "12345",
		"https://example.atlassian.net/wiki/pages/viewpage.action?pageId=12345": "12345",
		"https://example.atlassian.net/wiki/x/0gKWSQ": "1234567890",
		"https://example.atlassian.net/wiki/x/sWjeOg": "987654321",
	}
	// 各 case で ParsePageRef が期待値を返す
}
func TestParsePageRefRejects(t *testing.T) {
	// "", "abc", "https://example.atlassian.net/wiki/spaces/DEV" はエラー（Kind == ErrValidation）
}
func TestKindOf(t *testing.T) {
	// fmt.Errorf("wrap: %w", &Error{Kind: ErrNotFound}) → ErrNotFound、errors.New("x") → ErrAPI
}
```

- [ ] **Step 2: テストが失敗することを確認する**

Run: `go test ./internal/domain/...`
Expected: FAIL（未定義）

- [ ] **Step 3: 実装する**

tiny link は `<code>` に `=` を補って `base64.URLEncoding` でデコードし、4 バイトに満たなければ 0 で埋めて `binary.LittleEndian.Uint32` で読む。

- [ ] **Step 4: テストが通ることを確認する**

Run: `go test ./internal/domain/...`
Expected: PASS

- [ ] **Step 5: コミット**（`gh stack add refactor/clean-architecture` でブランチを積んでから）

### Task 3: cql パッケージ

**Files:**
- Create: `internal/cql/cql.go`
- Test: `internal/cql/cql_test.go`

**Interfaces:**
- Produces:
  - `func Quote(s string) string`（`\` と `"` をエスケープして `"` で囲む）
  - `type Mode int` と定数 `ModeSite`・`ModeTitle`・`ModeText`
  - `type Sort int` と定数 `SortRelevance`・`SortModified`
  - `type Filter struct { Mode Mode; Query string; RawCQL string; SpaceKey string; Labels []string; Type string; Since string; Author string; Sort Sort }`
  - `func Build(f Filter) (string, error)`

- [ ] **Step 1: 失敗するテストを書く**

```go
func TestBuild(t *testing.T) {
	cases := []struct{ f Filter; want string }{
		{Filter{Mode: ModeSite, Query: "議事録", Type: "page"}, `siteSearch ~ "議事録" AND type = "page"`},
		{Filter{Mode: ModeTitle, Query: `a"b\c`, Type: "page"}, `title ~ "a\"b\\c" AND type = "page"`},
		{Filter{Mode: ModeText, Query: "x", SpaceKey: "DEV", Type: "page"}, `text ~ "x" AND space = "DEV" AND type = "page"`},
		{Filter{Mode: ModeSite, Query: "x", Labels: []string{"a", "b"}, Type: "page"}, `siteSearch ~ "x" AND label = "a" AND label = "b" AND type = "page"`},
		{Filter{Mode: ModeSite, Query: "x", Since: "30d", Type: "page"}, `siteSearch ~ "x" AND type = "page" AND lastmodified >= now("-30d")`},
		{Filter{Mode: ModeSite, Query: "x", Since: "2026-01-01", Type: "page"}, `siteSearch ~ "x" AND type = "page" AND lastmodified >= "2026-01-01"`},
		{Filter{Mode: ModeSite, Query: "x", Author: "me", Type: "page"}, `siteSearch ~ "x" AND type = "page" AND creator = currentUser()`},
		{Filter{RawCQL: "label = foo", SpaceKey: "DEV"}, `(label = foo) AND space = "DEV"`},
		{Filter{Mode: ModeSite, Query: "x", Type: "page", Sort: SortModified}, `siteSearch ~ "x" AND type = "page" ORDER BY lastmodified DESC`},
		{Filter{Mode: ModeSite, Query: "😀 'q'", Type: "page"}, `siteSearch ~ "😀 'q'" AND type = "page"`},
	}
}
func TestBuildRejects(t *testing.T) {
	// Since が "30x"・"yesterday" → エラー。Query も RawCQL も空で他の条件もない → エラー。
}
```

`--since` は `^\d+[hdwMy]$` と `YYYY-MM-DD` を受け付ける。条件の並びは「検索語 → space → label → type → since → author」で固定する。RawCQL に `ORDER BY` が含まれ、Sort も指定された場合はエラーにする。

- [ ] **Step 2〜4: 失敗の確認 → 実装 → 成功の確認**

Run: `go test ./internal/cql/...`

- [ ] **Step 5: コミット**

### Task 4: 設定の adapter

**Files:**
- Create: `internal/adapter/config/config.go`、`internal/adapter/config/token.go`
- Test: `internal/adapter/config/config_test.go`
- Delete（Task 11 で）: `internal/client/client.go` の設定関連

**Interfaces:**
- Produces:
  - `type Settings struct { BaseURL, Email, APIToken string; ReadOnly bool; WritableSpaces []string }`
  - `type Sources struct { Env func(string) string; Cwd string; UserConfigPath string; RunCommand func(name string, args ...string) ([]byte, error) }`
  - `func Load(src Sources) (Settings, []string, error)`（2 番目の戻り値は標準エラーに出す警告）
  - `func DefaultSources() Sources`（`os.Getenv`、`os.Getwd`、XDG に沿ったユーザー設定のパス、`exec.Command(...).Output()`）
  - `func SaveUser(path string, s FileConfig) error`（パーミッション 0600、ディレクトリは 0700）
  - `type FileConfig struct { BaseURL string \`yaml:"base_url"\`; Email string \`yaml:"email"\`; APIToken string \`yaml:"api_token,omitempty"\`; APITokenCommand []string \`yaml:"api_token_command,omitempty"\`; ReadOnly bool \`yaml:"read_only,omitempty"\`; WritableSpaces []string \`yaml:"writable_spaces,omitempty"\` }`
  - `func FindLocal(cwd string) string`、`const LocalConfigName = ".confluence-cli.yaml"`

`api_token_command` はシェルを介さずに実行するため、YAML の文字列配列（`["security", "find-generic-password", "-s", "confluence-cli", "-w"]`）で受け取る。

- [ ] **Step 1: 失敗するテストを書く**（`t.TempDir()` に設定ファイルを書いて `Load` を呼ぶ）

```go
func TestLoad_LocalConfigOnlyBaseURL(t *testing.T)        // ローカルの email/api_token/api_token_command は無視され、警告が 1 件以上返る
func TestLoad_LocalBaseURLMustBeAtlassianNet(t *testing.T) // ローカルの base_url が https://evil.example.com/wiki → 採用されず、ユーザー設定の値が使われる。ユーザー設定にもなければエラー
func TestLoad_RejectsHTTP(t *testing.T)                    // どの設定元でも http:// はエラー
func TestLoad_UserConfigAnyHTTPSHost(t *testing.T)         // ユーザー設定の https://confluence.example.com/wiki は採用される
func TestLoad_Precedence(t *testing.T)                     // env > user > local
func TestLoad_TokenCommand(t *testing.T)                   // RunCommand の出力 "tok\nextra\n" → APIToken == "tok"。api_token と両方あれば command を優先
func TestLoad_TokenCommandFails(t *testing.T)              // RunCommand がエラー、または出力の 1 行目が空 → エラーにコマンド名を含む
func TestLoad_PolicyCannotBeLoosenedByLocal(t *testing.T)  // user read_only=false, local true → true。user writable [A,B], local [B,C] → [B]。user 未指定, local [C] → [C]
func TestLoad_EnvReadOnly(t *testing.T)                    // CONFLUENCE_READ_ONLY=1 → ReadOnly true
func TestSaveUser_Permissions(t *testing.T)                // ファイル 0600
```

- [ ] **Step 2〜4: 失敗の確認 → 実装 → 成功の確認**

Run: `go test ./internal/adapter/config/...`

- [ ] **Step 5: コミット**

### Task 5: Confluence HTTP クライアントの中核

**Files:**
- Create: `internal/adapter/confluence/client.go`、`internal/adapter/confluence/retry.go`、`internal/adapter/confluence/paging.go`、`internal/adapter/confluence/errors.go`
- Test: `internal/adapter/confluence/client_test.go`

**Interfaces:**
- Consumes: `domain.Error`、`domain.ErrorKind`
- Produces:
  - `type Options struct { Root string; Email, Token string; HTTP *http.Client; Sleep func(time.Duration); MaxAttempts int }`。`Root` はサイトの root（`https://x.atlassian.net` か `https://api.atlassian.com/ex/confluence/<cloudId>`）で、`/wiki` を含まない。
  - `func New(o Options) *Client`
  - `type endpoint struct { method, path string; scopes []string }`（パッケージ内）
  - `func (c *Client) do(ctx context.Context, ep endpoint, q url.Values, in, out any) (http.Header, error)`（パッケージ内。パスは常に `/wiki/...` から書く）
  - `func collectV2[T any](ctx context.Context, c *Client, ep endpoint, q url.Values, limit int, all bool) ([]T, error)`（`Link` ヘッダーの `rel="next"` を追う）
  - `func collectV1[T any](...)`（`_links.next` を追う。値が `/wiki/` で始まらなければ `/wiki` を前に付ける）

リトライの値: 対象は GET・PUT・DELETE の 429 と 503。最大 4 回（初回を含む）。`Retry-After` は秒数と HTTP 日付の両方を解釈し、上限 60 秒。ないときは 1 秒・2 秒・4 秒に ±20% の jitter。

エラーの変換: 400→`ErrValidation`、401→`ErrAuth`、403→`ErrForbidden`、404→`ErrNotFound`、409→`ErrConflict`、429→`ErrRateLimit`、その他→`ErrAPI`。`Message` には Confluence の応答の `message`（なければ本文の先頭 500 バイト）を入れる。`Hint` の文言は次のとおり。
- 401: `API トークンが失効しているか、必要なスコープがありません（API トークンの有効期限は最長 1 年）。この操作に必要なスコープ: <scopes>`
- 403: `この操作の権限がありません。この操作に必要なスコープ: <scopes>`
- 409: `版の競合か、公開前の承認が必要なスペースです。page get で最新の版を確認してください。`

- [ ] **Step 1: 失敗するテストを書く**（`httptest.Server`、`Sleep` は呼ばれた時間を記録するだけの関数）

```go
func TestDo_BasicAuthAndHeaders(t *testing.T)     // Authorization が Basic base64(email:token)、Accept が application/json、本文がある時だけ Content-Type
func TestDo_ErrorMapping(t *testing.T)            // 各ステータスで Kind・Status・Hint（scopes が含まれる）を確認
func TestDo_RetryAfterSeconds(t *testing.T)       // 429 + Retry-After: 2 → 2 回目で成功、Sleep(2s) が 1 回
func TestDo_RetryBackoffWithoutHeader(t *testing.T) // 503 が 3 回 → 4 回目で成功、Sleep が 3 回で 0.8〜1.2s, 1.6〜2.4s, 3.2〜4.8s
func TestDo_NoRetryForPOST(t *testing.T)          // POST の 429 → リトライせず ErrRateLimit
func TestDo_GivesUpAfterMaxAttempts(t *testing.T) // 429 が続く → 4 回で ErrRateLimit
func TestCollectV2_FollowsLink(t *testing.T)      // 2 ページ（各 2 件）、limit=3 → 3 件、2 回目のリクエストは Link のパス
func TestCollectV2_All(t *testing.T)              // all=true → 4 件
func TestCollectV1_PrefixesWiki(t *testing.T)     // _links.next が "/rest/api/search?cursor=x" → 次のリクエストのパスが "/wiki/rest/api/search"
```

- [ ] **Step 2〜4: 失敗の確認 → 実装 → 成功の確認**

Run: `go test ./internal/adapter/confluence/...`

- [ ] **Step 5: コミット**

### Task 6: cloudId の解決とキャッシュ

**Files:**
- Create: `internal/adapter/confluence/cloudid.go`（ブランチの `internal/client/cloud_id.go` を置き換える）、`internal/adapter/cache/cache.go`
- Test: `internal/adapter/confluence/cloudid_test.go`、`internal/adapter/cache/cache_test.go`
- Delete（Task 11 で）: `internal/client/cloud_id.go`、`internal/client/cloud_id_test.go`

**Interfaces:**
- Produces:
  - `package cache`: `type File struct { Dir string }`、`func DefaultDir(env func(string) string, home string) string`（`$XDG_CACHE_HOME/confluence-cli`、未設定なら `~/.cache/confluence-cli`）、`func (f File) Load(host string) (string, bool)`、`func (f File) Store(host, cloudID string) error`（`<Dir>/cloud-id-<host>` に書く、0600）
  - `package confluence`: `type CloudIDCache interface { Load(host string) (string, bool); Store(host, cloudID string) error }`、`func ResolveRoot(ctx context.Context, h *http.Client, baseURL string, cache CloudIDCache) (root string, warn string)`

`ResolveRoot` の規則:
- `baseURL` のホストが `<sub>.atlassian.net` でなければ、`baseURL` から末尾の `/wiki` を除いた値を返す。
- キャッシュにあれば、HTTP を呼ばずに `https://api.atlassian.com/ex/confluence/<id>` を返す。
- なければ `https://<host>/_edge/tenant_info` を呼んで保存して返す。
- 取得に失敗したら、`https://<host>` と警告文（`cloudId を取得できなかったため、サイトの URL に直接送信します`）を返す。失敗は保存しない。

- [ ] **Step 1: 失敗するテストを書く**

```go
func TestResolveRoot_UsesCache(t *testing.T)       // キャッシュあり → httptest に 1 回もリクエストが来ない
func TestResolveRoot_FetchesAndStores(t *testing.T) // キャッシュなし → 1 回取得し Store される
func TestResolveRoot_FallbackOnError(t *testing.T)  // tenant_info が 500 → サイトの root と警告、Store されない
func TestResolveRoot_NonAtlassianHost(t *testing.T) // https://confluence.example.com/wiki → https://confluence.example.com
func TestFileCache_RoundTrip(t *testing.T)         // Store → Load、ファイルが 0600
```

テストでは tenant_info の URL を httptest に向けるため、`ResolveRoot` の内部で使う関数に tenant_info の URL を作る関数を渡せるようにする（パッケージ内の変数ではなく引数で）。

- [ ] **Step 2〜4: 失敗の確認 → 実装 → 成功の確認**
- [ ] **Step 5: コミット**

### Task 7: ページとスペースの adapter（v2）

**Files:**
- Create: `internal/adapter/confluence/pages.go`、`internal/adapter/confluence/spaces.go`
- Test: `internal/adapter/confluence/pages_test.go`、`internal/adapter/confluence/spaces_test.go`

**Interfaces:**
- Consumes: Task 5 の `do`・`collectV2`
- Produces（`*Client` のメソッド）:
  - `GetPage(ctx, id domain.PageID, body bool, version int) (domain.Page, error)`（`body` が true なら `body-format=storage`、`version > 0` なら `version=<n>`。`WebURL` は応答の `_links.base` + `_links.webui`）
  - `GetPagesByIDs(ctx, ids []domain.PageID) ([]domain.PageSummary, error)`（`GET /wiki/api/v2/pages?id=a,b,c`）
  - `ListPages(ctx, spaceID string, limit int, all bool) ([]domain.PageSummary, error)`（本文を取得しない）
  - `CreatePage(ctx, spaceID string, parentID domain.PageID, title, storage string) (domain.Page, error)`
  - `UpdatePage(ctx, id domain.PageID, title, storage string, version int, message string) (domain.Page, error)`（`version` は送る版番号そのもの）
  - `DeletePage(ctx, id domain.PageID) error`
  - `Children(ctx, id domain.PageID, limit int, all bool) ([]domain.PageSummary, error)`（`/direct-children`）
  - `Ancestors(ctx, id domain.PageID) ([]domain.Ancestor, error)`（ルートに近い順。Title は空のまま返す）
  - `Versions(ctx, id domain.PageID, limit int, all bool) ([]domain.Version, error)`
  - `SpaceByKey(ctx, key string) (domain.Space, error)`、`SpaceByID(ctx, id string) (domain.Space, error)`

最初に `/private/tmp/claude-501/-Users-sho-kitamura-src-github-com-genshibangou16-confluence-cli/29d3c0ad-fee5-482b-a63a-c9e1e0067d5e/scratchpad/spec/v2.json`（最新の v2 仕様）を `docs/openapi-v2.v3.json` にコピーして差し替える。各エンドポイントの `scopes` は、その仕様の `x-atlassian-oauth2-scopes` の値をそのまま使う（例: ancestors は `read:content.metadata:confluence`、direct-children は `read:hierarchical-content:confluence`、spaces は `read:space:confluence`）。

- [ ] **Step 1: 失敗するテストを書く**（httptest で、送られたメソッド・パス・クエリ・本文 JSON を記録して確認する）

```go
func TestGetPage_QueryAndMapping(t *testing.T)  // body=true,version=3 → body-format=storage&version=3。応答の body.storage.value・version・WebURL が詰められる
func TestListPages_NoBody(t *testing.T)          // クエリに body-format がない
func TestUpdatePage_RequestBody(t *testing.T)    // {"id","status":"current","title","body":{"representation":"storage","value"},"version":{"number":N,"message":M}}
func TestCreatePage_RequestBody(t *testing.T)    // parentID が空なら parentId を送らない
func TestAncestors_Order(t *testing.T)
func TestSpaceByKey_NotFound(t *testing.T)       // 結果が 0 件 → ErrNotFound
```

- [ ] **Step 2〜4: 失敗の確認 → 実装 → 成功の確認**
- [ ] **Step 5: コミット**

### Task 8: 検索とラベルの adapter

**Files:**
- Create: `internal/adapter/confluence/search.go`、`internal/adapter/confluence/labels.go`
- Test: `internal/adapter/confluence/search_test.go`、`internal/adapter/confluence/labels_test.go`

**Interfaces:**
- Produces（`*Client` のメソッド）:
  - `Search(ctx, cql string, limit int, all bool) ([]domain.SearchHit, error)`（`GET /wiki/rest/api/search`、`excerpt=highlight_unescaped`。`SpaceKey` は `content` の `_expandable.space` か `resultGlobalContainer.displayUrl` の `/spaces/<KEY>` から取る。`ID` は `content.id`。`LastModified` は `lastModified`）
  - `Labels(ctx, prefix string, limit int, all bool) ([]domain.Label, error)`（v2 `/labels`）
  - `PageLabels(ctx, id domain.PageID, limit int, all bool) ([]domain.Label, error)`（v2）
  - `PagesWithLabel(ctx, labelID, spaceID string, limit int, all bool) ([]domain.PageSummary, error)`（v2）
  - `AddLabels(ctx, id domain.PageID, names []string) error`（v1 `POST /wiki/rest/api/content/{id}/label`、本文 `[{"prefix":"global","name":...}]`）
  - `RemoveLabel(ctx, id domain.PageID, name string) error`（v1 `DELETE /wiki/rest/api/content/{id}/label?name=`）

v1 のスコープ（v1 仕様の値）: search は `search:confluence`、ラベル付与は `read:label:confluence` と `write:label:confluence`、ラベル削除は `write:label:confluence`。v2 の `/labels` は `read:label:confluence`。

- [ ] **Step 1: 失敗するテストを書く**

```go
func TestSearch_MapsFields(t *testing.T)     // 応答の例（v1 仕様の SearchResult 形）から ID・Title・SpaceKey・Excerpt・URL・LastModified
func TestSearch_EncodesCQL(t *testing.T)     // cql に日本語と " を含めても、サーバーが受け取った cql パラメータが一致する
func TestAddLabels_Body(t *testing.T)
func TestRemoveLabel_UsesQueryName(t *testing.T) // name に "/" を含むラベルでもパスでなくクエリに入る
```

- [ ] **Step 2〜4: 失敗の確認 → 実装 → 成功の確認**
- [ ] **Step 5: コミット**

### Task 9: 読み取り系のユースケース

**Files:**
- Create: `internal/usecase/ports.go`、`internal/usecase/search.go`、`internal/usecase/space.go`、`internal/usecase/page_query.go`、`internal/usecase/page_tree.go`、`internal/usecase/label.go`
- Test: 各ファイルの `_test.go`（ポートの手書きフェイクを使う。フェイクはテストファイルの中に置く）

**Interfaces:**
- Consumes: Task 2 の domain、Task 3 の `cql.Build`
- Produces:
  - `ports.go`（使う側ごとに最小のメソッド集合）:
    - `type Searcher interface { Search(ctx context.Context, cql string, limit int, all bool) ([]domain.SearchHit, error) }`
    - `type SpaceFinder interface { SpaceByKey(ctx context.Context, key string) (domain.Space, error); SpaceByID(ctx context.Context, id string) (domain.Space, error) }`
    - `type PageGetter interface { GetPage(ctx context.Context, id domain.PageID, body bool, version int) (domain.Page, error) }`
    - `type PageLister interface { ListPages(...); Children(...); Ancestors(...); GetPagesByIDs(...); Versions(...) }`（シグネチャは Task 7 と同じ）
    - `type LabelReader interface { Labels(...); PageLabels(...); PagesWithLabel(...) }`
  - `type SearchService struct`、`func NewSearchService(s Searcher) *SearchService`、`func (s *SearchService) Search(ctx, f cql.Filter, limit int, all bool) (hits []domain.SearchHit, warnings []string, err error)`
  - `type SpaceService struct`、`func NewSpaceService(f SpaceFinder) *SpaceService`、`func (s *SpaceService) ResolveID(ctx, key, id string) (string, error)`（key があれば ID に変換、両方空なら ""、両方あれば一致を確認）
  - `type PageQuery struct`、`func NewPageQuery(g PageGetter) *PageQuery`、`func (q *PageQuery) Get(ctx, id domain.PageID, r GetRequest) (PageView, error)`
    - `type Format string` と定数 `FormatMarkdown`・`FormatStorage`・`FormatJSON`
    - `type GetRequest struct { Format Format; Version int; Outline bool; Section string }`
    - `type PageView struct { Page domain.Page; Content string }`（この Task では Format が storage/json のときだけ実装する。markdown・Outline・Section と、`Outline []storage.Heading` フィールドは Task 16 で足す）
  - `type PageTree struct`、`func NewPageTree(l PageLister) *PageTree`、`List`・`Children`・`Ancestors`（Type が `page` のものだけ `GetPagesByIDs` で Title を埋める。フォルダーなどは Title を空のまま返し、表では TYPE 列に種類を出す）・`Versions` メソッド
  - `type LabelQuery struct` と `List`・`OfPage`・`Pages` メソッド

- [ ] **Step 1: 失敗するテストを書く**

```go
func TestSearch_FallsBackToTextOnValidationError(t *testing.T)
// Mode=ModeSite で 1 回目が &domain.Error{Kind: ErrValidation} → 2 回目の cql が `text ~ "..."` で始まり、warnings に 1 件
func TestSearch_NoFallbackForOtherModes(t *testing.T) // ModeTitle の 400 はそのまま返る
func TestResolveID(t *testing.T)                      // key→ID、両方空→""、key と id が食い違う→ErrValidation
func TestPageQuery_StorageContent(t *testing.T)       // Format=storage → Content は Page.Body そのもの
func TestPageTree_AncestorsWithTitles(t *testing.T)   // Ancestors が [1(folder),2(page),3(page)] → GetPagesByIDs は [2,3] で呼ばれ、結果は 1,2,3 の順で 2・3 に Title
```

- [ ] **Step 2〜4: 失敗の確認 → 実装 → 成功の確認**

Run: `go test ./internal/usecase/... && go list -deps ./internal/usecase | grep -E 'spf13/cobra|net/http$' ; test $? -eq 1`
Expected: テストが PASS し、usecase の依存に cobra と net/http がない。

- [ ] **Step 5: コミット**

### Task 10: 出力

**Files:**
- Create: `internal/cli/output/table.go`、`internal/cli/output/json.go`、`internal/cli/output/errors.go`
- Test: `internal/cli/output/output_test.go`

**Interfaces:**
- Produces:
  - `func Table(w io.Writer, header []string, rows [][]string) error`（tabwriter。区切り行の `--` は出さない）
  - `func Truncate(s string, n int) string`（rune 単位、超えたら末尾に `…`）
  - `func CleanExcerpt(s string) string`（`@@@hl@@@`・`@@@endhl@@@`・HTML タグを除き、空白をまとめる）
  - `func JSON(w io.Writer, v any) error`（インデントなし 1 行。読み手のトークンを減らすため）
  - `type ErrorJSON struct { Error struct { Kind string \`json:"kind"\`; Status int \`json:"status,omitempty"\`; Message string \`json:"message"\`; Hint string \`json:"hint,omitempty"\` } \`json:"error"\` }`
  - `func WriteError(w io.Writer, err error, asJSON bool)`

出力用の構造体（`json` タグ付き）は各コマンドのファイルに置き、ページングのリンクなどを持たせない。

- [ ] **Step 1: 失敗するテストを書く**

```go
func TestTruncate_Runes(t *testing.T)       // Truncate("あいうえお", 3) == "あいう…"
func TestCleanExcerpt(t *testing.T)         // "@@@hl@@@議事@@@endhl@@@録\n<b>x</b>" → "議事録 x"
func TestWriteError_JSON(t *testing.T)      // &domain.Error{Kind: ErrNotFound, Status: 404, Message: "m"} → {"error":{"kind":"not_found","status":404,"message":"m"}} が 1 行だけ
func TestWriteError_Text(t *testing.T)      // "Error: m\nHint: h"
```

- [ ] **Step 2〜4: 失敗の確認 → 実装 → 成功の確認**
- [ ] **Step 5: コミット**

### Task 11: CLI の組み直し（読み取り系）と旧コードの削除

**Files:**
- Create: `internal/cli/root.go`、`internal/cli/app.go`、`internal/cli/search.go`、`internal/cli/page.go`、`internal/cli/page_get.go`、`internal/cli/page_list.go`、`internal/cli/page_tree.go`、`internal/cli/label.go`、`internal/cli/config.go`
- Modify: `main.go`
- Delete: `cmd/`、`internal/api/`、`internal/client/`
- Test: `internal/cli/cli_test.go`（httptest の Confluence と、`t.TempDir()` の設定ファイルを使って、ルートコマンドを実行する）

**Interfaces:**
- Consumes: Task 4〜10 のすべて
- Produces:
  - `func Execute() int`（終了コードを返す。`main` は `os.Exit(cli.Execute())`）
  - `func NewRootCmd(deps Deps) *cobra.Command`、`type Deps struct { Sources config.Sources; CacheDir string; HTTP *http.Client; Stdout, Stderr io.Writer; Stdin io.Reader }`
  - `app.go`: `func buildApp(ctx context.Context, d Deps) (*App, error)`。`App` は `Search *usecase.SearchService`、`Spaces *usecase.SpaceService`、`Pages *usecase.PageQuery`、`Tree *usecase.PageTree`、`Labels *usecase.LabelQuery`、`Settings config.Settings` を持つ。認証情報が必要なコマンドの `RunE` の中でだけ呼ぶ（`config` と `--help` は認証情報なしで動く）。

コマンドとフラグ（この Task で動くもの）:
- `search [query]`: `--title`・`--text`（排他、どちらもなければ siteSearch）、`--cql`、`--space-key`、`--label`（複数可）、`--type`（既定 `page`）、`--since`、`--author`、`--sort relevance|modified`、`--limit`（既定 10）、`--all`、`--format table|json`。table の列は `ID  TITLE  SPACE  MODIFIED(YYYY-MM-DD)  EXCERPT(60 rune)`。`--title` のときは `SPACE ID` 列も出す（スペースキーと space-id の両方。space-id は検索結果に含まれないので、`--title` の結果だけ `GetPagesByIDs` で補う）。
- `page get <ref>`: この Task では `--format storage|json` と `--version`。`storage` は本文だけを標準出力に、`version: <n>` を標準エラーに出す。
- `page list`・`page children <ref>`・`page ancestors <ref>`・`page versions <ref>`・`label list`・`label pages <label-id>`: `--space-key`/`--space-id`（該当するもの）、`--limit`、`--all`、`--format table|json`。
- `config init`（トークン入力は `term.ReadPassword`。端末でなければ 1 行読む）、`config init-local`（`base_url` だけ書く）、`config show`（トークンは先頭 4 文字だけ表示、`api_token_command` を使っているならそのことを表示、read_only と writable_spaces も表示）、`config path`。
- ルート: `SilenceUsage: true`、`SilenceErrors: true`。エラーは `output.WriteError` で出し、`--format json` が指定されていれば JSON にする。設定の警告と cloudId の警告は標準エラーに出す。

- [ ] **Step 1: 失敗するテストを書く**

```go
func TestSearch_TableColumns(t *testing.T)        // httptest の検索応答 → 表の列と、cql パラメータが siteSearch で始まる
func TestSearch_TitleShowsSpaceKeyAndID(t *testing.T)
func TestPageGet_AcceptsTinyLink(t *testing.T)    // /wiki/x/0gKWSQ → GET /wiki/api/v2/pages/1234567890
func TestPageGet_StorageToStdoutVersionToStderr(t *testing.T)
func TestError_NoUsageAndJSON(t *testing.T)       // 404 + --format json → stdout は空、stderr は JSON 1 行、usage を含まない、終了コード 1
func TestConfigShow_NoCredentialsNeeded(t *testing.T)
func TestPageList_NoBodyRequested(t *testing.T)
```

- [ ] **Step 2〜4: 失敗の確認 → 実装 → 成功の確認**

Run: `go test ./... && go vet ./...`

- [ ] **Step 5: 依存の向きを確認する**

Run: `for p in domain cql usecase; do go list -deps ./internal/$p | grep -E 'spf13/cobra|^net/http$|/internal/adapter|/internal/cli' && echo "NG $p"; done; go list -deps ./internal/adapter/... | grep /internal/usecase && echo NG`
Expected: `NG` が 1 行も出ない。

- [ ] **Step 6: 実機で読み取り系を確認する**

Run: `go build -o /private/tmp/claude-501/-Users-sho-kitamura-src-github-com-genshibangou16-confluence-cli/29d3c0ad-fee5-482b-a63a-c9e1e0067d5e/scratchpad/cfl . && /private/tmp/claude-501/-Users-sho-kitamura-src-github-com-genshibangou16-confluence-cli/29d3c0ad-fee5-482b-a63a-c9e1e0067d5e/scratchpad/cfl search "議事録" --limit 3 && /private/tmp/claude-501/-Users-sho-kitamura-src-github-com-genshibangou16-confluence-cli/29d3c0ad-fee5-482b-a63a-c9e1e0067d5e/scratchpad/cfl search "議事録" --title --limit 3 && /private/tmp/claude-501/-Users-sho-kitamura-src-github-com-genshibangou16-confluence-cli/29d3c0ad-fee5-482b-a63a-c9e1e0067d5e/scratchpad/cfl page get "$CONFLUENCE_E2E_ROOT" --format storage && /private/tmp/claude-501/-Users-sho-kitamura-src-github-com-genshibangou16-confluence-cli/29d3c0ad-fee5-482b-a63a-c9e1e0067d5e/scratchpad/cfl page ancestors "$CONFLUENCE_E2E_ROOT" && /private/tmp/claude-501/-Users-sho-kitamura-src-github-com-genshibangou16-confluence-cli/29d3c0ad-fee5-482b-a63a-c9e1e0067d5e/scratchpad/cfl label list --limit 3`
Expected: 全文検索の表で SPACE・EXCERPT 列が埋まる。タイトル検索で部分一致のページが出る。

- [ ] **Step 7: コミットして PR を作る**（`gh stack submit`）

### Task 12: storage 形式の整形式チェック

**Files:**
- Create: `internal/storage/xml.go`、`internal/storage/validate.go`
- Create: `testdata/storage/rich.xml`（合成フィクスチャ。設計書の E2E の節にある要素をすべて含める。インラインコメントのマーカーも含める）
- Test: `internal/storage/validate_test.go`

**Interfaces:**
- Produces:
  - `func Decoder(body string) (*xml.Decoder, int)`（2 番目の戻り値は包んだ分のバイト数。`<root xmlns:ac="http://atlassian.com/content" xmlns:ri="http://atlassian.com/resource/identifier" xmlns:at="http://atlassian.com/template">` で包み、`Strict=true`・`Entity=xml.HTMLEntity` を設定。Task 13 と Task 15 も使う）
  - `type SyntaxError struct { Line, Column int; Msg string }`
  - `func Validate(body string) error`（整形式でなければ `*SyntaxError`。行と列は元の本文での位置）

- [ ] **Step 1: 失敗するテストを書く**

```go
func TestValidate_AcceptsRichFixture(t *testing.T)   // testdata/storage/rich.xml → nil
func TestValidate_AcceptsEmpty(t *testing.T)         // "" → nil
func TestValidate_AcceptsEntities(t *testing.T)      // "<p>a&nbsp;b&mdash;c</p>" → nil
func TestValidate_ReportsPosition(t *testing.T)      // "<p>ok</p>\n<p>bad</div>" → Line 2
func TestValidate_PrivateFixtures(t *testing.T)      // testdata/private/*.xml があればすべて nil。なければ t.Skip
```

- [ ] **Step 2〜4: 失敗の確認 → 実装 → 成功の確認**
- [ ] **Step 5: 実在ページのフィクスチャを取得する**（コミットしない）

Run: `mkdir -p testdata/private && for id in <検索で選んだ 5 ページ程度の id>; do /private/tmp/claude-501/-Users-sho-kitamura-src-github-com-genshibangou16-confluence-cli/29d3c0ad-fee5-482b-a63a-c9e1e0067d5e/scratchpad/cfl page get $id --format storage > testdata/private/$id.xml; done && go test ./internal/storage/...`
選ぶページは、code マクロ・パネル・テーブル・レイアウトを含むものにする（`search --cql 'macro = code'` などで探す）。

- [ ] **Step 6: コミット**（`gh stack add feat/storage-read` でブランチを積んでから）

### Task 13: 見出し・節・切り貼り

**Files:**
- Create: `internal/storage/sections.go`
- Test: `internal/storage/sections_test.go`

**Interfaces:**
- Produces:
  - `type Heading struct { Level int; Text string; Start, End int; TopLevel bool }`（`Start`・`End` は見出し要素の開始タグの先頭と終了タグの末尾のバイト位置。`Text` は正規化済み）
  - `func Headings(body string) ([]Heading, error)`（`TopLevel` は、親がルート、または `ac:layout-cell` であること）
  - `type Section struct { Heading Heading; ContentStart, ContentEnd int }`
  - `func FindSection(body, title string) (Section, error)`（見つからない→`ErrValidation` で「見出しがありません」、複数→`ErrValidation` で一致した見出しの一覧を Message に含める）
  - `func ReplaceSection(body, title, content string) (string, error)`
  - `func Append(body, add string) string`、`func Prepend(body, add string) string`

節の終わりの規則: `Heading.End` から、次のどれかが最初に現れる位置まで。
- `TopLevel` で、レベルが同じかより上の見出しの開始位置
- 同じかより上のレベルの見出しを中に含むトップレベルの要素（マクロ、`ac:layout-section`）の開始位置
- 見出しを含む `ac:layout-cell` の終了タグの開始位置
- 本文の末尾

バイト位置は `Decoder.InputOffset()` から、`Decoder` が返す包んだ分のバイト数を引いて求める。出力は常に元の文字列の部分文字列をつなげて作る。

- [ ] **Step 1: 失敗するテストを書く**

```go
func TestHeadings_IgnoresCDATA(t *testing.T)            // code マクロの CDATA に "<h2>x</h2>" → 見出しにならない
func TestHeadings_NestedNotTopLevel(t *testing.T)       // expand の rich-text-body の h2 → TopLevel false
func TestHeadings_LayoutCellIsTopLevel(t *testing.T)
func TestHeadings_NormalizedText(t *testing.T)          // "<h2> <strong>A</strong>&nbsp;B  </h2>" → "A B"
func TestFindSection_Boundaries(t *testing.T)           // h1 A, h2 B, h3 C, h2 D → "B" の範囲は C を含み D の直前まで
func TestFindSection_StopsAtMacroContainingHeading(t *testing.T)
func TestFindSection_Ambiguous(t *testing.T)            // トップレベルに同名 2 つ → エラーに両方の位置
func TestFindSection_IgnoresNestedSameName(t *testing.T) // トップレベルと expand の中に同名 → トップレベルの 1 つに決まる
func TestFindSection_EmptyBody(t *testing.T)            // "" → 見出しがないエラー
func TestReplaceSection_PreservesOutsideBytes(t *testing.T)
// rich.xml の節を置き換えた結果の [0:ContentStart] と [ContentEnd:] が元とバイト単位で一致
func TestAppendPrepend_EmptyBody(t *testing.T)          // Append("", "<p>x</p>") == "<p>x</p>"
func TestSections_PrivateFixtures(t *testing.T)         // testdata/private の各ページで Headings がエラーにならず、各トップレベル見出しの ReplaceSection(同じ内容) が元と一致
```

- [ ] **Step 2〜4: 失敗の確認 → 実装 → 成功の確認**
- [ ] **Step 5: コミット**

### Task 14: コメントマーカー・損失の検出・失われたテキスト

**Files:**
- Create: `internal/storage/inspect.go`
- Test: `internal/storage/inspect_test.go`

**Interfaces:**
- Produces:
  - `func CountCommentMarkers(fragment string) int`（`ac:inline-comment-marker` 要素の数）
  - `var ConvertibleMacros = []string{"code", "info", "tip", "note", "warning", "toc"}`（Markdown で表せるマクロ。Task 15・Task 17 はこの一覧のマクロを変換する）
  - `func LossyElements(body string) []string`（Markdown の変換規則で表せない要素の名前の一覧。重複なし、出現順。対象は、`ConvertibleMacros` にないマクロ名（`macro:<name>` の形）、`ac:layout`、`rowspan`/`colspan` のあるテーブル（`table:merged-cells`）、`ac:inline-comment-marker`（`inline-comment`））
  - `func LostText(sent, got string) []string`（両方からテキストを取り出して空白で単語に分け、`sent` にあって `got` にない語の並びを返す。属性だけの差は出ない）

- [ ] **Step 1: 失敗するテストを書く**

```go
func TestCountCommentMarkers(t *testing.T)
func TestLossyElements_Rich(t *testing.T)   // rich.xml → "macro:status" など、変換表にないものだけ
func TestLossyElements_ConvertibleOnly(t *testing.T) // code・info・toc・task-list だけ → 空
func TestLostText_IgnoresAttributeChanges(t *testing.T) // ac:macro-id が付いただけ → 空
func TestLostText_ReportsMissing(t *testing.T)          // 段落が 1 つ消えた → その語
```

- [ ] **Step 2〜4: 失敗の確認 → 実装 → 成功の確認**
- [ ] **Step 5: コミット**

### Task 15: storage → Markdown

**Files:**
- Create: `internal/convert/tomd/tomd.go`、`internal/convert/panels.go`（パネルと Alert の対応表。Task 17 と共有）
- Test: `internal/convert/tomd/tomd_test.go`、`testdata/convert/tomd/*.xml` と `*.md`（ゴールデンファイル）

**Interfaces:**
- Consumes: Task 12 の `storage.Decoder`、Task 14 の `storage.ConvertibleMacros`
- Produces:
  - `package convert`: `var PanelToAlert = map[string]string{"info": "NOTE", "tip": "TIP", "note": "WARNING", "warning": "CAUTION"}`、`var AlertToPanel = map[string]string{"NOTE": "info", "IMPORTANT": "info", "TIP": "tip", "WARNING": "note", "CAUTION": "warning"}`
  - `package tomd`: `func Convert(storage string) (string, error)`

変換規則は設計書の「トークン効率」の節のとおり。設計書にない要素の扱いは次のとおり。
- 変換規則のないマクロ: `[macro: <name>]`。中に rich-text-body があれば、その中身は変換して続ける（expand の中の文章を失わない）。
- メンション: `@user(<accountId>)`。ページリンク: `[<タイトル>](page:<スペースキー>/<タイトル>)`。添付と画像: `![<alt>](attachment:<filename>)`。絵文字: `:<name>:`。
- 結合セルのあるテーブル: HTML のテーブルとして残さず、セルの値を左上のセルに置いた Markdown のテーブルにし、表の直前に `[table: merged cells]` を出す。

- [ ] **Step 1: 失敗するテストを書く**（ゴールデンファイルの比較。`-update` フラグでゴールデンファイルを書き直せるようにする）

```go
func TestConvert_Golden(t *testing.T)          // testdata/convert/tomd/{headings,lists,code,panels,tasks,table,macros,mentions}.xml
func TestConvert_PanelAlert(t *testing.T)      // warning マクロ（タイトル "T"）→ "> [!CAUTION]\n> **T**\n> 本文"
func TestConvert_CodeKeepsCDATA(t *testing.T)  // CDATA の "<a> & ]]" がそのまま、言語が ```go
func TestConvert_PrivateFixtures(t *testing.T) // testdata/private の各ページでエラーにならず、出力が storage の 60% 以下のバイト数
```

- [ ] **Step 2〜4: 失敗の確認 → 実装 → 成功の確認**
- [ ] **Step 5: コミット**

### Task 16: `page get` の markdown・outline・section

**Files:**
- Modify: `internal/usecase/page_query.go`、`internal/cli/page_get.go`
- Test: `internal/usecase/page_query_test.go`、`internal/cli/cli_test.go`

**Interfaces:**
- Consumes: Task 13 の `Headings`・`FindSection`、Task 15 の `tomd.Convert`
- Produces: `PageQuery.Get` が `FormatMarkdown`（既定）・`Outline`・`Section` を扱う。

出力（CLI）:
- markdown: 1〜4 行目に `id: <id>`・`title: <title>`・`version: <n>`・`url: <url>`、5 行目に `---`、続けて本文の Markdown。
- `--outline`: ヘッダー 4 行の後に、トップレベルの見出しをレベルに応じてインデントした一覧（`  - B`）。
- `--section`: 節の中身だけを `--format` の形式で出す（json なら `content` フィールド）。
- json: `{"id","title","status","spaceId","version","url","content"}`（content は `--format json` で取得した本文。markdown を既定にするため `--content-format markdown|storage` を json のときだけ受け付け、既定は markdown）。

- [ ] **Step 1: 失敗するテストを書く**

```go
func TestPageQuery_MarkdownDefault(t *testing.T)
func TestPageQuery_Outline(t *testing.T)            // 空の本文 → 空の Outline、エラーなし
func TestPageQuery_SectionMarkdown(t *testing.T)
func TestPageQuery_SectionNotFound(t *testing.T)    // ErrValidation
func TestPageGet_MarkdownHeader(t *testing.T)       // CLI 出力の先頭 5 行
```

- [ ] **Step 2〜4: 失敗の確認 → 実装 → 成功の確認**
- [ ] **Step 5: 実機で確認する**

Run: `go build -o /private/tmp/claude-501/-Users-sho-kitamura-src-github-com-genshibangou16-confluence-cli/29d3c0ad-fee5-482b-a63a-c9e1e0067d5e/scratchpad/cfl . && /private/tmp/claude-501/-Users-sho-kitamura-src-github-com-genshibangou16-confluence-cli/29d3c0ad-fee5-482b-a63a-c9e1e0067d5e/scratchpad/cfl page get <テストルート以外の、見出しの多いページ> | head -40 && /private/tmp/claude-501/-Users-sho-kitamura-src-github-com-genshibangou16-confluence-cli/29d3c0ad-fee5-482b-a63a-c9e1e0067d5e/scratchpad/cfl page get <同じページ> --outline`
Expected: HTML タグが出ず、Markdown と見出しの一覧が出る。

- [ ] **Step 6: コミットして PR を作る**（`gh stack submit`）

### Task 17: Markdown → storage

**Files:**
- Create: `internal/convert/frommd/frommd.go`、`internal/convert/frommd/render.go`、`internal/convert/languages.go`
- Test: `internal/convert/frommd/frommd_test.go`、`testdata/convert/frommd/*.md` と `*.xml`

**Interfaces:**
- Consumes: Task 15 の `convert.AlertToPanel`
- Produces:
  - `package convert`: `func NormalizeLanguage(s string) string`（別名表は設計書のとおり。Confluence の言語名一覧にないものは `text`。空なら空）
  - `package frommd`: `func Convert(md string) (string, error)`

goldmark に GFM の拡張（テーブル・取り消し線・タスクリスト）を入れ、storage 形式を書き出す独自のレンダラーを作る。生の HTML のノード（`ast.HTMLBlock`・`ast.RawHTML`）は、テキストとしてエスケープして出す。Alert は、blockquote の最初の段落が `[!TYPE]` だけの行で始まるときだけパネルにする。CDATA の中に `]]>` があれば `]]]]><![CDATA[>` に分割する。

- [ ] **Step 1: 失敗するテストを書く**

```go
func TestConvert_ReferenceExamples(t *testing.T)
// スキルの変換リファレンスの各例（見出し・装飾・リンク・画像・リスト・テーブル・水平線・引用・コード・タスク・TOC）を
// testdata/convert/frommd にゴールデンファイルとして置く。Alert の WARNING→note、CAUTION→warning は設計書の対応表に合わせる。
func TestConvert_AlertMapping(t *testing.T)     // NOTE→info, IMPORTANT→info, TIP→tip, WARNING→note, CAUTION→warning。普通の引用は blockquote
func TestConvert_CodeLanguageAlias(t *testing.T) // ```sh → bash, ```golang → go, ```foo → text
func TestConvert_CDATASplit(t *testing.T)       // コードに "]]>" → 出力が Validate を通り、tomd で戻すと元のコード
func TestConvert_EscapesRawHTML(t *testing.T)   // "<script>x</script>" → "&lt;script&gt;"
func TestConvert_OutputIsWellFormed(t *testing.T) // 全ゴールデン入力の出力が storage.Validate を通る
func TestConvert_TaskIDsSequential(t *testing.T)
```

- [ ] **Step 2〜4: 失敗の確認 → 実装 → 成功の確認**
- [ ] **Step 5: コミット**（`gh stack add feat/safe-write` でブランチを積んでから）

### Task 18: 書き込みのガードとページ作成

**Files:**
- Create: `internal/usecase/guard.go`、`internal/usecase/page_command.go`
- Modify: `internal/usecase/ports.go`
- Test: `internal/usecase/guard_test.go`、`internal/usecase/page_command_test.go`

**Interfaces:**
- Consumes: Task 12 の `Validate`、Task 14 の `LostText`、Task 17 の `frommd.Convert`
- Produces:
  - `ports.go` に追加: `type PageWriter interface { CreatePage(...); UpdatePage(...); DeletePage(...) }`（シグネチャは Task 7 と同じ）
  - `type WritePolicy struct { ReadOnly bool; WritableSpaces []string }`
  - `type WriteGuard struct`、`func NewWriteGuard(p WritePolicy, spaces SpaceFinder) *WriteGuard`、`func (g *WriteGuard) Check(ctx, spaceID string) error`（ReadOnly→`ErrReadOnly`。WritableSpaces が空でなく、spaceID のキーが含まれない→`ErrReadOnly`、Message にスペースキー）
  - `type InputFormat string` と定数 `InputStorage`・`InputMarkdown`
  - `type PageCommand struct`、`func NewPageCommand(g PageGetter, w PageWriter, guard *WriteGuard) *PageCommand`
  - `type CreateRequest struct { SpaceID string; ParentID domain.PageID; Title, Body string; Input InputFormat }`
  - `func (c *PageCommand) Create(ctx, r CreateRequest) (WriteResult, error)`
  - `type WriteResult struct { Page domain.Page; Lost []string; Diff string; DryRun bool; Warnings []string }`

`Create` の順序: 入力の変換 → `Validate`（失敗なら API を呼ばない）→ 親が指定されていれば親の spaceID を使って `guard.Check` → `CreatePage` → `GetPage(body=true)` で読み戻し → `LostText` を `Lost` に入れる。`Lost` が空でなければ、`WriteResult` とともに `&domain.Error{Kind: ErrAPI, Message: "書き込み後の本文から次のテキストが失われました: ..."}` を返す。

- [ ] **Step 1: 失敗するテストを書く**

```go
func TestGuard_ReadOnly(t *testing.T)
func TestGuard_WritableSpaces(t *testing.T)            // 許可リスト外 → ErrReadOnly、PageWriter が呼ばれない
func TestCreate_InvalidStorageNoAPICall(t *testing.T)  // "<p>x" → *storage.SyntaxError、CreatePage が呼ばれない
func TestCreate_MarkdownConverted(t *testing.T)        // CreatePage に渡る本文が frommd.Convert の結果
func TestCreate_VerifyReportsLostText(t *testing.T)    // 読み戻しの本文から段落が消えている → Lost と エラー
```

- [ ] **Step 2〜4: 失敗の確認 → 実装 → 成功の確認**
- [ ] **Step 5: コミット**

### Task 19: ページ更新

**Files:**
- Modify: `internal/usecase/page_command.go`
- Test: `internal/usecase/page_update_test.go`

**Interfaces:**
- Consumes: Task 13 の `ReplaceSection`・`FindSection`・`Append`・`Prepend`、Task 14 の `CountCommentMarkers`・`LossyElements`、go-udiff
- Produces:
  - `type UpdateMode int` と定数 `ModeReplace`・`ModeAppend`・`ModePrepend`・`ModeSection`
  - `type UpdateRequest struct { ID domain.PageID; Title *string; Body *string; Input InputFormat; Mode UpdateMode; Section string; ExpectedVersion int; Message string; DryRun, AllowLossy, AllowDropComments bool }`（`Title`・`Body` が nil なら現在の値を残す）
  - `func (c *PageCommand) Update(ctx, r UpdateRequest) (WriteResult, error)`

`Update` の順序:
1. `GetPage(body=true)` で現在のページを取得する。
2. `ExpectedVersion > 0` で現在の版番号と違えば `ErrVersionMismatch`（Message に両方の版番号）。
3. `guard.Check(page.SpaceID)`。
4. `Body` があれば入力を変換し、`Validate` する。
5. モードに応じて新しい本文を作る。`Body` が nil なら現在の本文そのもの。
6. 置き換える範囲（Replace は現在の本文全体、Section は節の中身）の `CountCommentMarkers` が 1 以上で、`AllowDropComments` がなければ、`ErrCommentLoss`。ただし DryRun のときは、エラーにせず警告を `WriteResult.Warnings` に入れる。
7. `Input == InputMarkdown` かつ `Mode == ModeReplace` で、`LossyElements(現在の本文)` が空でなく `AllowLossy` がなければ、`ErrLossy`（Message に要素の一覧）。
8. `DryRun` なら unified diff を `Diff` に入れて返す（API で書き込まない）。
9. `UpdatePage(version=現在+1)`。409 は Task 5 の Hint のまま返す。
10. 読み戻して `LostText` を確認する（Create と同じ）。

- [ ] **Step 1: 失敗するテストを書く**

```go
func TestUpdate_TitleOnlyKeepsBodyBytes(t *testing.T)    // UpdatePage に渡る本文が現在の本文とバイト単位で一致
func TestUpdate_BodyOnlyKeepsTitle(t *testing.T)
func TestUpdate_ExpectedVersionMismatch(t *testing.T)    // 現在 5、期待 4 → ErrVersionMismatch、UpdatePage が呼ばれない
func TestUpdate_SendsMessageAndNextVersion(t *testing.T)
func TestUpdate_AppendPrependSection(t *testing.T)       // 各モードで UpdatePage に渡る本文
func TestUpdate_SectionWithCommentMarkerRefused(t *testing.T)
func TestUpdate_SectionWithCommentMarkerAllowed(t *testing.T)
func TestUpdate_DryRunWarnsOnComments(t *testing.T)      // DryRun ではエラーにならず Warnings に 1 件
func TestUpdate_MarkdownReplaceLossyRefused(t *testing.T) // 現在の本文に status マクロ → ErrLossy、Message に "macro:status"
func TestUpdate_MarkdownAppendNotLossyChecked(t *testing.T)
func TestUpdate_DryRunNoWrite(t *testing.T)              // Diff が "-" と "+" の行を含み、UpdatePage が呼ばれない
func TestUpdate_ConflictPassedThrough(t *testing.T)      // UpdatePage が ErrConflict → Kind が ErrConflict のまま
func TestUpdate_EmptyBodyAppend(t *testing.T)            // 現在の本文 "" に Append → 追加分だけ
```

- [ ] **Step 2〜4: 失敗の確認 → 実装 → 成功の確認**
- [ ] **Step 5: コミット**

### Task 20: 削除・ラベル書き込みと、書き込み系の CLI

**Files:**
- Modify: `internal/usecase/page_command.go`、`internal/usecase/label.go`、`internal/usecase/ports.go`
- Create: `internal/cli/page_write.go`、`internal/cli/label_write.go`
- Modify: `internal/cli/app.go`
- Test: `internal/usecase/label_test.go`、`internal/cli/cli_write_test.go`

**Interfaces:**
- Produces:
  - `func (c *PageCommand) Delete(ctx, id domain.PageID) error`（取得 → `guard.Check` → 削除）
  - `ports.go` に追加: `type LabelWriter interface { AddLabels(...); RemoveLabel(...) }`
  - `type LabelCommand struct`、`func NewLabelCommand(g PageGetter, w LabelWriter, guard *WriteGuard) *LabelCommand`、`Add(ctx, id, names []string) error`、`Remove(ctx, id, name string) error`

CLI:
- `page create`: `--space-key`/`--space-id`（`--parent` があれば省略可）、`--parent <ref>`（旧 `--parent-id` も別名として残す）、`--title`（必須）、`--body`・`--body-file <path|->`（排他）、`--input-format storage|markdown`、`--format text|json`。
- `page update <ref>`: `--title`、`--body`・`--body-file`、`--input-format`、`--append`・`--prepend`・`--section <見出し>`（排他）、`--expect-version <n>`、`--message`、`--dry-run`、`--allow-lossy`、`--allow-drop-comments`、`--format text|json`。
- `page delete <ref>`、`label add <ref> <name>...`、`label remove <ref> <name>`。
- text 出力（create・update）: `id: <id>`・`title: <title>`・`version: <n>`・`url: <url>` の 4 行。dry-run は差分と警告だけ。

- [ ] **Step 1: 失敗するテストを書く**

```go
func TestPageUpdate_BodyFileStdin(t *testing.T)     // --body-file - で Stdin の内容が送られる
func TestPageUpdate_FlagsExclusive(t *testing.T)    // --append と --section → エラー、API を呼ばない
func TestPageCreate_TextOutput4Lines(t *testing.T)
func TestReadOnly_AllWriteCommandsRefused(t *testing.T) // CONFLUENCE_READ_ONLY=1 で create/update/delete/label add/remove が、書き込みのリクエストを 1 回も送らずに失敗し、--format json なら kind が read_only
func TestDelete_GuardChecked(t *testing.T)
```

- [ ] **Step 2〜4: 失敗の確認 → 実装 → 成功の確認**

Run: `go test ./... && go vet ./...`（Task 11 Step 5 の依存の向きの確認も再実行する）

- [ ] **Step 5: コミットして PR を作る**（`gh stack submit`）

### Task 21: E2E テスト

**Files:**
- Create: `e2e/helper_test.go`、`e2e/format_preservation_test.go`、`e2e/commands_test.go`
- Test: 同上（すべて `//go:build e2e`）

**Interfaces:**
- Produces（helper）:
  - `func setup(t *testing.T) *env`（`CONFLUENCE_E2E_ROOT` がなければ `t.Skip`。`go build` したバイナリのパス、テストルートの id、開始時のテストルートのタイトルと本文を保持。`t.Cleanup` で、作った子ページの削除とテストルートの復元を行う）
  - `func (e *env) run(args ...string) (stdout, stderr string, code int)`
  - `func (e *env) write(t *testing.T, target string, args ...string) (stdout, stderr string, code int)`（`target` のページの `ancestors` にテストルートがあるか、`target` がテストルートであることを、テスト用の読み取りコマンドで確認してから実行する。確認できなければ `t.Fatal`。create は `--parent` の値を `target` にする）
  - `func (e *env) storage(t *testing.T, id string) string`

テストの内容は設計書の「E2E テスト」と「既存ページの編集でフォーマットを壊さないことの確認」の節のとおり。フィクスチャは Task 12 の `testdata/storage/rich.xml` を使う。

- [ ] **Step 1: テストを書く**

```go
func TestE2E_Guard(t *testing.T)                     // テストルート外の id を write に渡すと Fatal になること（サブテストで確認し、コマンドは実行されない）
func TestE2E_TitleOnlyUpdateKeepsBody(t *testing.T)
func TestE2E_MessageOnlyUpdateKeepsBody(t *testing.T)
func TestE2E_StorageRoundTripIdentical(t *testing.T) // page get --format storage → page update --body-file - → 本文がベースラインと一致
func TestE2E_AppendPrependKeepBaseline(t *testing.T)
func TestE2E_SectionReplaceKeepsOutside(t *testing.T) // expand の中の同名見出しは不変
func TestE2E_MarkdownPartialUpdate(t *testing.T)
func TestE2E_CommentMarkerRefused(t *testing.T)
func TestE2E_LossyMarkdownReplaceRefused(t *testing.T)
func TestE2E_ExpectVersionConflict(t *testing.T)
func TestE2E_DryRunNoChange(t *testing.T)
func TestE2E_InvalidStorageNoAPICall(t *testing.T)   // 版番号が変わらない
func TestE2E_CreateMarkdownAndStorage(t *testing.T)
func TestE2E_TreeAndVersions(t *testing.T)           // children・ancestors・versions・get --version
func TestE2E_Labels(t *testing.T)                    // add → label pages と search --label にポーリングで出る（最大 60 秒）→ remove
func TestE2E_TinyLinkAndURL(t *testing.T)
func TestE2E_ReadOnlyAndWritableSpaces(t *testing.T) // 一時的なユーザー設定（XDG_CONFIG_HOME を t.TempDir に）で確認
func TestE2E_JSONErrorOnFailure(t *testing.T)
func TestE2E_SearchReadOnly(t *testing.T)            // siteSearch・--title・--text・--cql・各絞り込みフラグが 0 終了
```

フィクスチャのインラインコメントのマーカーは、対応するコメントがないため Confluence が保存時に削除する可能性がある。その場合は `TestE2E_CommentMarkerRefused` の対象ページを、Web UI でインラインコメントを付けたテストルートの子ページにするよう利用者に依頼する。

ベースラインとの比較が一致しない場合は、`udiff` の差分を `t.Errorf` に出す。Confluence の正規化で差が出た場合は、設計書の手順 5 のとおり差分を記録し、利用者に報告してから比較方法を決める（この Task の中で比較を緩めない）。

- [ ] **Step 2: 実行する**

Run: `CONFLUENCE_E2E_ROOT=<テストルート> go test -tags e2e -count=1 -v ./e2e/...`
Expected: すべて PASS。実行後にテストルートの子ページが 0 件で、本文とタイトルが開始時と同じ。

- [ ] **Step 3: 後片付けを確認する**

Run: `/private/tmp/claude-501/-Users-sho-kitamura-src-github-com-genshibangou16-confluence-cli/29d3c0ad-fee5-482b-a63a-c9e1e0067d5e/scratchpad/cfl page children "$CONFLUENCE_E2E_ROOT" && /private/tmp/claude-501/-Users-sho-kitamura-src-github-com-genshibangou16-confluence-cli/29d3c0ad-fee5-482b-a63a-c9e1e0067d5e/scratchpad/cfl page get "$CONFLUENCE_E2E_ROOT" --format storage`
Expected: 子ページなし。本文が開始時と同じ。

- [ ] **Step 4: コミット**（`gh stack add test/e2e` でブランチを積んでから）

### Task 22: インストールと最終確認

**Files:**
- Modify: `README.md`（コマンドとフラグ、設定項目 `api_token_command`・`read_only`・`writable_spaces`、スコープの一覧、E2E テストの実行方法）、`CLAUDE.md`（ビルドとテストのコマンド、パッケージ構成と依存の向き）

- [ ] **Step 1: ドキュメントを更新する**

README の内容は実装したコマンドの `--help` の出力と照合する。社内の URL と page-id を含めない。

- [ ] **Step 2: 全テスト**

Run: `go vet ./... && go test ./... && CONFLUENCE_E2E_ROOT=<テストルート> go test -tags e2e -count=1 ./e2e/...`
Expected: すべて PASS。

- [ ] **Step 3: バイナリを置き換える**

Run: `cp ~/go/bin/confluence-cli /tmp/confluence-cli.bak && go install . && ls -l ~/go/bin/confluence-cli && confluence-cli --help`
Expected: 新しいタイムスタンプ。`--help` に `search`・`page`・`label`・`config` が出る。

- [ ] **Step 4: 実機で読み取り系を確認する**

Run（cwd は `/tmp`）: `confluence-cli search "議事録" --limit 3 && confluence-cli search "議事録" --title --limit 3 && confluence-cli search --cql "type = page" --since 7d --limit 3 && confluence-cli page get <テストルート以外のページ> | head -20 && confluence-cli page get <同じページ> --outline && confluence-cli page children "$CONFLUENCE_E2E_ROOT" && confluence-cli page ancestors "$CONFLUENCE_E2E_ROOT" && confluence-cli page versions "$CONFLUENCE_E2E_ROOT" && confluence-cli label list --limit 3 && confluence-cli page update "$CONFLUENCE_E2E_ROOT" --body "<p>x</p>" --dry-run`
Expected: すべて終了コード 0。

- [ ] **Step 5: grep で社内情報が入っていないことを確認する**

Run: `git log -p main..HEAD | grep -n -i -E '<社名>|atlassian\.net/wiki/(spaces|x)/' ; git grep -n -i <社名>`
Expected: 出力なし（`example.atlassian.net` のテスト用 URL は除く）。

- [ ] **Step 6: コミットして PR を作る**（`gh stack submit`）
