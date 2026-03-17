package runner

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/fog-lang/fog/internal/bridge"
	"github.com/fog-lang/fog/internal/lang"
	"github.com/fog-lang/fog/internal/parser"
)

type Options struct {
	Stdout      io.Writer
	Stderr      io.Writer
	Stdin       io.Reader
	Args        []string
	Env         []string
	EntryOverride string
	DryRun      bool
	Verbose     bool
	Tags        []string
	Pipeline    []string
	Parallel    bool
	StopOnFail  bool
	WorkDir     string
	NoBridge    bool
	Timeout     time.Duration
	KeepTemp    bool
}

type Result struct {
	File     string
	Lang     string
	ExitCode int
	Duration time.Duration
	Error    error
	Stdout   string
	Stderr   string
	Skipped  bool
}

type RunSummary struct {
	Results  []*Result
	Total    int
	Passed   int
	Failed   int
	Skipped  int
	Duration time.Duration
}

func Run(bundle *parser.Bundle, opts *Options) (*RunSummary, error) {
	if opts.Stdout == nil {
		opts.Stdout = os.Stdout
	}
	if opts.Stderr == nil {
		opts.Stderr = os.Stderr
	}
	if opts.Stdin == nil {
		opts.Stdin = os.Stdin
	}

	tmpDir, err := os.MkdirTemp("", "fog-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create workspace: %w", err)
	}
	if !opts.KeepTemp {
		defer os.RemoveAll(tmpDir)
	} else {
		fmt.Fprintf(opts.Stderr, "[fog] workspace: %s\n", tmpDir)
	}

	br := bridge.New(tmpDir)
	if err := br.Init(); err != nil {
		return nil, fmt.Errorf("bridge init failed: %w", err)
	}

	for _, f := range bundle.Files {
		dest := filepath.Join(tmpDir, f.Name)
		if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
			return nil, fmt.Errorf("mkdir for %s: %w", f.Name, err)
		}
		content := injectBridgeHelpers(f, br, opts)
		if err := os.WriteFile(dest, content, 0644); err != nil {
			return nil, fmt.Errorf("write %s: %w", f.Name, err)
		}
	}

	if err := writeSharedAssets(bundle, tmpDir); err != nil {
		return nil, err
	}

	files, err := selectFiles(bundle, opts)
	if err != nil {
		return nil, err
	}

	if len(files) == 0 {
		return nil, fmt.Errorf("no matching files to run")
	}

	summary := &RunSummary{}
	start := time.Now()

	if isPipeline(bundle, opts) {
		err = runPipeline(files, bundle, tmpDir, br, opts, summary)
	} else if isParallel(bundle, opts) {
		err = runParallel(files, tmpDir, br, opts, summary)
	} else {
		err = runSequential(files, tmpDir, br, opts, summary)
	}

	summary.Duration = time.Since(start)
	summary.Total = len(files)
	for _, r := range summary.Results {
		if r.Skipped {
			summary.Skipped++
		} else if r.ExitCode == 0 && r.Error == nil {
			summary.Passed++
		} else {
			summary.Failed++
		}
	}

	return summary, err
}

func runSequential(files []*parser.File, tmpDir string, br *bridge.Exchange, opts *Options, summary *RunSummary) error {
	for _, f := range files {
		r := executeFile(f, tmpDir, br, opts, nil)
		summary.Results = append(summary.Results, r)
		if (r.Error != nil || r.ExitCode != 0) && opts.StopOnFail {
			return fmt.Errorf("stopped after failure in %s", f.Name)
		}
	}
	return nil
}

func runParallel(files []*parser.File, tmpDir string, br *bridge.Exchange, opts *Options, summary *RunSummary) error {
	results := make([]*Result, len(files))
	var wg sync.WaitGroup
	for i, f := range files {
		wg.Add(1)
		go func(idx int, file *parser.File) {
			defer wg.Done()
			var outBuf, errBuf bytes.Buffer
			pOpts := *opts
			pOpts.Stdout = io.MultiWriter(opts.Stdout, &outBuf)
			pOpts.Stderr = io.MultiWriter(opts.Stderr, &errBuf)
			r := executeFile(file, tmpDir, br, &pOpts, nil)
			r.Stdout = outBuf.String()
			r.Stderr = errBuf.String()
			results[idx] = r
		}(i, f)
	}
	wg.Wait()
	summary.Results = append(summary.Results, results...)
	return nil
}

func runPipeline(files []*parser.File, bundle *parser.Bundle, tmpDir string, br *bridge.Exchange, opts *Options, summary *RunSummary) error {
	order := bundle.Meta.Pipeline
	if len(opts.Pipeline) > 0 {
		order = opts.Pipeline
	}

	byName := make(map[string]*parser.File)
	for _, f := range files {
		byName[f.Name] = f
	}

	var pipeOutput []byte
	for i, name := range order {
		f, ok := byName[name]
		if !ok {
			return fmt.Errorf("pipeline: file %q not found in bundle", name)
		}

		pOpts := *opts
		var stdinReader io.Reader = opts.Stdin
		if i > 0 && pipeOutput != nil {
			stdinReader = bytes.NewReader(pipeOutput)
		}
		pOpts.Stdin = stdinReader

		var outBuf bytes.Buffer
		pOpts.Stdout = io.MultiWriter(opts.Stdout, &outBuf)

		r := executeFile(f, tmpDir, br, &pOpts, nil)
		summary.Results = append(summary.Results, r)
		pipeOutput = outBuf.Bytes()

		if (r.Error != nil || r.ExitCode != 0) && opts.StopOnFail {
			return fmt.Errorf("pipeline stopped at %s (exit %d)", f.Name, r.ExitCode)
		}
	}
	return nil
}

func executeFile(f *parser.File, tmpDir string, br *bridge.Exchange, opts *Options, stdinOverride io.Reader) *Result {
	start := time.Now()
	result := &Result{File: f.Name, Lang: f.Lang}

	runtime, err := lang.Get(f.Lang)
	if err != nil {
		result.Error = err
		return result
	}

	if runtime.Mode == lang.DataOnly {
		result.Skipped = true
		return result
	}

	if err := lang.CheckAvailable(f.Lang); err != nil {
		result.Error = err
		return result
	}

	filePath := filepath.Join(tmpDir, f.Name)
	outBin := filepath.Join(tmpDir, "_fog_bin_"+sanitizeName(f.Name))

	if runtime.Mode == lang.Compiled && runtime.CompileCmd != nil {
		compileArgs := runtime.CompileCmd(tmpDir, filePath, outBin)
		if opts.Verbose {
			fmt.Fprintf(opts.Stderr, "[fog] [%s] compile: %s\n", f.Name, strings.Join(compileArgs, " "))
		}
		if !opts.DryRun {
			if err := runCmd(compileArgs, tmpDir, opts, opts.Stdin, nil); err != nil {
				result.Error = fmt.Errorf("compile failed: %w", err)
				if exit, ok := err.(*exec.ExitError); ok {
					result.ExitCode = exit.ExitCode()
				}
				result.Duration = time.Since(start)
				return result
			}
		}
	}

	runArgs := runtime.RunCmd(tmpDir, filePath, outBin)
	runArgs = append(runArgs, f.Args...)
	runArgs = append(runArgs, opts.Args...)

	if opts.Verbose {
		fmt.Fprintf(opts.Stderr, "[fog] [%s] run: %s\n", f.Name, strings.Join(runArgs, " "))
	}

	if opts.DryRun {
		fmt.Fprintf(opts.Stdout, "[fog:dry-run] %s -> %s\n", f.Name, strings.Join(runArgs, " "))
		result.Duration = time.Since(start)
		return result
	}

	stdin := opts.Stdin
	if stdinOverride != nil {
		stdin = stdinOverride
	}
	if f.Stdin != "" {
		stdinFile := filepath.Join(tmpDir, f.Stdin)
		if sf, err := os.Open(stdinFile); err == nil {
			stdin = sf
			defer sf.Close()
		}
	}

	extraEnv := mergeEnv(opts.Env, f.Env, br.CollectOutputEnv())

	if err := runCmdWithEnv(runArgs, tmpDir, opts, stdin, extraEnv); err != nil {
		result.Error = err
		if exit, ok := err.(*exec.ExitError); ok {
			result.ExitCode = exit.ExitCode()
			result.Error = nil
		}
	}

	result.Duration = time.Since(start)
	return result
}

func runCmd(args []string, dir string, opts *Options, stdin io.Reader, extraEnv []string) error {
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Dir = dir
	cmd.Stdout = opts.Stdout
	cmd.Stderr = opts.Stderr
	cmd.Stdin = stdin
	if extraEnv != nil {
		cmd.Env = append(os.Environ(), extraEnv...)
	}
	return cmd.Run()
}

func runCmdWithEnv(args []string, dir string, opts *Options, stdin io.Reader, extraEnv []string) error {
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Dir = dir
	cmd.Stdout = opts.Stdout
	cmd.Stderr = opts.Stderr
	cmd.Stdin = stdin
	cmd.Env = append(os.Environ(), extraEnv...)
	if len(opts.Env) > 0 {
		cmd.Env = append(cmd.Env, opts.Env...)
	}
	return cmd.Run()
}

func selectFiles(bundle *parser.Bundle, opts *Options) ([]*parser.File, error) {
	if len(opts.Tags) > 0 {
		return filterByTags(bundle.Files, opts.Tags), nil
	}

	if len(opts.Pipeline) > 0 {
		return bundle.Files, nil
	}

	if bundle.Meta.Entrypoint != "" && !isParallel(bundle, opts) {
		return resolveEntry(bundle, bundle.Meta.Entrypoint)
	}

	if opts.EntryOverride != "" {
		return resolveEntry(bundle, opts.EntryOverride)
	}

	if isParallel(bundle, opts) || len(bundle.Meta.Pipeline) > 0 {
		return bundle.Files, nil
	}

	if len(bundle.Files) == 1 {
		return bundle.Files, nil
	}

	if main := findMain(bundle.Files); main != nil {
		return []*parser.File{main}, nil
	}

	return bundle.Files[:1], nil
}

func resolveEntry(bundle *parser.Bundle, name string) ([]*parser.File, error) {
	for _, f := range bundle.Files {
		if f.Name == name {
			return []*parser.File{f}, nil
		}
	}
	return nil, fmt.Errorf("entrypoint %q not found in bundle", name)
}

func filterByTags(files []*parser.File, tags []string) []*parser.File {
	tagSet := make(map[string]bool)
	for _, t := range tags {
		tagSet[t] = true
	}
	var out []*parser.File
	for _, f := range files {
		for _, t := range f.Tags {
			if tagSet[t] {
				out = append(out, f)
				break
			}
		}
	}
	return out
}

func findMain(files []*parser.File) *parser.File {
	priorities := []string{
		"main.py", "main.js", "main.ts", "main.go", "main.rs", "main.c",
		"main.cpp", "main.rb", "main.sh", "main.lua", "main.java",
		"index.js", "index.ts", "index.py",
		"app.py", "app.js", "app.ts",
		"run.py", "run.js", "run.sh",
	}
	byBase := make(map[string]*parser.File)
	for _, f := range files {
		byBase[strings.ToLower(filepath.Base(f.Name))] = f
	}
	for _, p := range priorities {
		if f, ok := byBase[p]; ok {
			return f
		}
	}
	sorted := make([]*parser.File, len(files))
	copy(sorted, files)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Name < sorted[j].Name })
	return nil
}

func injectBridgeHelpers(f *parser.File, br *bridge.Exchange, opts *Options) []byte {
	if opts.NoBridge {
		return f.Content
	}
	lines := br.InjectEnvHelpers(f.Lang)
	if len(lines) == 0 {
		return f.Content
	}
	var sb strings.Builder
	for _, l := range lines {
		sb.WriteString(l)
		sb.WriteByte('\n')
	}
	sb.Write(f.Content)
	return []byte(sb.String())
}

func writeSharedAssets(bundle *parser.Bundle, tmpDir string) error {
	for _, f := range bundle.Files {
		if f.Lang == "css" || f.Lang == "html" || f.Lang == "json" ||
			f.Lang == "yaml" || f.Lang == "text" || f.Lang == "markdown" ||
			f.Lang == "dotenv" || f.Lang == "xml" || f.Lang == "proto" {
			dest := filepath.Join(tmpDir, f.Name)
			if _, err := os.Stat(dest); err == nil {
				continue
			}
			if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
				return err
			}
			if err := os.WriteFile(dest, f.Content, 0644); err != nil {
				return err
			}
		}
	}
	return nil
}

func mergeEnv(globalEnv []string, fileEnv map[string]string, bridgeEnv map[string]string) []string {
	var out []string
	out = append(out, globalEnv...)
	for k, v := range bridgeEnv {
		out = append(out, k+"="+v)
	}
	for k, v := range fileEnv {
		out = append(out, k+"="+v)
	}
	return out
}

func isParallel(bundle *parser.Bundle, opts *Options) bool {
	return bundle.Meta.Parallel || opts.Parallel
}

func isPipeline(bundle *parser.Bundle, opts *Options) bool {
	return len(bundle.Meta.Pipeline) > 0 || len(opts.Pipeline) > 0
}

func sanitizeName(name string) string {
	return strings.NewReplacer("/", "_", "\\", "_", ".", "_", " ", "_").Replace(name)
}
