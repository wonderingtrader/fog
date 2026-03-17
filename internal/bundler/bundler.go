package bundler

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/fog-lang/fog/internal/parser"
)

type PackOptions struct {
	Output     string
	Entrypoint string
	StripEmpty bool
	Name       string
	Version    string
	Desc       string
	Parallel   bool
	Pipeline   []string
}

type ExtractOptions struct {
	Output    string
	Overwrite bool
	Files     []string
	Flat      bool
}

type DiffResult struct {
	OnlyInA  []string
	OnlyInB  []string
	Changed  []string
	Same     []string
}

func Pack(inputs []string, opts *PackOptions) error {
	var sb strings.Builder

	if opts.Name != "" {
		sb.WriteString(fmt.Sprintf("[@name=%s]\n", opts.Name))
	}
	if opts.Version != "" {
		sb.WriteString(fmt.Sprintf("[@version=%s]\n", opts.Version))
	}
	if opts.Desc != "" {
		sb.WriteString(fmt.Sprintf("[@desc=%s]\n", opts.Desc))
	}
	if opts.Entrypoint != "" {
		sb.WriteString(fmt.Sprintf("[@entrypoint=%s]\n", opts.Entrypoint))
	}
	if opts.Parallel {
		sb.WriteString("[@parallel]\n")
	}
	if len(opts.Pipeline) > 0 {
		sb.WriteString(fmt.Sprintf("[@pipeline=%s]\n", strings.Join(opts.Pipeline, ",")))
	}
	if sb.Len() > 0 {
		sb.WriteString("\n")
	}

	for _, path := range inputs {
		info, err := os.Stat(path)
		if err != nil {
			return fmt.Errorf("cannot stat %s: %w", path, err)
		}
		if info.IsDir() {
			if err := packDir(&sb, path, opts); err != nil {
				return err
			}
		} else {
			if err := packOneFile(&sb, path, filepath.Base(path), opts); err != nil {
				return err
			}
		}
	}

	return os.WriteFile(opts.Output, []byte(sb.String()), 0644)
}

func packDir(sb *strings.Builder, dir string, opts *PackOptions) error {
	var paths []string
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			if shouldSkipDir(filepath.Base(path)) {
				return filepath.SkipDir
			}
			return nil
		}
		if !shouldSkipFile(path) {
			paths = append(paths, path)
		}
		return nil
	})
	if err != nil {
		return err
	}
	sort.Strings(paths)
	for _, p := range paths {
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return err
		}
		if err := packOneFile(sb, p, rel, opts); err != nil {
			return err
		}
	}
	return nil
}

func packOneFile(sb *strings.Builder, path, name string, opts *PackOptions) error {
	lang := parser.DetectLang(name)
	if lang == "" {
		return nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("cannot read %s: %w", path, err)
	}

	content := string(data)
	if opts.StripEmpty && strings.TrimSpace(content) == "" {
		return nil
	}

	content = strings.TrimRight(content, "\n")
	sb.WriteString(fmt.Sprintf("[%s]\n%s\n\n", name, content))
	return nil
}

func Extract(bundlePath string, opts *ExtractOptions) error {
	bundle, err := parser.ParseFile(bundlePath)
	if err != nil {
		return err
	}

	outDir := opts.Output
	if outDir == "" {
		outDir = "."
	}

	filterSet := make(map[string]bool)
	for _, f := range opts.Files {
		filterSet[f] = true
	}

	for _, f := range bundle.Files {
		if len(filterSet) > 0 && !filterSet[f.Name] {
			continue
		}

		destName := f.Name
		if opts.Flat {
			destName = filepath.Base(f.Name)
		}
		dest := filepath.Join(outDir, destName)

		if !opts.Overwrite {
			if _, err := os.Stat(dest); err == nil {
				return fmt.Errorf("file %s exists; use --overwrite to replace", dest)
			}
		}

		if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
			return fmt.Errorf("mkdir for %s: %w", dest, err)
		}

		content := f.Content
		if len(content) > 0 && content[len(content)-1] != '\n' {
			content = append(content, '\n')
		}

		if err := os.WriteFile(dest, content, 0644); err != nil {
			return fmt.Errorf("write %s: %w", dest, err)
		}
		fmt.Printf("  extracted: %s\n", dest)
	}
	return nil
}

func Inspect(bundlePath string, verbose bool) error {
	bundle, err := parser.ParseFile(bundlePath)
	if err != nil {
		return err
	}

	meta := bundle.Meta
	fmt.Printf("Bundle:      %s\n", bundlePath)
	if meta.Name != "" {
		fmt.Printf("Name:        %s\n", meta.Name)
	}
	if meta.Version != "" {
		fmt.Printf("Version:     %s\n", meta.Version)
	}
	if meta.Description != "" {
		fmt.Printf("Description: %s\n", meta.Description)
	}
	if meta.Entrypoint != "" {
		fmt.Printf("Entrypoint:  %s\n", meta.Entrypoint)
	}
	if meta.Parallel {
		fmt.Printf("Parallel:    true\n")
	}
	if len(meta.Pipeline) > 0 {
		fmt.Printf("Pipeline:    %s\n", strings.Join(meta.Pipeline, " -> "))
	}
	if len(meta.Env) > 0 {
		fmt.Printf("Env vars:    %d\n", len(meta.Env))
	}
	fmt.Printf("Files:       %d\n\n", len(bundle.Files))

	langCounts := make(map[string]int)
	totalBytes := 0
	totalLines := 0

	for _, f := range bundle.Files {
		langCounts[f.Lang]++
		totalBytes += len(f.Content)
		lines := strings.Count(string(f.Content), "\n") + 1
		totalLines += lines
		hidden := ""
		if f.Hidden {
			hidden = " [hidden]"
		}
		desc := ""
		if f.Description != "" {
			desc = " - " + f.Description
		}
		fmt.Printf("  %-42s  %-16s  %5d lines  %7d bytes%s%s\n",
			f.Name, f.Lang, lines, len(f.Content), hidden, desc)
		if verbose && len(f.Tags) > 0 {
			fmt.Printf("    tags: %s\n", strings.Join(f.Tags, ", "))
		}
		if verbose && len(f.Env) > 0 {
			for k, v := range f.Env {
				fmt.Printf("    env:  %s=%s\n", k, v)
			}
		}
	}

	fmt.Printf("\nLanguages:\n")
	langs := make([]string, 0, len(langCounts))
	for l := range langCounts {
		langs = append(langs, l)
	}
	sort.Strings(langs)
	for _, l := range langs {
		fmt.Printf("  %-16s %d file(s)\n", l, langCounts[l])
	}
	fmt.Printf("\nTotal: %d lines, %d bytes\n", totalLines, totalBytes)
	return nil
}

func Diff(pathA, pathB string) (*DiffResult, error) {
	a, err := parser.ParseFile(pathA)
	if err != nil {
		return nil, fmt.Errorf("bundle A: %w", err)
	}
	b, err := parser.ParseFile(pathB)
	if err != nil {
		return nil, fmt.Errorf("bundle B: %w", err)
	}

	aMap := make(map[string][]byte)
	for _, f := range a.Files {
		aMap[f.Name] = f.Content
	}
	bMap := make(map[string][]byte)
	for _, f := range b.Files {
		bMap[f.Name] = f.Content
	}

	result := &DiffResult{}
	for name, ac := range aMap {
		bc, ok := bMap[name]
		if !ok {
			result.OnlyInA = append(result.OnlyInA, name)
		} else if string(ac) == string(bc) {
			result.Same = append(result.Same, name)
		} else {
			result.Changed = append(result.Changed, name)
		}
	}
	for name := range bMap {
		if _, ok := aMap[name]; !ok {
			result.OnlyInB = append(result.OnlyInB, name)
		}
	}
	return result, nil
}

func Validate(bundlePath string) error {
	bundle, err := parser.ParseFile(bundlePath)
	if err != nil {
		return fmt.Errorf("parse error: %w", err)
	}

	var errs []string
	seen := make(map[string]int)
	for i, f := range bundle.Files {
		if prev, ok := seen[f.Name]; ok {
			errs = append(errs, fmt.Sprintf("duplicate file name %q (files %d and %d)", f.Name, prev+1, i+1))
		}
		seen[f.Name] = i

		if f.Lang == "" {
			errs = append(errs, fmt.Sprintf("file %q has unknown language", f.Name))
		}
	}

	if ep := bundle.Meta.Entrypoint; ep != "" {
		found := false
		for _, f := range bundle.Files {
			if f.Name == ep {
				found = true
				break
			}
		}
		if !found {
			errs = append(errs, fmt.Sprintf("entrypoint %q not found in bundle", ep))
		}
	}

	for _, name := range bundle.Meta.Pipeline {
		found := false
		for _, f := range bundle.Files {
			if f.Name == name {
				found = true
				break
			}
		}
		if !found {
			errs = append(errs, fmt.Sprintf("pipeline step %q not found in bundle", name))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("validation failed:\n  " + strings.Join(errs, "\n  "))
	}

	fmt.Printf("ok: %s (%d file(s))\n", bundlePath, len(bundle.Files))
	return nil
}

func shouldSkipDir(name string) bool {
	skip := map[string]bool{
		".git": true, ".svn": true, "node_modules": true,
		"__pycache__": true, "vendor": true, ".idea": true,
		".vscode": true, "dist": true, "build": true, ".fog": true,
	}
	return skip[name]
}

func shouldSkipFile(path string) bool {
	base := strings.ToLower(filepath.Base(path))
	skip := map[string]bool{
		".ds_store": true, "thumbs.db": true, ".gitignore": true,
	}
	return skip[base]
}
