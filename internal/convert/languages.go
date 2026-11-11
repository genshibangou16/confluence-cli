package convert

import "strings"

// codeLanguages は Confluence の code マクロが知っている言語名の一覧。
// 一覧にない言語名を language パラメータに渡すと Confluence がコードを正しく表示しないため、一覧にない名前は text にする。
var codeLanguages = map[string]bool{
	"actionscript3": true, "applescript": true, "bash": true, "c#": true, "cpp": true, "css": true,
	"coldfusion": true, "delphi": true, "diff": true, "erl": true, "groovy": true, "xml": true,
	"java": true, "jfx": true, "php": true, "perl": true, "text": true, "powershell": true,
	"ruby": true, "sql": true, "sass": true, "scala": true, "vb": true, "go": true, "kotlin": true,
	"typescript": true, "json": true, "swift": true, "rust": true, "csharp": true, "javascript": true,
	"python": true, "yaml": true, "html/xml": true, "c": true,
}

// languageAliases は Markdown のフェンスで使われる言語名の別名から Confluence の言語名への対応。
var languageAliases = map[string]string{
	"sh": "bash", "zsh": "bash", "shell": "bash", "console": "bash",
	"golang": "go",
	"py":     "python", "python3": "python",
	"js": "javascript", "node": "javascript",
	"ts":   "typescript",
	"yml":  "yaml",
	"html": "html/xml",
	"c++":  "cpp",
	"cs":   "csharp",
	"rb":   "ruby", "kt": "kotlin", "rs": "rust", "erlang": "erl",
}

// NormalizeLanguage は Markdown のフェンスの言語名を Confluence の code マクロの言語名にする。
// 大文字と小文字を区別しない。空なら空を返す。
func NormalizeLanguage(s string) string {
	if s == "" {
		return ""
	}
	s = strings.ToLower(s)
	if a, ok := languageAliases[s]; ok {
		return a
	}
	if codeLanguages[s] {
		return s
	}
	return "text"
}
