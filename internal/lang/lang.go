package lang

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

type ExecMode int

const (
	Interpreted ExecMode = iota
	Compiled
	DataOnly
	BrowserOpen
	ServerSide
)

type Runtime struct {
	Lang        string
	Mode        ExecMode
	Extensions  []string
	CompileCmd  func(srcDir, file, outBin string) []string
	RunCmd      func(srcDir, file, outBin string) []string
	CheckBin    string
}

var registry = map[string]*Runtime{
	"python": {
		Lang: "python", Mode: Interpreted,
		CheckBin: "python3",
		RunCmd: func(_, file, _ string) []string {
			return []string{pythonBin(), "-u", file}
		},
	},
	"javascript": {
		Lang: "javascript", Mode: Interpreted,
		CheckBin: "node",
		RunCmd: func(_, file, _ string) []string {
			return []string{"node", file}
		},
	},
	"typescript": {
		Lang: "typescript", Mode: Interpreted,
		CheckBin: "npx",
		RunCmd: func(_, file, _ string) []string {
			return []string{"npx", "--yes", "ts-node", file}
		},
	},
	"tsx": {
		Lang: "tsx", Mode: Interpreted,
		CheckBin: "npx",
		RunCmd: func(_, file, _ string) []string {
			return []string{"npx", "--yes", "ts-node", "--esm", file}
		},
	},
	"jsx": {
		Lang: "jsx", Mode: Interpreted,
		CheckBin: "node",
		RunCmd: func(_, file, _ string) []string {
			return []string{"node", "--experimental-vm-modules", file}
		},
	},
	"ruby": {
		Lang: "ruby", Mode: Interpreted,
		CheckBin: "ruby",
		RunCmd: func(_, file, _ string) []string {
			return []string{"ruby", file}
		},
	},
	"shell": {
		Lang: "shell", Mode: Interpreted,
		CheckBin: "bash",
		RunCmd: func(_, file, _ string) []string {
			return []string{"bash", file}
		},
	},
	"zsh": {
		Lang: "zsh", Mode: Interpreted,
		CheckBin: "zsh",
		RunCmd: func(_, file, _ string) []string {
			return []string{"zsh", file}
		},
	},
	"php": {
		Lang: "php", Mode: Interpreted,
		CheckBin: "php",
		RunCmd: func(_, file, _ string) []string {
			return []string{"php", file}
		},
	},
	"lua": {
		Lang: "lua", Mode: Interpreted,
		CheckBin: "lua",
		RunCmd: func(_, file, _ string) []string {
			return []string{"lua", file}
		},
	},
	"r": {
		Lang: "r", Mode: Interpreted,
		CheckBin: "Rscript",
		RunCmd: func(_, file, _ string) []string {
			return []string{"Rscript", file}
		},
	},
	"perl": {
		Lang: "perl", Mode: Interpreted,
		CheckBin: "perl",
		RunCmd: func(_, file, _ string) []string {
			return []string{"perl", file}
		},
	},
	"swift": {
		Lang: "swift", Mode: Interpreted,
		CheckBin: "swift",
		RunCmd: func(_, file, _ string) []string {
			return []string{"swift", file}
		},
	},
	"dart": {
		Lang: "dart", Mode: Interpreted,
		CheckBin: "dart",
		RunCmd: func(_, file, _ string) []string {
			return []string{"dart", "run", file}
		},
	},
	"elixir": {
		Lang: "elixir", Mode: Interpreted,
		CheckBin: "elixir",
		RunCmd: func(_, file, _ string) []string {
			return []string{"elixir", file}
		},
	},
	"elixir-script": {
		Lang: "elixir-script", Mode: Interpreted,
		CheckBin: "elixir",
		RunCmd: func(_, file, _ string) []string {
			return []string{"elixir", file}
		},
	},
	"julia": {
		Lang: "julia", Mode: Interpreted,
		CheckBin: "julia",
		RunCmd: func(_, file, _ string) []string {
			return []string{"julia", file}
		},
	},
	"powershell": {
		Lang: "powershell", Mode: Interpreted,
		CheckBin: "pwsh",
		RunCmd: func(_, file, _ string) []string {
			return []string{"pwsh", "-NonInteractive", "-File", file}
		},
	},
	"clojure": {
		Lang: "clojure", Mode: Interpreted,
		CheckBin: "clojure",
		RunCmd: func(_, file, _ string) []string {
			return []string{"clojure", file}
		},
	},
	"haskell": {
		Lang: "haskell", Mode: Interpreted,
		CheckBin: "runghc",
		RunCmd: func(_, file, _ string) []string {
			return []string{"runghc", file}
		},
	},
	"kotlin": {
		Lang: "kotlin", Mode: Interpreted,
		CheckBin: "kotlinc",
		RunCmd: func(_, file, _ string) []string {
			return []string{"kotlinc", "-script", file}
		},
	},
	"scala": {
		Lang: "scala", Mode: Interpreted,
		CheckBin: "scala",
		RunCmd: func(_, file, _ string) []string {
			return []string{"scala", file}
		},
	},
	"go": {
		Lang: "go", Mode: Compiled,
		CheckBin: "go",
		CompileCmd: func(srcDir, _, outBin string) []string {
			return []string{"go", "build", "-o", outBin, "."}
		},
		RunCmd: func(_, _, outBin string) []string {
			return []string{outBin}
		},
	},
	"rust": {
		Lang: "rust", Mode: Compiled,
		CheckBin: "rustc",
		CompileCmd: func(_, file, outBin string) []string {
			return []string{"rustc", file, "-o", outBin}
		},
		RunCmd: func(_, _, outBin string) []string {
			return []string{outBin}
		},
	},
	"c": {
		Lang: "c", Mode: Compiled,
		CheckBin: "gcc",
		CompileCmd: func(srcDir, _, outBin string) []string {
			return []string{"gcc", "-o", outBin, "-I", srcDir, srcDir + "/*.c"}
		},
		RunCmd: func(_, _, outBin string) []string {
			return []string{outBin}
		},
	},
	"cpp": {
		Lang: "cpp", Mode: Compiled,
		CheckBin: "g++",
		CompileCmd: func(srcDir, file, outBin string) []string {
			return []string{"g++", "-std=c++17", "-o", outBin, file}
		},
		RunCmd: func(_, _, outBin string) []string {
			return []string{outBin}
		},
	},
	"java": {
		Lang: "java", Mode: Compiled,
		CheckBin: "javac",
		CompileCmd: func(srcDir, file, _ string) []string {
			return []string{"javac", "-d", srcDir, file}
		},
		RunCmd: func(srcDir, file, _ string) []string {
			return []string{"java", "-cp", srcDir, javaClassName(file)}
		},
	},
	"csharp": {
		Lang: "csharp", Mode: Compiled,
		CheckBin: "dotnet",
		CompileCmd: func(srcDir, file, outBin string) []string {
			return []string{"dotnet", "script", file}
		},
		RunCmd: func(srcDir, file, outBin string) []string {
			return []string{"dotnet", "script", file}
		},
	},
	"nim": {
		Lang: "nim", Mode: Compiled,
		CheckBin: "nim",
		CompileCmd: func(_, file, outBin string) []string {
			return []string{"nim", "compile", "--out:" + outBin, file}
		},
		RunCmd: func(_, _, outBin string) []string {
			return []string{outBin}
		},
	},
	"zig": {
		Lang: "zig", Mode: Compiled,
		CheckBin: "zig",
		CompileCmd: func(_, file, outBin string) []string {
			return []string{"zig", "build-exe", file, "-femit-bin=" + outBin}
		},
		RunCmd: func(_, _, outBin string) []string {
			return []string{outBin}
		},
	},
	"html": {
		Lang: "html", Mode: BrowserOpen,
		RunCmd: func(_, file, _ string) []string {
			return browserOpenCmd(file)
		},
	},
	"sql": {
		Lang: "sql", Mode: Interpreted,
		CheckBin: "sqlite3",
		RunCmd: func(_, file, _ string) []string {
			return []string{"sqlite3", ":memory:", ".read " + file}
		},
	},
	"makefile": {
		Lang: "makefile", Mode: Interpreted,
		CheckBin: "make",
		RunCmd: func(srcDir, _, _ string) []string {
			return []string{"make", "-C", srcDir}
		},
	},
	"dockerfile": {
		Lang: "dockerfile", Mode: DataOnly,
	},
	"json":      {Lang: "json", Mode: DataOnly},
	"yaml":      {Lang: "yaml", Mode: DataOnly},
	"toml":      {Lang: "toml", Mode: DataOnly},
	"xml":       {Lang: "xml", Mode: DataOnly},
	"markdown":  {Lang: "markdown", Mode: DataOnly},
	"text":      {Lang: "text", Mode: DataOnly},
	"dotenv":    {Lang: "dotenv", Mode: DataOnly},
	"graphql":   {Lang: "graphql", Mode: DataOnly},
	"proto":     {Lang: "proto", Mode: DataOnly},
	"terraform": {Lang: "terraform", Mode: DataOnly},
	"css":       {Lang: "css", Mode: DataOnly},
	"header":    {Lang: "header", Mode: DataOnly},
}

func Get(l string) (*Runtime, error) {
	r, ok := registry[l]
	if !ok {
		return nil, fmt.Errorf("no runtime for language %q — see 'fog langs'", l)
	}
	return r, nil
}

func IsRunnable(l string) bool {
	r, ok := registry[l]
	if !ok {
		return false
	}
	return r.Mode != DataOnly
}

func CheckAvailable(l string) error {
	r, ok := registry[l]
	if !ok {
		return fmt.Errorf("unknown language %q", l)
	}
	if r.CheckBin == "" {
		return nil
	}
	if _, err := exec.LookPath(r.CheckBin); err != nil {
		return fmt.Errorf("runtime not found: %q is required for language %q", r.CheckBin, l)
	}
	return nil
}

func ListAll() []*Runtime {
	out := make([]*Runtime, 0, len(registry))
	for _, r := range registry {
		out = append(out, r)
	}
	return out
}

func pythonBin() string {
	if _, err := exec.LookPath("python3"); err == nil {
		return "python3"
	}
	return "python"
}

func javaClassName(file string) string {
	base := filepath.Base(file)
	return strings.TrimSuffix(base, filepath.Ext(base))
}

func browserOpenCmd(file string) []string {
	for _, bin := range []string{"xdg-open", "open", "start"} {
		if _, err := exec.LookPath(bin); err == nil {
			return []string{bin, file}
		}
	}
	return []string{"xdg-open", file}
}
