// Package config loads and exposes the language registry from configs/languages.yaml.
package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Limits defines resource caps for a single execution phase.
type Limits struct {
	WallTimeS    int `yaml:"wall_time_s"`
	MemoryKB     int `yaml:"memory_kb"`
	MaxProcesses int `yaml:"max_processes"`
}

// BuildPhase describes how to compile a language that requires compilation.
type BuildPhase struct {
	Cmd           string   `yaml:"cmd"`
	Args          []string `yaml:"args"`
	Limits        Limits   `yaml:"limits"`
	FlagAllowlist []string `yaml:"flag_allowlist"`
}

// RunPhase describes how to execute the compiled artifact or script.
type RunPhase struct {
	Cmd    string   `yaml:"cmd"`
	Args   []string `yaml:"args"`
	Limits Limits   `yaml:"limits"`
}

// Language is one entry in the language registry.
type Language struct {
	ID   string `yaml:"id"`
	Name string `yaml:"name"`

	SourceFilename         string `yaml:"source_filename"`
	SourceFilenameStrategy string `yaml:"source_filename_strategy"`

	Artifact                string `yaml:"artifact"`
	ArtifactFilenameStrategy string `yaml:"artifact_filename_strategy"`

	VersionCmd []string `yaml:"version_cmd"`

	Build *BuildPhase `yaml:"build"`
	Run   RunPhase    `yaml:"run"`
}

// Registry is the top-level YAML document structure.
type Registry struct {
	Languages []Language `yaml:"languages"`
}

// Config is the loaded, validated configuration.
type Config struct {
	ByID map[string]*Language
}

// Load reads the YAML file at path and returns a validated Config.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("config: cannot read %s: %w", path, err)
	}

	var reg Registry
	if err := yaml.Unmarshal(data, &reg); err != nil {
		return nil, fmt.Errorf("config: YAML parse error in %s: %w", path, err)
	}

	if len(reg.Languages) == 0 {
		return nil, fmt.Errorf("config: no languages defined in %s", path)
	}

	byID := make(map[string]*Language, len(reg.Languages))
	for i := range reg.Languages {
		lang := &reg.Languages[i]
		if lang.ID == "" {
			return nil, fmt.Errorf("config: language at index %d is missing 'id'", i)
		}
		if lang.Name == "" {
			return nil, fmt.Errorf("config: language %q is missing 'name'", lang.ID)
		}
		if lang.Run.Cmd == "" {
			return nil, fmt.Errorf("config: language %q is missing 'run.cmd'", lang.ID)
		}
		if lang.SourceFilenameStrategy != "from_request" && lang.SourceFilename == "" {
			return nil, fmt.Errorf("config: language %q needs 'source_filename' or 'source_filename_strategy: from_request'", lang.ID)
		}
		if _, dup := byID[lang.ID]; dup {
			return nil, fmt.Errorf("config: duplicate language id %q", lang.ID)
		}
		byID[lang.ID] = lang
	}

	return &Config{ByID: byID}, nil
}

// Get looks up a language by ID.
func (c *Config) Get(id string) (*Language, bool) {
	lang, ok := c.ByID[id]
	return lang, ok
}

// All returns all languages.
func (c *Config) All() []*Language {
	langs := make([]*Language, 0, len(c.ByID))
	for _, l := range c.ByID {
		langs = append(langs, l)
	}
	return langs
}
