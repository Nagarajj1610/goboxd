package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/thesouldev/goboxd/internal/config"
)

func writeTemp(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "languages.yaml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("writeTemp: %v", err)
	}
	return path
}

func TestLoad_ValidYAML(t *testing.T) {
	yaml := `
languages:
  - id: py3
    name: Python 3
    source_filename: solution.py
    version_cmd: ["/usr/bin/python3", "--version"]
    run:
      cmd: /usr/bin/python3
      args: ["{{source}}"]
      limits:
        wall_time_s: 9
        memory_kb: 102400
        max_processes: 100
`
	cfg, err := config.Load(writeTemp(t, yaml))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	lang, ok := cfg.Get("py3")
	if !ok {
		t.Fatal("py3 not found in config")
	}
	if lang.Name != "Python 3" {
		t.Errorf("expected name 'Python 3', got %q", lang.Name)
	}
	if lang.Run.Limits.WallTimeS != 9 {
		t.Errorf("expected wall_time_s 9, got %d", lang.Run.Limits.WallTimeS)
	}
}

func TestLoad_MissingID(t *testing.T) {
	yaml := `
languages:
  - name: No ID Language
    source_filename: solution.py
    run:
      cmd: /usr/bin/python3
      limits:
        wall_time_s: 5
`
	_, err := config.Load(writeTemp(t, yaml))
	if err == nil {
		t.Fatal("expected error for missing id, got nil")
	}
}

func TestLoad_MissingRunCmd(t *testing.T) {
	yaml := `
languages:
  - id: broken
    name: Broken
    source_filename: solution.py
    run:
      cmd: ""
      limits:
        wall_time_s: 5
`
	_, err := config.Load(writeTemp(t, yaml))
	if err == nil {
		t.Fatal("expected error for empty run.cmd, got nil")
	}
}

func TestLoad_EmptyLanguages(t *testing.T) {
	_, err := config.Load(writeTemp(t, `languages: []`))
	if err == nil {
		t.Fatal("expected error for empty language list, got nil")
	}
}

func TestLoad_UnknownLanguageID(t *testing.T) {
	yaml := `
languages:
  - id: py3
    name: Python 3
    source_filename: solution.py
    version_cmd: ["/usr/bin/python3", "--version"]
    run:
      cmd: /usr/bin/python3
      limits:
        wall_time_s: 9
        memory_kb: 102400
        max_processes: 100
`
	cfg, err := config.Load(writeTemp(t, yaml))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_, ok := cfg.Get("rust")
	if ok {
		t.Error("expected 'rust' to be not found, but it was")
	}
}

func TestLoad_DuplicateID(t *testing.T) {
	yaml := `
languages:
  - id: py3
    name: Python 3
    source_filename: solution.py
    version_cmd: ["/usr/bin/python3", "--version"]
    run:
      cmd: /usr/bin/python3
      limits:
        wall_time_s: 9
        memory_kb: 102400
        max_processes: 100
  - id: py3
    name: Python 3 duplicate
    source_filename: solution.py
    version_cmd: ["/usr/bin/python3", "--version"]
    run:
      cmd: /usr/bin/python3
      limits:
        wall_time_s: 9
        memory_kb: 102400
        max_processes: 100
`
	_, err := config.Load(writeTemp(t, yaml))
	if err == nil {
		t.Fatal("expected error for duplicate language id, got nil")
	}
}
