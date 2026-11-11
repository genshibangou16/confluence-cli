# search の --title の説明と抜粋の絵文字の修正 実装計画

**Goal:** `search --title` の説明を実際の一致の仕方に合わせ、0 件のときに検索し直す方法を警告で示す。検索結果の抜粋に `\uD83D\uDDD3` のような文字列で入る絵文字を、元の文字に戻す。

**Architecture:** 説明の修正は `internal/cli/search.go`・README・設計書。0 件の警告は、検索の警告を組み立てている `internal/usecase/search.go` に足す。抜粋の文字列の復元は、API の応答の表現に依存する処理なので、検索 API の応答を読む `internal/adapter/confluence/search.go` に置く。

**Spec:** `docs/superpowers/specs/2026-11-10-confluence-cli-overhaul-design.md`（タイトル検索と検索の出力の節）

## 調べて分かったこと

### --title は文字列の部分一致にならない

`--title` は CQL の `title ~ "語"` を送る。Confluence はタイトルを語に分けて照合するため、タイトルの一部を連ねた文字列では一致しないことがある。実際の Confluence で次の結果になった（タイトルの英字と語を `ABC`・`週次MTG` のように置き換えて示す。対象は同じスペースの `20250101_ABC週次MTG` の形のページと、`ABC週次定例` のページ）。

| CQL | 結果 |
|---|---|
| `title ~ "ABC週次MTG"` | 0 件 |
| `title ~ "ABC週次MTG*"` | 0 件 |
| `title ~ "*ABC週次MTG"` | 0 件 |
| `title ~ "週次MTG"` | `20250101_ABC週次MTG` などが一致 |
| `title ~ "ABC週次"` | `ABC週次定例` だけが一致 |

ワイルドカードを付けても一致しないため、CQL の組み立てを変えて部分一致にする方法はない。`--help`・README・設計書の「部分一致」という説明を「語の一致」に直し、0 件のときに検索し直す方法を警告で示す。

### 抜粋の絵文字は API が文字列で返している

検索 API（`GET /wiki/rest/api/search`）の応答の `excerpt` に、絵文字が `\uD83D\uDDD3` という 12 文字の文字列（JSON では `"\\uD83D\\uDDD3"`）で入っている。`excerpt=highlight` と `excerpt=highlight_unescaped` のどちらでも同じ。CLI の変換ではなく、API の応答の時点でこの形になっている。

1,000 件の検索結果を調べた範囲では、次のとおりだった。

- `\uXXXX` の文字列は、すべてサロゲートペア（上位 `D800`〜`DBFF` と下位 `DC00`〜`DFFF` の組）だった。
- タイトルには `\uXXXX` の文字列がなかった。

## 方針

- **抜粋の復元の範囲:** 上位と下位のサロゲートの `\uXXXX` が隣り合う組だけを、1 文字に戻す。組になっていない `\uXXXX` と、サロゲート以外の `\uXXXX` は、本文にその文字列が書かれている可能性があるため変えない。サロゲートペアの文字列が本文に書かれていることは、ほぼないと考える。
- **復元の対象:** `excerpt` だけにする。タイトルには、調べた範囲で同じ形が現れなかった。
- **0 件の警告:** `--title` での検索（`--cql` を使わない場合）で結果が 0 件のときだけ出す。警告は既存の仕組み（テキストでは標準エラー、`--format json` では `{"warnings":[...]}`）で出る。

## Task 1: --title の説明と 0 件の警告

**Files:**
- Modify: `internal/cli/search.go`（Long と `--title` のフラグの説明）
- Modify: `internal/usecase/search.go`
- Test: `internal/usecase/search_test.go`
- Modify: `README.md`、`docs/superpowers/specs/2026-11-10-confluence-cli-overhaul-design.md`

- [ ] `TestSearch_TitleNoHitsWarns` を書く。`Mode: cql.ModeTitle` で検索 API が 0 件を返すと、warnings に 1 件入る。
- [ ] `TestSearch_TitleHitsNoWarning` を書く。`ModeTitle` で 1 件以上なら warnings は空。`RawCQL` を指定した検索と、`ModeSite`・`ModeText` で 0 件のときも warnings は空。
- [ ] 失敗を確かめる: `go test ./internal/usecase -run TestSearch_Title`
- [ ] `SearchService.Search` のタイトル検索の分岐で、0 件なら警告を返す。文言: `--title の検索は 0 件でした。--title はタイトルを語に分けて照合するため、タイトルに含まれる文字列でも一致しないことがあります。別の語にするか、--title を外して検索し直してください`
- [ ] テストが通ることを確かめる。
- [ ] `--help` の説明を「タイトルの語の一致」にする。README の例のコメントと設計書の 3 か所（82・114・250 行目）も直す。
- [ ] コミットする。

## Task 2: 抜粋のサロゲートペアの文字列を文字に戻す

**Files:**
- Modify: `internal/adapter/confluence/search.go`
- Test: `internal/adapter/confluence/search_test.go`

- [ ] `TestSearch_DecodesEscapedSurrogatePairsInExcerpt` を書く。応答の excerpt が `\\uD83D\\uDDD3 日付` のとき、`Excerpt` が `🗓 日付` になる。
- [ ] `TestDecodeEscapedSurrogatePairs` をテーブルテストで書く。
  - `\uD83D\uDDD3` → `🗓`
  - 連続した 2 組 → 2 文字
  - 上位だけ（`\uD83D` の後に文字）→ 変えない
  - 下位だけ → 変えない
  - サロゲート以外（`\u0041`）→ 変えない
  - 小文字の 16 進数（`\ud83d\udde3`）→ 文字に戻す
  - 16 進数が 4 桁に満たない → 変えない
- [ ] 失敗を確かめる: `go test ./internal/adapter/confluence -run 'Surrogate'`
- [ ] `decodeEscapedSurrogatePairs(s string) string` を実装し、`Search` で `highlightMarkers.Replace` の後の excerpt に適用する。正規表現 `\\u([dD][89abAB][0-9a-fA-F]{2})\\u([dD][c-fC-F][0-9a-fA-F]{2})` で組を探し、`utf16.DecodeRune` で 1 文字にする。
- [ ] テストが通ることを確かめる。
- [ ] コミットする。

## 確認

- `go vet ./... && go test ./...`
- 実際の Confluence（読み取りだけ）で次を確かめる。
  - `search --label <絵文字を含む抜粋のラベル>` の抜粋に絵文字が文字として出る。
  - `search "<タイトルの一部を連ねた語>" --title` で 0 件のときに警告が出る。
