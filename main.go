package main

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/fog-lang/fog/internal/bundler"
	"github.com/fog-lang/fog/internal/lang"
	"github.com/fog-lang/fog/internal/parser"
	"github.com/fog-lang/fog/internal/runner"
	"github.com/fog-lang/fog/internal/watcher"
)

const version = "2.0.0"

const helpText = `fog — multi-language bundle runner

Usage:
  fog run    [options] <bundle>  [-- script-args...]
  fog pack   [options] <inputs...> -o <output>
  fog extract [options] <bundle>
  fog inspect [options] <bundle>
  fog diff   <bundle-a> <bundle-b>
  fog validate <bundle>
  fog new    <name> [template]
  fog langs
  fog version

Run options:
  --entry <file>          Override entrypoint
  --tag <tag>             Run only files with tag (repeatable)
  --pipeline <a,b,c>      Run files as pipeline (stdout piped as stdin)
  --parallel              Run all files in parallel
  --env KEY=VAL           Set environment variable (repeatable)
  --watch                 Re-run on file change
  --watch-interval <ms>   Watch poll interval (default: 500)
  --stop-on-fail          Stop on first failure
  --no-bridge             Disable cross-language bridge helpers
  --keep-temp             Keep temp workspace (prints path)
  --timeout <s>           Execution timeout in seconds
  --verbose               Show execution details
  --dry-run               Show what would run

Pack options:
  -o <file>               Output file (required)
  --entry <file>          Set entrypoint metadata
  --name <name>           Bundle name
  --version <ver>         Bundle version
  --desc <text>           Bundle description
  --parallel              Mark bundle as parallel
  --pipeline <a,b,c>      Set pipeline order
  --strip-empty           Skip empty files

Extract options:
  -o <dir>                Output directory (default: .)
  --overwrite             Overwrite existing files
  --file <name>           Extract only this file (repeatable)
  --flat                  Ignore subdirectory structure

Inspect options:
  --verbose               Show tags and per-file env vars

Templates (fog new):
  python     Python multi-file starter
  web        HTML + CSS + JS bundle
  fullstack  Python server + HTML/CSS/JS frontend
  pipeline   Multi-language data pipeline
  polyglot   Mixed-language demo
`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, helpText)
		os.Exit(1)
	}

	cmd := os.Args[1]
	args := os.Args[2:]

	var err error
	switch cmd {
	case "run":
		err = cmdRun(args)
	case "pack":
		err = cmdPack(args)
	case "extract":
		err = cmdExtract(args)
	case "inspect":
		err = cmdInspect(args)
	case "diff":
		err = cmdDiff(args)
	case "validate":
		err = cmdValidate(args)
	case "new":
		err = cmdNew(args)
	case "langs":
		err = cmdLangs()
	case "version", "--version", "-V":
		fmt.Printf("fog %s\n", version)
	case "help", "--help", "-h":
		fmt.Print(helpText)
	default:
		fmt.Fprintf(os.Stderr, "fog: unknown command %q\n\nRun 'fog help' for usage.\n", cmd)
		os.Exit(1)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "fog: %v\n", err)
		os.Exit(1)
	}
}

func cmdRun(args []string) error {
	opts := &runner.Options{
		Stdout: os.Stdout,
		Stderr: os.Stderr,
		Stdin:  os.Stdin,
	}

	var bundleFile string
	var watchMode bool
	var watchInterval int = 500
	var passthroughArgs []string
	var tags []string
	var pipeline []string
	var timeoutSec int

	i := 0
	for i < len(args) {
		a := args[i]
		if a == "--" {
			passthroughArgs = args[i+1:]
			break
		}
		switch a {
		case "--verbose", "-v":
			opts.Verbose = true
		case "--dry-run":
			opts.DryRun = true
		case "--parallel":
			opts.Parallel = true
		case "--stop-on-fail":
			opts.StopOnFail = true
		case "--no-bridge":
			opts.NoBridge = true
		case "--keep-temp":
			opts.KeepTemp = true
		case "--watch":
			watchMode = true
		case "--watch-interval":
			i++
			if i >= len(args) {
				return fmt.Errorf("--watch-interval requires a value")
			}
			fmt.Sscanf(args[i], "%d", &watchInterval)
		case "--entry":
			i++
			if i >= len(args) {
				return fmt.Errorf("--entry requires a value")
			}
			opts.EntryOverride = args[i]
		case "--tag":
			i++
			if i >= len(args) {
				return fmt.Errorf("--tag requires a value")
			}
			tags = append(tags, args[i])
		case "--pipeline":
			i++
			if i >= len(args) {
				return fmt.Errorf("--pipeline requires a value")
			}
			pipeline = strings.Split(args[i], ",")
		case "--env":
			i++
			if i >= len(args) {
				return fmt.Errorf("--env requires KEY=VALUE")
			}
			opts.Env = append(opts.Env, args[i])
		case "--timeout":
			i++
			if i >= len(args) {
				return fmt.Errorf("--timeout requires a value in seconds")
			}
			fmt.Sscanf(args[i], "%d", &timeoutSec)
		default:
			if strings.HasPrefix(a, "--env=") {
				opts.Env = append(opts.Env, strings.TrimPrefix(a, "--env="))
			} else if bundleFile == "" && !strings.HasPrefix(a, "--") {
				bundleFile = a
			} else {
				passthroughArgs = append(passthroughArgs, a)
			}
		}
		i++
	}

	if bundleFile == "" {
		return fmt.Errorf("run requires a bundle file\nUsage: fog run <bundle> [options]")
	}

	opts.Args = passthroughArgs
	opts.Tags = tags
	opts.Pipeline = pipeline
	if timeoutSec > 0 {
		opts.Timeout = time.Duration(timeoutSec) * time.Second
	}

	if watchMode {
		return runWatch(bundleFile, opts, time.Duration(watchInterval)*time.Millisecond)
	}

	return executeBundle(bundleFile, opts)
}

func executeBundle(bundleFile string, opts *runner.Options) error {
	bundle, err := parser.ParseFile(bundleFile)
	if err != nil {
		return err
	}

	summary, err := runner.Run(bundle, opts)
	if err != nil {
		return err
	}

	if opts.Verbose && summary != nil {
		printSummary(summary, opts)
	}

	if summary != nil {
		for _, r := range summary.Results {
			if r.ExitCode != 0 {
				os.Exit(r.ExitCode)
			}
			if r.Error != nil {
				return r.Error
			}
		}
	}

	return nil
}

func runWatch(bundleFile string, opts *runner.Options, interval time.Duration) error {
	fmt.Fprintf(os.Stderr, "[fog] watching %s (interval: %v)\n", bundleFile, interval)
	if err := executeBundle(bundleFile, opts); err != nil {
		fmt.Fprintf(os.Stderr, "[fog] error: %v\n", err)
	}
	w := watcher.New(bundleFile, interval, func() {
		fmt.Fprintf(os.Stderr, "\n[fog] change detected, re-running...\n\n")
		if err := executeBundle(bundleFile, opts); err != nil {
			fmt.Fprintf(os.Stderr, "[fog] error: %v\n", err)
		}
	})
	w.Start()
	return nil
}

func printSummary(s *runner.RunSummary, opts *runner.Options) {
	fmt.Fprintf(opts.Stderr, "\n[fog] summary: %d run, %d passed, %d failed, %d skipped (%v)\n",
		s.Total, s.Passed, s.Failed, s.Skipped, s.Duration.Round(time.Millisecond))
}

func cmdPack(args []string) error {
	opts := &bundler.PackOptions{}
	var inputs []string

	i := 0
	for i < len(args) {
		a := args[i]
		switch a {
		case "-o", "--output":
			i++
			if i >= len(args) {
				return fmt.Errorf("-o requires a value")
			}
			opts.Output = args[i]
		case "--entry":
			i++
			if i >= len(args) {
				return fmt.Errorf("--entry requires a value")
			}
			opts.Entrypoint = args[i]
		case "--name":
			i++
			opts.Name = args[i]
		case "--version":
			i++
			opts.Version = args[i]
		case "--desc":
			i++
			opts.Desc = args[i]
		case "--parallel":
			opts.Parallel = true
		case "--pipeline":
			i++
			if i >= len(args) {
				return fmt.Errorf("--pipeline requires a value")
			}
			opts.Pipeline = strings.Split(args[i], ",")
		case "--strip-empty":
			opts.StripEmpty = true
		default:
			if strings.HasPrefix(a, "-o=") {
				opts.Output = strings.TrimPrefix(a, "-o=")
			} else if !strings.HasPrefix(a, "-") {
				inputs = append(inputs, a)
			}
		}
		i++
	}

	if opts.Output == "" {
		return fmt.Errorf("pack requires -o <output>")
	}
	if len(inputs) == 0 {
		return fmt.Errorf("pack requires at least one input file or directory")
	}

	if err := bundler.Pack(inputs, opts); err != nil {
		return err
	}

	fmt.Printf("packed: %s\n", opts.Output)
	return nil
}

func cmdExtract(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("extract requires a bundle file")
	}

	opts := &bundler.ExtractOptions{}
	var bundleFile string

	i := 0
	for i < len(args) {
		a := args[i]
		switch a {
		case "-o", "--output":
			i++
			opts.Output = args[i]
		case "--overwrite":
			opts.Overwrite = true
		case "--flat":
			opts.Flat = true
		case "--file":
			i++
			opts.Files = append(opts.Files, args[i])
		default:
			if bundleFile == "" {
				bundleFile = a
			}
		}
		i++
	}

	if bundleFile == "" {
		return fmt.Errorf("extract requires a bundle file argument")
	}

	return bundler.Extract(bundleFile, opts)
}

func cmdInspect(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("inspect requires a bundle file")
	}
	verbose := false
	bundleFile := ""
	for _, a := range args {
		if a == "--verbose" || a == "-v" {
			verbose = true
		} else {
			bundleFile = a
		}
	}
	if bundleFile == "" {
		return fmt.Errorf("inspect requires a bundle file argument")
	}
	return bundler.Inspect(bundleFile, verbose)
}

func cmdDiff(args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("diff requires two bundle files: fog diff <a> <b>")
	}
	result, err := bundler.Diff(args[0], args[1])
	if err != nil {
		return err
	}

	if len(result.OnlyInA) == 0 && len(result.OnlyInB) == 0 && len(result.Changed) == 0 {
		fmt.Println("bundles are identical")
		return nil
	}

	for _, f := range result.OnlyInA {
		fmt.Printf("- %s\n", f)
	}
	for _, f := range result.OnlyInB {
		fmt.Printf("+ %s\n", f)
	}
	for _, f := range result.Changed {
		fmt.Printf("~ %s\n", f)
	}
	return nil
}

func cmdValidate(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("validate requires a bundle file")
	}
	return bundler.Validate(args[0])
}

func cmdNew(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("new requires a name: fog new <name> [template]")
	}
	name := args[0]
	template := "python"
	if len(args) > 1 {
		template = args[1]
	}

	templates := map[string]string{
		"python": pythonTemplate(name),
		"web":    webTemplate(name),
		"fullstack": fullstackTemplate(name),
		"pipeline": pipelineTemplate(name),
		"polyglot": polyglotTemplate(name),
	}

	content, ok := templates[template]
	if !ok {
		var names []string
		for k := range templates {
			names = append(names, k)
		}
		sort.Strings(names)
		return fmt.Errorf("unknown template %q; available: %s", template, strings.Join(names, ", "))
	}

	outFile := name + ".fog"
	if err := os.WriteFile(outFile, []byte(content), 0644); err != nil {
		return fmt.Errorf("write failed: %w", err)
	}
	fmt.Printf("created: %s (template: %s)\n", outFile, template)
	fmt.Printf("run with: fog run %s\n", outFile)
	return nil
}

func cmdLangs() error {
	runtimes := lang.ListAll()
	sort.Slice(runtimes, func(i, j int) bool {
		return runtimes[i].Lang < runtimes[j].Lang
	})
	fmt.Println("Supported languages:")
	for _, r := range runtimes {
		mode := "interpreted"
		switch r.Mode {
		case lang.Compiled:
			mode = "compiled"
		case lang.DataOnly:
			mode = "data/asset"
		case lang.BrowserOpen:
			mode = "browser"
		}
		bin := ""
		if r.CheckBin != "" {
			bin = " (requires: " + r.CheckBin + ")"
		}
		fmt.Printf("  %-18s  %-14s%s\n", r.Lang, mode, bin)
	}
	return nil
}

func pythonTemplate(name string) string {
	return fmt.Sprintf(`[@name=%s]
[@entrypoint=main.py]

[main.py]
from utils import greet, compute

result = compute(6, 7)
greet("World", result)

[utils.py]
def greet(name, value):
    print(f"Hello, {name}! The answer is {value}")

def compute(a, b):
    return a * b
`, name)
}

func webTemplate(name string) string {
	return fmt.Sprintf(`[@name=%s]
[@entrypoint=index.html]

[index.html]
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <title>%s</title>
    <link rel="stylesheet" href="style.css">
</head>
<body>
    <h1 id="title">Hello from fog!</h1>
    <p id="message"></p>
    <script src="app.js"></script>
</body>
</html>

[style.css]
body { font-family: sans-serif; max-width: 600px; margin: 40px auto; padding: 0 20px; }
h1 { color: #2563eb; }

[app.js]
document.getElementById('message').textContent = 'Built with fog — multi-language bundles';
`, name, name)
}

func fullstackTemplate(name string) string {
	return fmt.Sprintf(`[@name=%s]
[@entrypoint=server.py]

[server.py]
import http.server, os, json

class Handler(http.server.SimpleHTTPRequestHandler):
    def do_GET(self):
        if self.path == '/api/hello':
            self.send_response(200)
            self.send_header('Content-Type', 'application/json')
            self.end_headers()
            self.wfile.write(json.dumps({"message": "Hello from fog!"}).encode())
        else:
            super().do_GET()
    def log_message(self, *a): pass

os.chdir(os.path.dirname(os.path.abspath(__file__)))
print("Server: http://localhost:8080")
http.server.HTTPServer(('', 8080), Handler).serve_forever()

[index.html]
<!DOCTYPE html>
<html lang="en">
<head><meta charset="UTF-8"><title>%s</title><link rel="stylesheet" href="style.css"></head>
<body>
    <h1>fog fullstack</h1>
    <div id="result">Loading...</div>
    <script src="app.js"></script>
</body>
</html>

[style.css]
body { font-family: sans-serif; max-width: 600px; margin: 60px auto; text-align: center; }
#result { margin-top: 20px; font-size: 1.4em; color: #059669; }

[app.js]
fetch('/api/hello').then(r=>r.json()).then(d=>{ document.getElementById('result').textContent = d.message; });
`, name, name)
}

func pipelineTemplate(name string) string {
	return fmt.Sprintf(`[@name=%s]
[@pipeline=generate.py,process.js,report.sh]

[generate.py]
import json
data = [{"id": i, "value": i * i} for i in range(1, 6)]
print(json.dumps(data))

[process.js]
const chunks = [];
process.stdin.on('data', c => chunks.push(c));
process.stdin.on('end', () => {
    const data = JSON.parse(chunks.join(''));
    const total = data.reduce((s, x) => s + x.value, 0);
    process.stdout.write(JSON.stringify({ items: data.length, total }));
});

[report.sh]
read input
echo "Pipeline result: $input"
`, name)
}

func polyglotTemplate(name string) string {
	return fmt.Sprintf(`[@name=%s]
[@parallel]

[hello.py]
print("Python says: Hello!")

[hello.js]
console.log("JavaScript says: Hello!")

[hello.sh]
echo "Shell says: Hello!"
`, name)
}
