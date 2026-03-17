package bridge

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Exchange struct {
	dir string
}

func New(workDir string) *Exchange {
	return &Exchange{dir: filepath.Join(workDir, ".fog-bridge")}
}

func (e *Exchange) Init() error {
	return os.MkdirAll(e.dir, 0700)
}

func (e *Exchange) Set(key, value string) error {
	return os.WriteFile(filepath.Join(e.dir, sanitizeKey(key)), []byte(value), 0600)
}

func (e *Exchange) Get(key string) (string, error) {
	data, err := os.ReadFile(filepath.Join(e.dir, sanitizeKey(key)))
	if err != nil {
		return "", fmt.Errorf("bridge key %q not found", key)
	}
	return string(data), nil
}

func (e *Exchange) SetJSON(key string, v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return e.Set(key, string(data))
}

func (e *Exchange) GetJSON(key string, v any) error {
	raw, err := e.Get(key)
	if err != nil {
		return err
	}
	return json.Unmarshal([]byte(raw), v)
}

func (e *Exchange) WriteStdinFile(name, content string) (string, error) {
	p := filepath.Join(e.dir, "stdin_"+sanitizeKey(name))
	return p, os.WriteFile(p, []byte(content), 0600)
}

func (e *Exchange) EnvDir() string {
	return e.dir
}

func (e *Exchange) CollectOutputEnv() map[string]string {
	entries, err := os.ReadDir(filepath.Join(e.dir, "env"))
	if err != nil {
		return nil
	}
	result := make(map[string]string)
	for _, en := range entries {
		if en.IsDir() {
			continue
		}
		val, err := os.ReadFile(filepath.Join(e.dir, "env", en.Name()))
		if err == nil {
			result[en.Name()] = string(val)
		}
	}
	return result
}

func (e *Exchange) InjectEnvHelpers(lang string) []string {
	_ = os.MkdirAll(filepath.Join(e.dir, "env"), 0700)
	envDir := filepath.Join(e.dir, "env")

	switch lang {
	case "python":
		return []string{
			"import os as _fog_os, builtins as _fog_b",
			fmt.Sprintf("_fog_env_dir = %q", envDir),
			"def fog_set(k, v): open(_fog_env_dir+'/'+k,'w').write(str(v))",
			"def fog_get(k): return open(_fog_env_dir+'/'+k).read()",
			"def fog_export(k, v): fog_set(k, v); _fog_os.environ[k] = str(v)",
		}
	case "javascript":
		return []string{
			fmt.Sprintf("const _fogEnvDir = %q;", envDir),
			"const _fogFs = require('fs');",
			"const fog = { set: (k,v) => _fogFs.writeFileSync(_fogEnvDir+'/'+k, String(v)), get: (k) => _fogFs.readFileSync(_fogEnvDir+'/'+k,'utf8'), export: (k,v) => { fog.set(k,v); process.env[k]=String(v); } };",
		}
	case "ruby":
		return []string{
			fmt.Sprintf("_FOG_ENV_DIR = %q", envDir),
			"module Fog; def self.set(k,v) File.write(\"#{_FOG_ENV_DIR}/#{k}\", v.to_s) end; def self.get(k) File.read(\"#{_FOG_ENV_DIR}/#{k}\") end; def self.export(k,v) self.set(k,v); ENV[k.to_s]=v.to_s end; end",
		}
	case "shell":
		return []string{
			fmt.Sprintf("_FOG_ENV_DIR=%q", envDir),
			`fog_set() { echo -n "$2" > "$_FOG_ENV_DIR/$1"; }`,
			`fog_get() { cat "$_FOG_ENV_DIR/$1"; }`,
			`fog_export() { fog_set "$1" "$2"; export "$1=$2"; }`,
		}
	}
	return nil
}

func sanitizeKey(key string) string {
	return strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-' || r == '.' {
			return r
		}
		return '_'
	}, key)
}
