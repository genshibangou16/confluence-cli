//go:build e2e

package e2e

import (
	"regexp"
	"strings"
	"testing"

	"github.com/genshibangou16/confluence-cli/internal/convert/frommd"
)

// withFixtures はフィクスチャごとに子ページを作り、作成直後の storage 形式をベースラインとして f に渡す。
// ベースラインは Confluence が保存時に行う正規化を反映した本文で、以降の比較はすべてベースラインとのバイト単位の一致で行う。
func withFixtures(t *testing.T, e *env, f func(t *testing.T, id, baseline string)) {
	t.Helper()
	for _, name := range fixtures {
		t.Run(name, func(t *testing.T) {
			id := e.create(t, e.root, "page", fixture(t, name), "storage")
			f(t, id, e.storage(t, id))
		})
	}
}

// sectionRange は baseline のトップレベルの見出し heading の中身の範囲を、CLI の実装とは独立に文字列の位置で求める。
// 中身はその見出しの直後から、endMarkers とレイアウトのセルの終わりのうち最初に現れるものの直前まで。どれも無ければ本文の終わりまで。
func sectionRange(t *testing.T, baseline, heading string, endMarkers ...string) (start, end int) {
	t.Helper()
	i := strings.Index(baseline, heading)
	if i < 0 {
		t.Fatalf("ベースラインに %s がない", heading)
	}
	start = i + len(heading)
	end = len(baseline)
	for _, m := range append(endMarkers, "</ac:layout-cell>") {
		if j := strings.Index(baseline[start:], m); j >= 0 && start+j < end {
			end = start + j
		}
	}
	return start, end
}

func TestE2E_TitleOnlyUpdateKeepsBody(t *testing.T) {
	e := setup(t)
	withFixtures(t, e, func(t *testing.T, id, baseline string) {
		title := e.newTitle(t, "renamed")
		e.mustWrite(t, id, "page", "update", "--title", title)
		assertSame(t, "タイトルだけの更新", baseline, e.storage(t, id))
		if got := e.title(t, id); got != title {
			t.Errorf("タイトル = %q, want %q", got, title)
		}
	})
}

func TestE2E_MessageOnlyUpdateKeepsBody(t *testing.T) {
	e := setup(t)
	withFixtures(t, e, func(t *testing.T, id, baseline string) {
		v := e.version(t, id)
		e.mustWrite(t, id, "page", "update", "--message", "e2e 版コメントだけの更新")
		assertSame(t, "版コメントだけの更新", baseline, e.storage(t, id))
		if got := e.version(t, id); got != v+1 {
			t.Errorf("版 = %d, want %d", got, v+1)
		}
		if out := e.mustRun(t, "page", "versions", id, "--format", "json"); !strings.Contains(out, "e2e 版コメントだけの更新") {
			t.Errorf("page versions に版コメントがない:\n%s", out)
		}
	})
}

func TestE2E_StorageRoundTripIdentical(t *testing.T) {
	e := setup(t)
	withFixtures(t, e, func(t *testing.T, id, baseline string) {
		got := e.storage(t, id)
		// 本文全体の置き換えでは、送る本文にマーカーが残っていても、現在の本文のマーカーの数で拒否されるため、--allow-drop-comments を付ける。
		// 書き戻した本文にマーカーが残ることは、ベースラインとの一致で確認する。
		if _, errOut, code := e.writeIn(t, id, got, "page", "update", "--body-file", "-", "--allow-drop-comments"); code != 0 {
			t.Fatalf("書き戻しの終了コードが %d: %s", code, errOut)
		}
		assertSame(t, "storage 形式の書き戻し", baseline, e.storage(t, id))
	})
}

func TestE2E_AppendPrependKeepBaseline(t *testing.T) {
	e := setup(t)
	withFixtures(t, e, func(t *testing.T, id, baseline string) {
		const add = "<p>e2e で末尾に追加した段落</p>"
		e.mustWrite(t, id, "page", "update", "--append", "--body", add)
		appended := e.storage(t, id)
		assertSame(t, "--append", baseline+add, appended)

		const pre = "<p>e2e で先頭に追加した段落</p>"
		e.mustWrite(t, id, "page", "update", "--prepend", "--body", pre)
		assertSame(t, "--prepend", pre+appended, e.storage(t, id))
	})
}

func TestE2E_SectionReplaceKeepsOutside(t *testing.T) {
	e := setup(t)
	withFixtures(t, e, func(t *testing.T, id, baseline string) {
		// トップレベルの「背景」の節は、次のトップレベルの h2 である「手順」の直前まで。
		// expand の中にある同名の見出しは「手順」より後にあるため、節の外側としてベースラインとの一致で確認する。
		start, end := sectionRange(t, baseline, "<h2>背景</h2>", "<h2>手順</h2>")
		const repl = "<p>e2e で置き換えた節</p>"
		// フィクスチャの「背景」の節にはインラインコメントのマーカーがあるため、--allow-drop-comments を付ける。
		e.mustWrite(t, id, "page", "update", "--section", "背景", "--body", repl, "--allow-drop-comments")
		got := e.storage(t, id)
		assertSame(t, "--section", baseline[:start]+repl+baseline[end:], got)
		if !strings.Contains(got, "<h2>背景</h2><p>expand の中にある、トップレベルと同じ文字列の見出し。</p>") {
			t.Errorf("expand の中の見出しが変わった")
		}
	})
}

func TestE2E_MarkdownPartialUpdate(t *testing.T) {
	e := setup(t)
	const appendMD = "## e2e 追記\n\n- [ ] e2e の追加タスク\n- [x] e2e の完了タスク\n\n```sh\necho \"a < b\" && exit 0\n```\n\n> [!WARNING]\n> e2e の注意\n"
	const sectionMD = "e2e で置き換えた**まとめ**。\n\n1. 一つ目\n2. 二つ目\n"
	wantAppend, err := frommd.Convert(appendMD)
	if err != nil {
		t.Fatal(err)
	}
	wantSection, err := frommd.Convert(sectionMD)
	if err != nil {
		t.Fatal(err)
	}
	withFixtures(t, e, func(t *testing.T, id, baseline string) {
		e.mustWrite(t, id, "page", "update", "--append", "--input-format", "markdown", "--body", appendMD)
		appended := e.storage(t, id)
		if !strings.HasPrefix(appended, baseline) {
			t.Errorf("Markdown の --append: 既存の本文がベースラインと一致しない")
		}
		assertSame(t, "Markdown の --append", baseline+wantAppend, appended)
		reportTaskIDs(t, appended)

		// レイアウトのないページでは、追記した h2 が「まとめ」の節の終わりになる。
		start, end := sectionRange(t, appended, "<h2>まとめ</h2>", "<h2>e2e 追記</h2>")
		e.mustWrite(t, id, "page", "update", "--section", "まとめ", "--input-format", "markdown", "--body", sectionMD)
		assertSame(t, "Markdown の --section", appended[:start]+wantSection+appended[end:], e.storage(t, id))
	})
}

var taskIDRe = regexp.MustCompile(`<ac:task-id>([^<]*)</ac:task-id>`)

// reportTaskIDs は、既存のタスクリストがあるページに Markdown のタスクリストを追記したときの ac:task-id を記録する。
// Markdown の変換は ac:task-id に 1 からの連番を振るため、既存のタスクと番号が重なる。Confluence がその番号をどう扱うかを残す。
func reportTaskIDs(t *testing.T, body string) {
	t.Helper()
	var ids []string
	for _, m := range taskIDRe.FindAllStringSubmatch(body, -1) {
		ids = append(ids, m[1])
	}
	t.Logf("追記後の ac:task-id: %v", ids)
	for _, s := range []string{"e2e の追加タスク", "e2e の完了タスク"} {
		if !strings.Contains(body, s) {
			t.Errorf("追記したタスク %q が本文にない", s)
		}
	}
}

func TestE2E_CommentMarkerRefused(t *testing.T) {
	e := setup(t)
	withFixtures(t, e, func(t *testing.T, id, baseline string) {
		start, end := sectionRange(t, baseline, "<h2>背景</h2>", "<h2>手順</h2>")
		if !strings.Contains(baseline[start:end], "ac:inline-comment-marker") {
			t.Skip("Confluence が保存時にフィクスチャのインラインコメントのマーカーを削除した。Web UI でインラインコメントを付けたページで確認する")
		}
		v := e.version(t, id)
		_, errOut, code := e.write(t, id, "page", "update", "--section", "背景", "--body", "<p>x</p>", "--format", "json")
		if code == 0 {
			t.Fatal("マーカーを含む節の置き換えが拒否されなかった")
		}
		if k := errorKind(t, errOut); k != "comment_loss" {
			t.Errorf("kind = %q, want comment_loss", k)
		}
		if got := e.version(t, id); got != v {
			t.Errorf("版が %d から %d に変わった", v, got)
		}
		assertSame(t, "拒否された更新", baseline, e.storage(t, id))
	})
}

func TestE2E_LossyMarkdownReplaceRefused(t *testing.T) {
	e := setup(t)
	// インラインコメントのマーカーがあると先に comment_loss で拒否されるため、マーカーのない本文で確認する。
	// status マクロは Markdown の変換規則がないマクロ。
	body := `<h2>見出し</h2><p>状態: <ac:structured-macro ac:name="status" ac:schema-version="1"><ac:parameter ac:name="colour">Green</ac:parameter><ac:parameter ac:name="title">完了</ac:parameter></ac:structured-macro></p>`
	id := e.create(t, e.root, "lossy", body, "storage")
	baseline := e.storage(t, id)
	v := e.version(t, id)
	_, errOut, code := e.write(t, id, "page", "update", "--input-format", "markdown", "--body", "# 置き換え\n", "--format", "json")
	if code == 0 {
		t.Fatal("変換規則のないマクロを含むページの置き換えが拒否されなかった")
	}
	if k := errorKind(t, errOut); k != "lossy" {
		t.Errorf("kind = %q, want lossy", k)
	}
	if got := e.version(t, id); got != v {
		t.Errorf("版が %d から %d に変わった", v, got)
	}
	assertSame(t, "拒否された更新", baseline, e.storage(t, id))
}
