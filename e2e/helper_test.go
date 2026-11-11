//go:build e2e

// Package e2e は、ビルドしたバイナリを実際の Confluence に対して実行するテスト。
// 書き込みはテストルート（環境変数 CONFLUENCE_E2E_ROOT のページ）とその子孫だけに行う。
// テストルート以外のページへの書き込みを防ぐため、書き込み系コマンドは env.write からだけ実行し、
// env.write は実行前に対象ページの祖先にテストルートがあることを API で確認する。
package e2e

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/aymanbagabas/go-udiff"
)

const rootEnv = "CONFLUENCE_E2E_ROOT"

// binPath は TestMain でビルドしたバイナリ。
var binPath string

// runPrefix は、この実行で作ったページのタイトルの先頭に付ける文字列。
// 後片付けで、記録から漏れた子ページを見つけるために使う。
var runPrefix = fmt.Sprintf("e2e-%s-", time.Now().Format("20060102-150405"))

func TestMain(m *testing.M) {
	if os.Getenv(rootEnv) == "" {
		// setup が各テストを t.Skip する。ビルドは不要。
		os.Exit(m.Run())
	}
	dir, err := os.MkdirTemp("", "confluence-cli-e2e-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	binPath = filepath.Join(dir, "confluence-cli")
	build := exec.Command("go", "build", "-o", binPath, ".")
	build.Dir = ".."
	build.Stdout, build.Stderr = os.Stderr, os.Stderr
	if err := build.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "build:", err)
		os.Exit(1)
	}
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

type env struct {
	bin  string
	root string
	// rootTitle と rootBody は開始時のテストルートのタイトルと本文（storage 形式）。
	rootTitle   string
	rootBody    string
	rootVersion int
	// extraEnv はバイナリの実行時に環境変数へ足す KEY=VALUE。
	extraEnv []string
	// created は作ったページの ID を作った順に持つ。後片付けでは逆順に消し、子孫から消す。
	created []string
	// executed は最後に実行したコマンドの引数。ガードがコマンドを実行しなかったことの確認に使う。
	executed []string
	// restoringRoot は、後片付けでテストルートを開始時の状態に戻している間だけ true。
	// このときに限り、テストルート自体を page update の対象にできる。
	restoringRoot bool
}

// setup は CONFLUENCE_E2E_ROOT がなければテストをスキップする。
// テストルートのタイトル・本文・版番号を保存し、t.Cleanup で作った子ページを消してからテストルートを確認・復元する。
func setup(t *testing.T) *env {
	t.Helper()
	root := os.Getenv(rootEnv)
	if root == "" {
		t.Skip(rootEnv + " が未設定のため E2E テストをスキップする")
	}
	if !isPageID(root) {
		t.Fatalf("%s はページ ID（数字）で指定する", rootEnv)
	}
	e := &env{bin: binPath, root: root}
	e.rootTitle = e.title(t, root)
	e.rootBody = e.storage(t, root)
	e.rootVersion = e.version(t, root)
	t.Cleanup(func() { e.cleanup(t) })
	e.removeLeftovers(t)
	return e
}

// removeLeftovers は、中断した実行が残した e2e- で始まるテストルートの子ページを削除する。
// 後片付けは実行ごとの接頭辞で照合するため、中断した実行のページは後片付けでは見つからない。
func (e *env) removeLeftovers(t *testing.T) {
	t.Helper()
	var cs []struct {
		ID    string `json:"id"`
		Title string `json:"title"`
	}
	decode(t, e.mustRun(t, "page", "children", e.root, "--all", "--format", "json"), &cs)
	n := 0
	for _, c := range cs {
		if !strings.HasPrefix(c.Title, "e2e-") {
			continue
		}
		e.mustWrite(t, c.ID, "page", "delete")
		n++
	}
	if n > 0 {
		t.Logf("setup: 中断した実行が残した子ページを %d 件削除した", n)
	}
}

func isPageID(s string) bool {
	_, err := strconv.ParseUint(s, 10, 64)
	return err == nil && s != ""
}

// run は読み取り系のコマンドを実行する。書き込み系のコマンドは実行せず、終了コード -1 を返す。
func (e *env) run(args ...string) (stdout, stderr string, code int) {
	if isWriteCommand(args) {
		return "", "e2e: 書き込み系のコマンドは env.write で実行する", -1
	}
	return e.exec("", args...)
}

func (e *env) exec(stdin string, args ...string) (stdout, stderr string, code int) {
	e.executed = append([]string(nil), args...)
	cmd := exec.Command(e.bin, args...)
	cmd.Env = append(os.Environ(), e.extraEnv...)
	cmd.Stdin = strings.NewReader(stdin)
	var out, errb strings.Builder
	cmd.Stdout, cmd.Stderr = &out, &errb
	err := cmd.Run()
	code = 0
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			code = ee.ExitCode()
		} else {
			code = -1
			errb.WriteString(err.Error())
		}
	}
	return out.String(), errb.String(), code
}

// writeCommands は env.write でだけ実行できるコマンド。
var writeCommands = map[string]bool{
	"page create": true, "page update": true, "page delete": true,
	"label add": true, "label remove": true,
}

func isWriteCommand(args []string) bool {
	return len(args) >= 2 && writeCommands[args[0]+" "+args[1]]
}

// fatalf は *testing.T の Fatalf と同じく、呼んだ goroutine を終了させる関数。
// TestE2E_Guard で、ガードが失敗させることを *testing.T を失敗させずに確かめるために差し替える。
type fatalf func(format string, args ...any)

// write は target のページへの書き込み系コマンドを、target がテストルートかその子孫であることを確認してから実行する。
// args は "page update" のようにコマンド名から始め、ページの引数を含めない。
// page create では target を --parent に、それ以外では target をコマンド名の直後のページ引数にする。
func (e *env) write(t *testing.T, target string, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	return e.writeIn(t, target, "", args...)
}

// writeIn は標準入力を渡す write。
func (e *env) writeIn(t *testing.T, target, stdin string, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	return e.guardedWrite(t.Fatalf, target, stdin, args...)
}

func (e *env) guardedWrite(fail fatalf, target, stdin string, args ...string) (stdout, stderr string, code int) {
	full, err := e.writeArgs(target, args)
	if err == nil {
		err = e.checkTarget(target, args)
	}
	if err != nil {
		fail("e2e: 書き込みを実行しない: %v", err)
		return "", "", -1
	}
	return e.exec(stdin, full...)
}

// writeArgs は write に渡された引数から、実行する引数を組み立てる。
func (e *env) writeArgs(target string, args []string) ([]string, error) {
	if !isWriteCommand(args) {
		return nil, fmt.Errorf("書き込み系のコマンドではない: %q", args)
	}
	if !isPageID(target) {
		return nil, fmt.Errorf("対象はページ ID（数字）で指定する: %q", target)
	}
	for _, a := range args[2:] {
		// ページの指定を target 以外から渡せないようにする。
		if a == "--parent" || a == "--parent-id" || strings.HasPrefix(a, "--parent=") || strings.HasPrefix(a, "--parent-id=") ||
			a == "--space-key" || a == "--space-id" || strings.HasPrefix(a, "--space-key=") || strings.HasPrefix(a, "--space-id=") {
			return nil, fmt.Errorf("書き込み先は target で指定する: %q", a)
		}
	}
	if args[0]+" "+args[1] == "page create" {
		return append(append([]string{}, args...), "--parent", target), nil
	}
	return append([]string{args[0], args[1], target}, args[2:]...), nil
}

// checkTarget は、テストルート自体を対象にできる場合を絞ったうえで checkWritable を呼ぶ。
// テストルート自体は、page create の親と、後片付けでの復元だけが対象にできる。
func (e *env) checkTarget(target string, args []string) error {
	if target == e.root && args[0]+" "+args[1] != "page create" && !e.restoringRoot {
		return fmt.Errorf("テストルート自体は %q の対象にできない", args[0]+" "+args[1])
	}
	return e.checkWritable(target)
}

// checkWritable は target がテストルートか、祖先にテストルートを含むページなら nil を返す。
func (e *env) checkWritable(target string) error {
	if target == e.root {
		return nil
	}
	out, errOut, code := e.exec("", "page", "ancestors", target, "--format", "json")
	if code != 0 {
		return fmt.Errorf("ページ %s の祖先を取得できない（終了コード %d）: %s", target, code, strings.TrimSpace(errOut))
	}
	var as []struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(out), &as); err != nil {
		return fmt.Errorf("ページ %s の祖先を解釈できない: %v", target, err)
	}
	for _, a := range as {
		if a.ID == e.root {
			return nil
		}
	}
	return fmt.Errorf("ページ %s はテストルートの子孫ではない", target)
}

// mustRun は読み取り系のコマンドを実行し、終了コードが 0 でなければテストを失敗させる。
func (e *env) mustRun(t *testing.T, args ...string) string {
	t.Helper()
	out, errOut, code := e.run(args...)
	if code != 0 {
		t.Fatalf("%q の終了コードが %d: %s", args, code, errOut)
	}
	return out
}

// mustWrite は write を実行し、終了コードが 0 でなければテストを失敗させる。
func (e *env) mustWrite(t *testing.T, target string, args ...string) string {
	t.Helper()
	out, errOut, code := e.write(t, target, args...)
	if code != 0 {
		t.Fatalf("%q（対象 %s）の終了コードが %d: %s", args, target, code, errOut)
	}
	return out
}

// storage はページの本文を storage 形式で返す。
func (e *env) storage(t *testing.T, id string) string {
	t.Helper()
	return e.mustRun(t, "page", "get", id, "--format", "storage")
}

type pageJSON struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Status  string `json:"status"`
	SpaceID string `json:"spaceId"`
	Version int    `json:"version"`
	URL     string `json:"url"`
	Content string `json:"content"`
}

func (e *env) pageJSON(t *testing.T, id string) pageJSON {
	t.Helper()
	var p pageJSON
	decode(t, e.mustRun(t, "page", "get", id, "--format", "json"), &p)
	return p
}

func (e *env) version(t *testing.T, id string) int  { t.Helper(); return e.pageJSON(t, id).Version }
func (e *env) title(t *testing.T, id string) string { t.Helper(); return e.pageJSON(t, id).Title }

func decode(t *testing.T, s string, v any) {
	t.Helper()
	if err := json.Unmarshal([]byte(s), v); err != nil {
		t.Fatalf("JSON を解釈できない: %v\n%s", err, s)
	}
}

// newTitle は、この実行の接頭辞とテスト名を付けたページのタイトルを返す。
func (e *env) newTitle(t *testing.T, name string) string {
	return runPrefix + safeName(t.Name()) + "-" + name
}

var unsafeTitleRe = regexp.MustCompile(`[^A-Za-z0-9_-]+`)

func safeName(s string) string { return unsafeTitleRe.ReplaceAllString(s, "_") }

// create は parent の子ページを作り、ID を返す。作ったページは後片付けで消す。
// input は storage か markdown。
func (e *env) create(t *testing.T, parent, name, body, input string) string {
	t.Helper()
	out, errOut, code := e.writeIn(t, parent, body, "page", "create", "--title", e.newTitle(t, name),
		"--body-file", "-", "--input-format", input, "--format", "json")
	var w struct {
		ID string `json:"id"`
	}
	// 書き込み後の確認で失敗しても、作ったページの情報は出るため、終了コードより先に ID を記録する。
	if json.Unmarshal([]byte(out), &w) == nil && w.ID != "" {
		e.created = append(e.created, w.ID)
	}
	if code != 0 {
		t.Fatalf("ページを作れない（終了コード %d）: %s", code, errOut)
	}
	return w.ID
}

// fixture は testdata/storage のフィクスチャを返す。
func fixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "testdata", "storage", name))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

var fixtures = []string{"rich.xml", "rich-nolayout.xml"}

// assertSame は got と want がバイト単位で一致しなければ、ブロック要素ごとに改行した unified diff を出して失敗させる。
func assertSame(t *testing.T, label, want, got string) {
	t.Helper()
	if got == want {
		return
	}
	d := udiff.Unified("want", "got", splitBlocks(want), splitBlocks(got))
	if d == "" {
		// 改行の位置だけが違うときは、ブロックごとの diff に差が出ないため、両方をそのまま出す。
		d = fmt.Sprintf("want: %q\ngot:  %q", want, got)
	}
	t.Errorf("%s: 本文がベースラインと一致しない\n%s", label, d)
}

// blockBreaker は、改行をほとんど含まない storage 形式を、diff の行がブロック要素ごとになるように改行する。表示だけに使う。
var blockBreaker = func() *strings.Replacer {
	var pairs []string
	for _, tag := range []string{"</p>", "</h1>", "</h2>", "</h3>", "</li>", "</tr>", "</table>", "</ul>", "</ol>", "</blockquote>",
		"</ac:structured-macro>", "</ac:layout-cell>", "</ac:layout-section>", "</ac:task>", "<hr />"} {
		pairs = append(pairs, tag, tag+"\n")
	}
	return strings.NewReplacer(pairs...)
}()

func splitBlocks(s string) string { return blockBreaker.Replace(s) }

// errJSON は --format json での失敗時に標準エラーに出る JSON。
type errJSON struct {
	Error struct {
		Kind    string `json:"kind"`
		Status  int    `json:"status"`
		Message string `json:"message"`
	} `json:"error"`
}

// errorKind は標準エラーが JSON 1 つだけであることを確認し、エラーの種類を返す。
func errorKind(t *testing.T, stderr string) string {
	t.Helper()
	dec := json.NewDecoder(strings.NewReader(stderr))
	var e errJSON
	if err := dec.Decode(&e); err != nil {
		t.Fatalf("標準エラーが JSON ではない: %v\n%s", err, stderr)
	}
	if dec.More() {
		t.Fatalf("標準エラーに JSON が 2 つ以上ある:\n%s", stderr)
	}
	if e.Error.Kind == "" || e.Error.Message == "" {
		t.Fatalf("エラーの JSON に kind か message がない:\n%s", stderr)
	}
	return e.Error.Kind
}

// cleanup は作った子ページを消し、記録から漏れたこのテストの子ページも消す。そのあとテストルートを確認する。
// 後片付けの失敗で残りの後片付けが止まらないよう、t.Fatal ではなく t.Error を使う。
func (e *env) cleanup(t *testing.T) {
	e.extraEnv = nil
	logErr := func(format string, args ...any) { t.Errorf(format, args...) }
	for i := len(e.created) - 1; i >= 0; i-- {
		id := e.created[i]
		if _, errOut, code := e.guardedWrite(logErr, id, "", "page", "delete"); code != 0 {
			t.Errorf("後片付け: ページ %s を削除できない: %s", id, errOut)
		}
	}
	// サブテストで作ったページのタイトルは、親のテスト名に続けて _ とサブテスト名が付くため、親のテスト名までで照合する。
	prefix := runPrefix + safeName(t.Name())
	out, errOut, code := e.run("page", "children", e.root, "--all", "--format", "json")
	if code != 0 {
		t.Errorf("後片付け: テストルートの子を取得できない: %s", errOut)
	} else {
		var cs []struct {
			ID    string `json:"id"`
			Title string `json:"title"`
		}
		if err := json.Unmarshal([]byte(out), &cs); err != nil {
			t.Errorf("後片付け: 子の一覧を解釈できない: %v", err)
		}
		for _, c := range cs {
			if !strings.HasPrefix(c.Title, prefix) {
				continue
			}
			t.Errorf("後片付け: 記録から漏れた子ページ %s（%s）を削除する", c.ID, c.Title)
			if _, errOut, code := e.guardedWrite(logErr, c.ID, "", "page", "delete"); code != 0 {
				t.Errorf("後片付け: ページ %s を削除できない: %s", c.ID, errOut)
			}
		}
	}
	e.checkRoot(t)
}

// checkRoot はテストルートのタイトルと本文が開始時と同じかを確認し、違えば開始時の状態に戻す。
// テストはテストルートを書き換えないため、版番号が変わっていれば失敗にする。
func (e *env) checkRoot(t *testing.T) {
	out, errOut, code := e.run("page", "get", e.root, "--format", "json", "--content-format", "storage")
	if code != 0 {
		t.Errorf("後片付け: テストルートを取得できない: %s", errOut)
		return
	}
	var p pageJSON
	if err := json.Unmarshal([]byte(out), &p); err != nil {
		t.Errorf("後片付け: テストルートを解釈できない: %v", err)
		return
	}
	if p.Version != e.rootVersion {
		t.Errorf("後片付け: テストルートの版が %d から %d に変わった", e.rootVersion, p.Version)
	}
	if p.Title == e.rootTitle && p.Content == e.rootBody {
		return
	}
	e.restoringRoot = true
	defer func() { e.restoringRoot = false }()
	logErr := func(format string, args ...any) { t.Errorf(format, args...) }
	if strings.TrimSpace(e.rootBody) == "" {
		// page update は本文全体を空にする更新を拒否するため、本文が変わっていれば CLI では空に戻せない。
		if p.Content != e.rootBody {
			t.Errorf("後片付け: テストルートの本文が変わったが、開始時の本文が空のため CLI では戻せない。Web UI で戻す")
			return
		}
		if _, errOut, code := e.guardedWrite(logErr, e.root, "", "page", "update", "--title", e.rootTitle); code != 0 {
			t.Errorf("後片付け: テストルートのタイトルを戻せない: %s", errOut)
		}
		return
	}
	if _, errOut, code := e.guardedWrite(logErr, e.root, e.rootBody, "page", "update", "--title", e.rootTitle,
		"--body-file", "-", "--allow-drop-comments", "--allow-lossy"); code != 0 {
		t.Errorf("後片付け: テストルートを戻せない: %s", errOut)
	}
}

// pollUntil は cond が true を返すまで、最大 timeout の間 interval ごとに呼ぶ。
func pollUntil(timeout, interval time.Duration, cond func() bool) bool {
	deadline := time.Now().Add(timeout)
	for {
		if cond() {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(interval)
	}
}

// goexitRecorder は fatalf の代わりに渡し、呼ばれたことを記録してから goroutine を終了させる。
type goexitRecorder struct{ msg string }

func (r *goexitRecorder) fatalf(format string, args ...any) {
	r.msg = fmt.Sprintf(format, args...)
	runtime.Goexit()
}
