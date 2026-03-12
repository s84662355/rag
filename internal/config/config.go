package config

import (
	"errors"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server    ServerConfig      `yaml:"server"`
	MySQL     MySQLConfig       `yaml:"mysql"`
	Vector    VectorConfig      `yaml:"vector"`
	Embedding EmbeddingConfig   `yaml:"embedding"`
	Rerank    LLMProviderConfig `yaml:"rerank"`
	Rewrite   LLMProviderConfig `yaml:"rewrite"`
	QA        LLMProviderConfig `yaml:"qa"`
	Chat      LLMProviderConfig `yaml:"chat"`
	Chunk     ChunkConfig       `yaml:"chunk"`
}

type ServerConfig struct {
	Addr      string `yaml:"addr"`
	StaticDir string `yaml:"staticDir"`
}

type MySQLConfig struct {
	DSN string `yaml:"dsn"`
}

type VectorConfig struct {
	Type      string   `yaml:"type"`
	IndexName string   `yaml:"indexName"`
	Dims      int      `yaml:"dims"`
	ES        ESConfig `yaml:"es"`
}

type ESConfig struct {
	Address string `yaml:"address"`
}

type EmbeddingConfig struct {
	BaseURL string `yaml:"baseURL"`
	Model   string `yaml:"model"`
}

type LLMProviderConfig struct {
	APIKey  string `yaml:"apiKey"`
	BaseURL string `yaml:"baseURL"`
	Model   string `yaml:"model"`
}

type ChunkConfig struct {
	MaxChars int `yaml:"maxChars"`
	Overlap  int `yaml:"overlap"`
}

func Load(path string) (*Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(b, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	cfg.applyDefaults()

	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func (c *Config) applyDefaults() {
	if c.Server.Addr == "" {
		c.Server.Addr = ":8080"
	}
	if c.Server.StaticDir == "" {
		c.Server.StaticDir = "web"
	}
	if c.Vector.Dims <= 0 {
		c.Vector.Dims = 1024
	}
	if c.Chunk.MaxChars <= 0 {
		c.Chunk.MaxChars = 900
	}
	if c.Chunk.Overlap <= 0 {
		c.Chunk.Overlap = 120
	}
	if c.Chunk.Overlap >= c.Chunk.MaxChars {
		c.Chunk.Overlap = c.Chunk.MaxChars / 4
	}
}

func (c *Config) validate() error {
	if c.MySQL.DSN == "" {
		return errors.New("mysql.dsn is required")
	}
	if c.Vector.Type == "" {
		return errors.New("vector.type is required")
	}
	if c.Vector.Type != "es" {
		return fmt.Errorf("unsupported vector.type: %s", c.Vector.Type)
	}
	if c.Vector.IndexName == "" {
		return errors.New("vector.indexName is required")
	}
	if c.Vector.ES.Address == "" {
		return errors.New("vector.es.address is required")
	}
	if c.Embedding.BaseURL == "" || c.Embedding.Model == "" {
		return errors.New("embedding.baseURL and embedding.model are required")
	}
	return nil
}
