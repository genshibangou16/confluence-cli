package storage

import (
	"encoding/xml"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidate_AcceptsRichFixture(t *testing.T) {
	b, err := os.ReadFile("../../testdata/storage/rich.xml")
	if err != nil {
		t.Fatal(err)
	}
	if err := Validate(string(b)); err != nil {
		t.Fatalf("Validate(rich.xml) = %v", err)
	}
}

func TestValidate_AcceptsEmpty(t *testing.T) {
	if err := Validate(""); err != nil {
		t.Fatalf("Validate(\"\") = %v", err)
	}
}

func TestValidate_AcceptsEntities(t *testing.T) {
	if err := Validate("<p>a&nbsp;b&mdash;c</p>"); err != nil {
		t.Fatalf("Validate = %v", err)
	}
}

func TestValidate_ReportsPosition(t *testing.T) {
	cases := []struct {
		name string
		body string
		line int
		col  int
	}{
		// 列は、パーサーが誤りを検出した時点で読み終えた位置（1 始まり）。
		{"mismatched end tag", "<p>ok</p>\n<p>bad</div>", 2, 13},
		{"first line", "<p>bad</div>", 1, 13},
		{"unclosed at end", "<p>ok</p>\n<p>open", 2, 8},
		{"undefined entity", "<p>a&foo;b</p>", 1, 10},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := Validate(c.body)
			var se *SyntaxError
			if !errors.As(err, &se) {
				t.Fatalf("Validate(%q) = %v, want *SyntaxError", c.body, err)
			}
			if se.Line != c.line || se.Column != c.col {
				t.Errorf("position = %d:%d, want %d:%d (%s)", se.Line, se.Column, c.line, c.col, se.Msg)
			}
			if se.Msg == "" {
				t.Error("Msg is empty")
			}
		})
	}
}

func TestDecoder_PrefixLength(t *testing.T) {
	body := `<p>x</p>`
	d, n := Decoder(body)
	tok, err := d.Token() // ルート要素
	if err != nil {
		t.Fatal(err)
	}
	_ = tok
	tok, err = d.Token() // <p>
	if err != nil {
		t.Fatal(err)
	}
	if got := d.InputOffset() - int64(n); got != int64(len("<p>")) {
		t.Errorf("offset in body after <p> = %d, want %d", got, len("<p>"))
	}
}

func TestValidate_PrivateFixtures(t *testing.T) {
	files, err := filepath.Glob("../../testdata/private/*.xml")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Skip("testdata/private/*.xml がない")
	}
	for _, f := range files {
		t.Run(filepath.Base(f), func(t *testing.T) {
			b, err := os.ReadFile(f)
			if err != nil {
				t.Fatal(err)
			}
			if err := Validate(string(b)); err != nil {
				t.Errorf("Validate = %v", err)
			}
		})
	}
}

// Decoder が CDATA の分割を連結して読むことを、フィクスチャの code マクロで確認する。
func TestDecoder_RichFixtureCDATA(t *testing.T) {
	b, err := os.ReadFile("../../testdata/storage/rich.xml")
	if err != nil {
		t.Fatal(err)
	}
	d, _ := Decoder(string(b))
	var code strings.Builder
	inBody := false
	for {
		tok, err := d.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		switch tok := tok.(type) {
		case xml.StartElement:
			if tok.Name.Local == "plain-text-body" && code.Len() == 0 {
				inBody = true
			}
		case xml.EndElement:
			inBody = false
		case xml.CharData:
			if inBody {
				code.Write(tok)
			}
		}
	}
	for _, want := range []string{"]]>", "a < b && b > 0", `"<ok> & done"`} {
		if !strings.Contains(code.String(), want) {
			t.Errorf("code body does not contain %q", want)
		}
	}
}
