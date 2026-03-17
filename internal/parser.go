package parser

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var (
	headerRegex   = regexp.MustCompile(`^\[([^\]]+)\]\s*$`)
	metaKeyRegex  = regexp.MustCompile(`^@([a-zA-Z_][a-zA-Z0-9_-]*)(?:=(.*))?$`)
	envLineRegex  = regexp.MustCompile(`^([A-Z_][A-Z0-9_]*)=(.*)$`)
)

type File struct {
	Name        string
	Content     []byte
	Lang        string
	RunAfter    []string
	Tags        []string
	Env         map[string]string
	Args        []string
	Disabled    bool
	Hidden      bool
	Stdin       string
	Description string
}

type Bundle struct {
	Files       []*File
	Source      string
	Meta        Meta
}

type Meta struct {
	Entrypoint  string
	Name        string
	Version     string
	Description string
	Parallel    bool
	StopOnFail  bool
	Env         map[string]string
	Pipeline    []string
	Watch       []string
	Shell       string
}

func ParseFile(path string) (*Bundle, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("cannot open bundle: %w", err)
	}
	return ParseBytes(data, path)
}

func ParseBytes(data []byte, source string) (*Bundle, error) {
	bundle := &Bundle{
		Source: source,
		Meta: Meta{
			Env: make(map[string]string),
		},
	}

	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	scanner.Buffer(make([]byte, 4*1024*1024), 4*1024*1024)

	var current *File
	var lines []string
	var inFileEnv bool

	flush := func() {
		if current == nil {
			return
		}
		content := strings.Join(lines, "\n")
		content = strings.TrimRight(content, "\n")
		current.Content = []byte(content)
		bundle.Files = append(bundle.Files, current)
		current = nil
		lines = nil
		inFileEnv = false
	}

	for scanner.Scan() {
		line := scanner.Text()

		if m := headerRegex.FindStringSubmatch(line); m != nil {
			flush()
			raw := strings.TrimSpace(m[1])

			if mm := metaKeyRegex.FindStringSubmatch(raw); mm != nil {
				key := strings.ToLower(mm[1])
				val := strings.TrimSpace(mm[2])
				applyBundleMeta(bundle, key, val)
				continue
			}

			name, attrs := parseFileHeader(raw)
			f := &File{
				Name: name,
				Lang: DetectLang(name),
				Env:  make(map[string]string),
			}
			applyFileAttrs(f, attrs)
			current = f
			lines = []string{}
			inFileEnv = false
			continue
		}

		if current != nil {
			if inFileEnv {
				if em := envLineRegex.FindStringSubmatch(strings.TrimSpace(line)); em != nil {
					current.Env[em[1]] = em[2]
					continue
				}
				inFileEnv = false
			}
			if strings.TrimSpace(line) == "---env" {
				inFileEnv = true
				continue
			}
			lines = append(lines, line)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading bundle: %w", err)
	}

	flush()

	active := bundle.Files[:0]
	for _, f := range bundle.Files {
		if !f.Disabled {
			active = append(active, f)
		}
	}
	bundle.Files = active

	if len(bundle.Files) == 0 {
		return nil, fmt.Errorf("no active files found in bundle %q", source)
	}

	return bundle, nil
}

func parseFileHeader(raw string) (string, map[string]string) {
	attrs := make(map[string]string)
	parts := strings.Fields(raw)
	if len(parts) == 0 {
		return raw, attrs
	}
	name := parts[0]
	for _, part := range parts[1:] {
		kv := strings.SplitN(part, "=", 2)
		if len(kv) == 2 {
			attrs[strings.ToLower(kv[0])] = kv[1]
		} else {
			attrs[strings.ToLower(kv[0])] = "true"
		}
	}
	return name, attrs
}

func applyFileAttrs(f *File, attrs map[string]string) {
	if v, ok := attrs["after"]; ok {
		f.RunAfter = strings.Split(v, ",")
	}
	if v, ok := attrs["tags"]; ok {
		f.Tags = strings.Split(v, ",")
	}
	if v, ok := attrs["args"]; ok {
		f.Args = splitArgs(v)
	}
	if v, ok := attrs["stdin"]; ok {
		f.Stdin = v
	}
	if v, ok := attrs["desc"]; ok {
		f.Description = v
	}
	if _, ok := attrs["disabled"]; ok {
		f.Disabled = true
	}
	if _, ok := attrs["hidden"]; ok {
		f.Hidden = true
	}
	if v, ok := attrs["lang"]; ok {
		f.Lang = v
	}
}

func applyBundleMeta(b *Bundle, key, val string) {
	switch key {
	case "entrypoint", "entry", "main":
		b.Meta.Entrypoint = val
	case "name":
		b.Meta.Name = val
	case "version":
		b.Meta.Version = val
	case "description", "desc":
		b.Meta.Description = val
	case "parallel":
		b.Meta.Parallel = val == "true" || val == "1" || val == ""
	case "stop-on-fail", "stoponfail":
		b.Meta.StopOnFail = val == "true" || val == "1" || val == ""
	case "pipeline":
		b.Meta.Pipeline = strings.Split(val, ",")
	case "watch":
		b.Meta.Watch = strings.Split(val, ",")
	case "shell":
		b.Meta.Shell = val
	default:
		if envLineRegex.MatchString(strings.ToUpper(key) + "=" + val) {
			b.Meta.Env[strings.ToUpper(key)] = val
		}
	}
}

func DetectLang(filename string) string {
	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".py", ".pyw":
		return "python"
	case ".js", ".mjs", ".cjs":
		return "javascript"
	case ".ts", ".mts":
		return "typescript"
	case ".tsx":
		return "tsx"
	case ".jsx":
		return "jsx"
	case ".rb":
		return "ruby"
	case ".sh", ".bash":
		return "shell"
	case ".zsh":
		return "zsh"
	case ".go":
		return "go"
	case ".rs":
		return "rust"
	case ".java":
		return "java"
	case ".c":
		return "c"
	case ".cpp", ".cc", ".cxx":
		return "cpp"
	case ".h", ".hpp":
		return "header"
	case ".php":
		return "php"
	case ".lua":
		return "lua"
	case ".r":
		return "r"
	case ".pl":
		return "perl"
	case ".swift":
		return "swift"
	case ".kt", ".kts":
		return "kotlin"
	case ".cs":
		return "csharp"
	case ".ex":
		return "elixir"
	case ".exs":
		return "elixir-script"
	case ".hs":
		return "haskell"
	case ".clj":
		return "clojure"
	case ".ml":
		return "ocaml"
	case ".scala":
		return "scala"
	case ".zig":
		return "zig"
	case ".nim":
		return "nim"
	case ".dart":
		return "dart"
	case ".jl":
		return "julia"
	case ".ps1":
		return "powershell"
	case ".bat", ".cmd":
		return "batch"
	case ".html", ".htm":
		return "html"
	case ".css":
		return "css"
	case ".json":
		return "json"
	case ".yaml", ".yml":
		return "yaml"
	case ".toml":
		return "toml"
	case ".xml":
		return "xml"
	case ".md", ".markdown":
		return "markdown"
	case ".sql":
		return "sql"
	case ".txt":
		return "text"
	case ".env":
		return "dotenv"
	case ".graphql", ".gql":
		return "graphql"
	case ".proto":
		return "proto"
	case ".tf":
		return "terraform"
	case ".dockerfile":
		return "dockerfile"
	}
	base := strings.ToLower(filepath.Base(filename))
	if base == "dockerfile" {
		return "dockerfile"
	}
	if base == "makefile" || base == "gnumakefile" {
		return "makefile"
	}
	return "text"
}

func splitArgs(s string) []string {
	var out []string
	var cur strings.Builder
	inQuote := false
	quoteChar := byte(0)
	for i := 0; i < len(s); i++ {
		c := s[i]
		if inQuote {
			if c == quoteChar {
				inQuote = false
			} else {
				cur.WriteByte(c)
			}
		} else if c == '"' || c == '\'' {
			inQuote = true
			quoteChar = c
		} else if c == ' ' || c == '\t' {
			if cur.Len() > 0 {
				out = append(out, cur.String())
				cur.Reset()
			}
		} else {
			cur.WriteByte(c)
		}
	}
	if cur.Len() > 0 {
		out = append(out, cur.String())
	}
	return out
}
