//go:build e2e

package e2e

import (
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

func TestE2E_Guard(t *testing.T) {
	e := setup(t)
	// ガードに不具合があっても書き込まれないよう、拒否を確かめるコマンドは --dry-run にし、read_only の設定も付ける。
	e.extraEnv = []string{"CONFLUENCE_READ_ONLY=1"}
	defer func() { e.extraEnv = nil }()

	var as []struct {
		ID   string `json:"id"`
		Type string `json:"type"`
	}
	decode(t, e.mustRun(t, "page", "ancestors", e.root, "--format", "json"), &as)
	if len(as) == 0 {
		t.Fatal("テストルートに祖先がないため、テストルート外のページで確認できない")
	}

	refused := func(t *testing.T, target string, args ...string) {
		t.Helper()
		rec := &goexitRecorder{}
		done := make(chan struct{})
		e.executed = nil
		go func() {
			defer close(done)
			e.guardedWrite(rec.fatalf, target, "", args...)
		}()
		<-done
		if rec.msg == "" {
			t.Fatalf("対象 %s への %q が拒否されなかった", target, args)
		}
		if isWriteCommand(e.executed) {
			t.Fatalf("拒否されたのに書き込み系のコマンドを実行した: %q", e.executed)
		}
	}
	dryRun := []string{"page", "update", "--dry-run", "--body", "<p>x</p>"}
	for _, a := range as {
		t.Run("ancestor-"+a.Type, func(t *testing.T) { refused(t, a.ID, dryRun...) })
	}
	t.Run("not-a-page-id", func(t *testing.T) { refused(t, "../"+e.root, dryRun...) })
	t.Run("parent-flag", func(t *testing.T) {
		refused(t, e.root, "page", "create", "--title", "x", "--body", "<p>x</p>", "--parent", as[len(as)-1].ID)
	})
	t.Run("root-update", func(t *testing.T) { refused(t, e.root, dryRun...) })
	t.Run("read-command", func(t *testing.T) { refused(t, e.root, "page", "get") })
	t.Run("run-refuses-write", func(t *testing.T) {
		if _, _, code := e.run("page", "update", as[len(as)-1].ID, "--dry-run", "--body", "<p>x</p>"); code != -1 {
			t.Fatalf("run が書き込み系のコマンドを実行した（終了コード %d）", code)
		}
	})

	e.extraEnv = nil
	if err := e.checkWritable(e.root); err != nil {
		t.Errorf("テストルートへの書き込みが拒否された: %v", err)
	}
	child := e.create(t, e.root, "child", "<p>子</p>", "storage")
	if err := e.checkWritable(child); err != nil {
		t.Errorf("テストルートの子への書き込みが拒否された: %v", err)
	}
}

func TestE2E_ExpectVersionConflict(t *testing.T) {
	e := setup(t)
	id := e.create(t, e.root, "page", "<p>版 1</p>", "storage")
	v := e.version(t, id)
	e.mustWrite(t, id, "page", "update", "--body", "<p>版 2</p>")
	after := e.storage(t, id)
	_, errOut, code := e.write(t, id, "page", "update", "--expect-version", strconv.Itoa(v), "--body", "<p>版 3</p>", "--format", "json")
	if code == 0 {
		t.Fatal("古い版番号での更新が成功した")
	}
	if k := errorKind(t, errOut); k != "version_mismatch" {
		t.Errorf("kind = %q, want version_mismatch", k)
	}
	assertSame(t, "古い版番号での更新", after, e.storage(t, id))
	if got := e.version(t, id); got != v+1 {
		t.Errorf("版 = %d, want %d", got, v+1)
	}
}

func TestE2E_DryRunNoChange(t *testing.T) {
	e := setup(t)
	id := e.create(t, e.root, "page", "<p>元の本文</p>", "storage")
	baseline := e.storage(t, id)
	v := e.version(t, id)
	out := e.mustWrite(t, id, "page", "update", "--dry-run", "--body", "<p>dry-run の本文</p>", "--title", e.newTitle(t, "dry"))
	if !strings.Contains(out, "dry-run の本文") || !strings.Contains(out, "元の本文") {
		t.Errorf("差分に現在と新しい本文がない:\n%s", out)
	}
	assertSame(t, "--dry-run", baseline, e.storage(t, id))
	if got := e.version(t, id); got != v {
		t.Errorf("版が %d から %d に変わった", v, got)
	}
}

func TestE2E_InvalidStorageNoAPICall(t *testing.T) {
	e := setup(t)
	id := e.create(t, e.root, "page", "<p>元の本文</p>", "storage")
	baseline := e.storage(t, id)
	v := e.version(t, id)
	_, errOut, code := e.write(t, id, "page", "update", "--body", "<p>閉じていない", "--format", "json")
	if code == 0 {
		t.Fatal("整形式でない本文での更新が成功した")
	}
	if k := errorKind(t, errOut); k != "validation" {
		t.Errorf("kind = %q, want validation", k)
	}
	assertSame(t, "整形式でない本文での更新", baseline, e.storage(t, id))
	if got := e.version(t, id); got != v {
		t.Errorf("版が %d から %d に変わった", v, got)
	}

	title := e.newTitle(t, "invalid")
	out, errOut, code := e.write(t, e.root, "page", "create", "--title", title, "--body", "<p>閉じていない", "--format", "json")
	e.recordCreated(out)
	if code == 0 {
		t.Fatal("整形式でない本文での作成が成功した")
	}
	if k := errorKind(t, errOut); k != "validation" {
		t.Errorf("kind = %q, want validation", k)
	}
	if slices.ContainsFunc(e.children(t, e.root), func(c child) bool { return c.Title == title }) {
		t.Error("整形式でない本文のページが作られた")
	}
}

// recordCreated は page create の JSON 出力から ID を取り出し、後片付けの対象にする。
func (e *env) recordCreated(out string) {
	var w struct {
		ID string `json:"id"`
	}
	if json.Unmarshal([]byte(out), &w) == nil && w.ID != "" {
		e.created = append(e.created, w.ID)
	}
}

// forget は削除済みのページを後片付けの対象から外す。
func (e *env) forget(id string) {
	e.created = slices.DeleteFunc(e.created, func(c string) bool { return c == id })
}

type child struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

func (e *env) children(t *testing.T, id string) []child {
	t.Helper()
	var cs []child
	decode(t, e.mustRun(t, "page", "children", id, "--all", "--format", "json"), &cs)
	return cs
}

func hasChild(cs []child, id string) bool {
	return slices.ContainsFunc(cs, func(c child) bool { return c.ID == id })
}

func TestE2E_CreateMarkdownAndStorage(t *testing.T) {
	e := setup(t)
	storageBody := "<h1>見出し1</h1><p>導入</p><h2>節A</h2><p>A の本文</p><h2>節B</h2><p>B の本文</p>"
	sid := e.create(t, e.root, "storage", storageBody, "storage")

	if got := e.storage(t, sid); got != storageBody {
		t.Errorf("作成した本文が送った本文と一致しない:\n%s", got)
	}
	md, header, code := e.run("page", "get", sid)
	if code != 0 {
		t.Fatalf("page get の終了コードが %d: %s", code, header)
	}
	if !strings.HasPrefix(md, "# 見出し1\n") || !strings.Contains(md, "## 節A") || !strings.Contains(md, "A の本文") || strings.Contains(md, "id: ") {
		t.Errorf("markdown の標準出力が本文だけになっていない:\n%s", md)
	}
	for _, s := range []string{"id: " + sid + "\n", "version: 1\n", "---\n"} {
		if !strings.Contains(header, s) {
			t.Errorf("markdown の標準エラーに %q がない:\n%s", s, header)
		}
	}
	p := e.pageJSON(t, sid)
	if p.ID != sid || p.Version != 1 || !strings.Contains(p.Content, "A の本文") || p.URL == "" {
		t.Errorf("json の出力が想定と違う: %+v", p)
	}
	outline := e.mustRun(t, "page", "get", sid, "--outline")
	if !strings.Contains(outline, "- 見出し1\n") || !strings.Contains(outline, "  - 節A\n") || strings.Contains(outline, "A の本文") {
		t.Errorf("--outline の出力が想定と違う:\n%s", outline)
	}
	if got := e.mustRun(t, "page", "get", sid, "--section", "節A", "--format", "storage"); got != "<p>A の本文</p>" {
		t.Errorf("--section の出力 = %q", got)
	}
	if got := e.mustRun(t, "page", "get", sid, "--section", "節B"); !strings.Contains(got, "B の本文") || strings.Contains(got, "A の本文") {
		t.Errorf("--section の markdown の出力が想定と違う:\n%s", got)
	}

	mid := e.create(t, e.root, "markdown", "# MD の見出し\n\n本文 **太字**\n\n```go\nfmt.Println(\"<x>\")\n```\n", "markdown")
	got := e.storage(t, mid)
	for _, s := range []string{"<h1>MD の見出し</h1>", "<strong>太字</strong>", `ac:name="code"`, `<ac:parameter ac:name="language">go</ac:parameter>`, `<![CDATA[fmt.Println("<x>")`} {
		if !strings.Contains(got, s) {
			t.Errorf("Markdown から作った本文に %q がない:\n%s", s, got)
		}
	}

	if out := e.mustWrite(t, mid, "page", "delete"); !strings.Contains(out, "deleted: "+mid) {
		t.Errorf("page delete の出力 = %q", out)
	}
	e.forget(mid)
	cs := e.children(t, e.root)
	if hasChild(cs, mid) {
		t.Error("削除したページがテストルートの子に残っている")
	}
	if !hasChild(cs, sid) {
		t.Error("作成したページがテストルートの子にない")
	}
}

func TestE2E_TreeAndVersions(t *testing.T) {
	e := setup(t)
	parent := e.create(t, e.root, "parent", "<p>版 1 の本文</p>", "storage")
	v1 := e.storage(t, parent)
	grandchild := e.create(t, parent, "grandchild", "<p>孫</p>", "storage")

	if !hasChild(e.children(t, e.root), parent) {
		t.Error("テストルートの子に親ページがない")
	}
	if !hasChild(e.children(t, parent), grandchild) {
		t.Error("親ページの子に孫ページがない")
	}
	var as []struct {
		ID string `json:"id"`
	}
	decode(t, e.mustRun(t, "page", "ancestors", grandchild, "--format", "json"), &as)
	if n := len(as); n < 2 || as[n-2].ID != e.root || as[n-1].ID != parent {
		t.Errorf("孫ページの祖先の末尾が テストルート, 親ページ の順でない: %+v", as)
	}

	e.mustWrite(t, parent, "page", "update", "--body", "<p>版 2 の本文</p>", "--message", "e2e 版 2")
	e.mustWrite(t, parent, "page", "update", "--body", "<p>版 3 の本文</p>", "--message", "e2e 版 3")
	var vs []struct {
		Number  int    `json:"number"`
		Message string `json:"message"`
	}
	decode(t, e.mustRun(t, "page", "versions", parent, "--format", "json"), &vs)
	var nums []int
	for _, v := range vs {
		nums = append(nums, v.Number)
	}
	for _, n := range []int{1, 2, 3} {
		if !slices.Contains(nums, n) {
			t.Errorf("page versions に版 %d がない: %v", n, nums)
		}
	}
	if !slices.ContainsFunc(vs, func(v struct {
		Number  int    `json:"number"`
		Message string `json:"message"`
	}) bool {
		return v.Number == 3 && v.Message == "e2e 版 3"
	}) {
		t.Errorf("版 3 の版コメントがない: %+v", vs)
	}
	assertSame(t, "page get --version 1", v1, e.mustRun(t, "page", "get", parent, "--version", "1", "--format", "storage"))
	if got := e.mustRun(t, "page", "get", parent, "--version", "2", "--format", "storage"); got != "<p>版 2 の本文</p>" {
		t.Errorf("版 2 の本文 = %q", got)
	}
}

func TestE2E_Labels(t *testing.T) {
	e := setup(t)
	id := e.create(t, e.root, "label", "<p>ラベルの確認</p>", "storage")
	label := strings.ToLower(strings.TrimSuffix(runPrefix, "-")) + "-label"

	out := e.mustWrite(t, id, "label", "add", label)
	if !strings.Contains(out, "- "+label) {
		t.Errorf("label add の出力 = %q", out)
	}

	// ラベルの一覧と検索は、付与の直後には反映されないことがあるため、最大 60 秒ポーリングする。
	const timeout, interval = 60 * time.Second, 5 * time.Second
	var labelID string
	if !pollUntil(timeout, interval, func() bool {
		var ls []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		}
		out, _, code := e.run("label", "list", "--prefix", "global", "--all", "--format", "json")
		if code != 0 || json.Unmarshal([]byte(out), &ls) != nil {
			return false
		}
		for _, l := range ls {
			if l.Name == label {
				labelID = l.ID
				return true
			}
		}
		return false
	}) {
		t.Fatalf("label list に %s が %v 以内に出なかった", label, timeout)
	}
	labeled := func() bool {
		var ps []child
		out, _, code := e.run("label", "pages", labelID, "--format", "json")
		return code == 0 && json.Unmarshal([]byte(out), &ps) == nil && hasChild(ps, id)
	}
	if !pollUntil(timeout, interval, labeled) {
		t.Errorf("label pages にページが %v 以内に出なかった", timeout)
	}
	if !pollUntil(timeout, interval, func() bool {
		var hs []child
		out, _, code := e.run("search", "--label", label, "--format", "json")
		return code == 0 && json.Unmarshal([]byte(out), &hs) == nil && hasChild(hs, id)
	}) {
		t.Errorf("search --label にページが %v 以内に出なかった", timeout)
	}

	if out := e.mustWrite(t, id, "label", "remove", label); !strings.Contains(out, "removed: "+label) {
		t.Errorf("label remove の出力 = %q", out)
	}
	if !pollUntil(timeout, interval, func() bool { return !labeled() }) {
		t.Errorf("ラベルを外した後も label pages にページが %v の間残った", timeout)
	}
}

// tinyCode は、ページ ID を 4 バイトのリトルエンディアン整数にし、末尾の 0 バイトを除いて URL セーフな base64（パディングなし）にする。
func tinyCode(t *testing.T, id string) string {
	t.Helper()
	n, err := strconv.ParseUint(id, 10, 32)
	if err != nil {
		t.Fatalf("ページ ID %s が 4 バイトに収まらない: %v", id, err)
	}
	var b [4]byte
	binary.LittleEndian.PutUint32(b[:], uint32(n))
	return base64.RawURLEncoding.EncodeToString([]byte(strings.TrimRight(string(b[:]), "\x00")))
}

func TestE2E_TinyLinkAndURL(t *testing.T) {
	e := setup(t)
	p := e.pageJSON(t, e.root)
	i := strings.Index(p.URL, "/wiki/")
	if i < 0 {
		t.Fatalf("ページの URL に /wiki/ がない")
	}
	base := p.URL[:i] + "/wiki"
	for name, ref := range map[string]string{
		"page-url":  p.URL,
		"viewpage":  base + "/pages/viewpage.action?pageId=" + e.root,
		"tiny-link": base + "/x/" + tinyCode(t, e.root),
	} {
		t.Run(name, func(t *testing.T) {
			var got pageJSON
			decode(t, e.mustRun(t, "page", "get", ref, "--format", "json"), &got)
			if got.ID != e.root {
				t.Errorf("%s で取得したページの ID = %s", name, got.ID)
			}
		})
	}
}

// userConfig は利用者のユーザー設定を読み、変更を加えて一時ディレクトリに書き、XDG_CONFIG_HOME をそのディレクトリにする。
// 認証情報は利用者の設定のものを使い、ファイルの内容はテストの出力に出さない。
func (e *env) userConfig(t *testing.T, change func(m map[string]any)) {
	t.Helper()
	src := filepath.Join(os.Getenv("HOME"), ".config", "confluence-cli", "config.yaml")
	if x := os.Getenv("XDG_CONFIG_HOME"); x != "" {
		src = filepath.Join(x, "confluence-cli", "config.yaml")
	}
	b, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("ユーザー設定を読めない: %v", err)
	}
	m := map[string]any{}
	if err := yaml.Unmarshal(b, &m); err != nil {
		t.Fatalf("ユーザー設定を解釈できない")
	}
	change(m)
	out, err := yaml.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "confluence-cli"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "confluence-cli", "config.yaml"), out, 0o600); err != nil {
		t.Fatal(err)
	}
	e.extraEnv = []string{"XDG_CONFIG_HOME=" + dir}
}

func TestE2E_ReadOnlyAndWritableSpaces(t *testing.T) {
	e := setup(t)
	id := e.create(t, e.root, "page", "<p>元の本文</p>", "storage")
	baseline := e.storage(t, id)
	v := e.version(t, id)
	// --dry-run は書き込まないため、read_only と writable_spaces でも成功する。
	dryRunAllowed := func(t *testing.T) {
		t.Helper()
		if _, errOut, code := e.write(t, id, "page", "update", "--dry-run", "--body", "<p>dry-run の本文</p>"); code != 0 {
			t.Errorf("--dry-run が拒否された: %s", errOut)
		}
	}
	refused := func(t *testing.T, target string, args ...string) {
		t.Helper()
		out, errOut, code := e.write(t, target, args...)
		if args[1] == "create" {
			e.recordCreated(out)
		}
		if code == 0 {
			t.Fatalf("%q が拒否されなかった", args)
		}
		if k := errorKind(t, errOut); k != "read_only" {
			t.Errorf("kind = %q, want read_only", k)
		}
	}

	t.Run("read_only", func(t *testing.T) {
		e.userConfig(t, func(m map[string]any) { m["read_only"] = true; delete(m, "writable_spaces") })
		defer func() { e.extraEnv = nil }()
		refused(t, id, "page", "update", "--title", e.newTitle(t, "ro"), "--format", "json")
		refused(t, e.root, "page", "create", "--title", e.newTitle(t, "ro"), "--body", "<p>x</p>", "--format", "json")
		refused(t, id, "label", "add", "e2e-read-only", "--format", "json")
		refused(t, id, "page", "delete", "--format", "json")
		dryRunAllowed(t)
	})
	t.Run("writable_spaces", func(t *testing.T) {
		e.userConfig(t, func(m map[string]any) { delete(m, "read_only"); m["writable_spaces"] = []string{"E2E-NO-SUCH-SPACE"} })
		defer func() { e.extraEnv = nil }()
		refused(t, id, "page", "update", "--title", e.newTitle(t, "ws"), "--format", "json")
		refused(t, e.root, "page", "create", "--title", e.newTitle(t, "ws"), "--body", "<p>x</p>", "--format", "json")
		refused(t, id, "label", "add", "e2e-writable-spaces", "--format", "json")
		refused(t, id, "page", "delete", "--format", "json")
		dryRunAllowed(t)
	})

	assertSame(t, "拒否された更新", baseline, e.storage(t, id))
	if got := e.version(t, id); got != v {
		t.Errorf("版が %d から %d に変わった", v, got)
	}
	if p := e.pageJSON(t, id); p.Status != "current" {
		t.Errorf("ページの状態 = %q", p.Status)
	}
}

func TestE2E_JSONErrorOnFailure(t *testing.T) {
	e := setup(t)
	t.Run("not_found", func(t *testing.T) {
		// ページ ID 1 のページは存在しないものとして使う。
		_, errOut, code := e.run("page", "get", "1", "--format", "json")
		if code == 0 {
			t.Fatal("存在しないページの取得が成功した")
		}
		if k := errorKind(t, errOut); k != "not_found" {
			t.Errorf("kind = %q, want not_found", k)
		}
	})
	t.Run("validation", func(t *testing.T) {
		_, errOut, code := e.run("search", "--format", "json")
		if code == 0 {
			t.Fatal("条件のない検索が成功した")
		}
		if k := errorKind(t, errOut); k != "validation" {
			t.Errorf("kind = %q, want validation", k)
		}
	})
	t.Run("version_mismatch", func(t *testing.T) {
		id := e.create(t, e.root, "page", "<p>本文</p>", "storage")
		_, errOut, code := e.write(t, id, "page", "update", "--expect-version", "99", "--title", e.newTitle(t, "x"), "--format", "json")
		if code == 0 {
			t.Fatal("違う版番号での更新が成功した")
		}
		if k := errorKind(t, errOut); k != "version_mismatch" {
			t.Errorf("kind = %q, want version_mismatch", k)
		}
	})
}

// TestE2E_SearchReadOnly は検索の各方式と絞り込みフラグが成功することだけを確認する。
// 結果にはテストルート以外のページが含まれるため、結果の内容はテストの出力に出さない。
func TestE2E_SearchReadOnly(t *testing.T) {
	e := setup(t)
	var hs []struct {
		ID       string `json:"id"`
		SpaceKey string `json:"spaceKey"`
	}
	decode(t, e.mustRun(t, "search", "--cql", "id = "+e.root, "--format", "json"), &hs)
	if len(hs) != 1 || hs[0].ID != e.root || hs[0].SpaceKey == "" {
		t.Fatalf("CQL の id 指定でテストルートが 1 件だけ見つからない（%d 件）", len(hs))
	}
	spaceKey := hs[0].SpaceKey
	cases := map[string][]string{
		"siteSearch": {"search", "confluence"},
		"title":      {"search", "confluence", "--title"},
		"text":       {"search", "confluence", "--text"},
		"cql":        {"search", "--cql", "type = page"},
		"space-key":  {"search", "confluence", "--space-key", spaceKey},
		"label":      {"search", "--label", "e2e-no-such-label"},
		"type":       {"search", "confluence", "--type", "blogpost"},
		"since":      {"search", "confluence", "--since", "30d"},
		"since-date": {"search", "confluence", "--since", "2026-01-01"},
		"author":     {"search", "confluence", "--author", "me"},
		"cql-and":    {"search", "--cql", "type = page", "--space-key", spaceKey, "--since", "2w"},
		"sort":       {"search", "confluence", "--sort", "modified"},
		"table":      {"search", "confluence", "--limit", "3", "--format", "table"},
	}
	for name, args := range cases {
		t.Run(name, func(t *testing.T) {
			if !slices.Contains(args, "--format") {
				args = append(args, "--format", "json")
			}
			out, errOut, code := e.run(args...)
			if code != 0 {
				t.Fatalf("終了コード %d: %s", code, errOut)
			}
			if slices.Contains(args, "json") && !json.Valid([]byte(out)) {
				t.Errorf("出力が JSON ではない")
			}
		})
	}
}
