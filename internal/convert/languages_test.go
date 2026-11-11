package convert

import "testing"

func TestNormalizeLanguage(t *testing.T) {
	for in, want := range map[string]string{
		"": "", "sh": "bash", "zsh": "bash", "shell": "bash", "console": "bash", "golang": "go",
		"py": "python", "python3": "python", "js": "javascript", "node": "javascript", "ts": "typescript",
		"yml": "yaml", "html": "html/xml", "c++": "cpp", "cs": "csharp", "C#": "c#", "Go": "go",
		"java": "java", "html/xml": "html/xml", "sql": "sql", "foo": "text",
	} {
		if got := NormalizeLanguage(in); got != want {
			t.Errorf("NormalizeLanguage(%q) = %q, want %q", in, got, want)
		}
	}
}
