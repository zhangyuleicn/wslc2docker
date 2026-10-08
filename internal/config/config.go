// Package config 负责加载 wslc2docker 的单一机器真相源 endpoints.yaml，
// 并提供「请求 → 端点定义」的匹配路由能力。
package config

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// Endpoint 是 endpoints.yaml 中单条端点定义（权威的开/关与映射来源）。
type Endpoint struct {
	ID       int    `yaml:"id"`
	Resource string `yaml:"resource"`
	Method   string `yaml:"method"`
	Path     string `yaml:"path"`
	Wslc     string `yaml:"wslc"`
	Panel    string `yaml:"panel"`
	Bridge   string `yaml:"bridge"`
	MSPlan   string `yaml:"ms_plan"`
	Plan     string `yaml:"plan"`
	Enabled  bool   `yaml:"enabled"`
	Note     string `yaml:"note"`
}

// Config 是 endpoints.yaml 的内存表示。
type Config struct {
	Socket     string     `yaml:"socket"`
	TCP        string     `yaml:"tcp"`
	WslcBinary string     `yaml:"wslc_binary"`
	Backend    string     `yaml:"backend"` // "wslc" | "fake"
	LogLevel   string     `yaml:"log_level"`
	Endpoints  []Endpoint `yaml:"endpoints"`
}

// Load 读取并解析 yaml 文件。
func Load(path string) (*Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	var c Config
	if err := yaml.Unmarshal(raw, &c); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	if c.Socket == "" {
		c.Socket = "/var/run/docker.sock"
	}
	if c.WslcBinary == "" {
		c.WslcBinary = "wslc.exe"
	}
	if c.Backend == "" {
		c.Backend = "wslc"
	}
	if c.LogLevel == "" {
		c.LogLevel = "info"
	}
	return &c, nil
}

// ByID 返回 id → Endpoint 的索引，便于快速查 enabled 状态。
func (c *Config) ByID() map[int]*Endpoint {
	m := make(map[int]*Endpoint, len(c.Endpoints))
	for i := range c.Endpoints {
		m[c.Endpoints[i].ID] = &c.Endpoints[i]
	}
	return m
}

// normalizeVersion 去掉 Docker 客户端可能带上的 /v1.56 前缀。
func normalizeVersion(p string) string {
	if len(p) > 2 && p[1] == 'v' && p[2] >= '0' && p[2] <= '9' {
		if idx := strings.Index(p[1:], "/"); idx > 0 {
			return p[1:][idx:]
		}
	}
	return p
}

// compilePath 把 /containers/{id}/json 形式的模板编译为正则并提取参数名。
func compilePath(tmpl string) (*regexp.Regexp, []string) {
	var names []string
	var b strings.Builder
	b.WriteString("^")
	for i := 0; i < len(tmpl); {
		if tmpl[i] == '{' {
			end := strings.Index(tmpl[i:], "}")
			if end < 0 {
				b.WriteString(regexp.QuoteMeta(string(tmpl[i])))
				i++
				continue
			}
			names = append(names, tmpl[i+1:i+end])
			b.WriteString(`([^/]+)`)
			i = i + end + 1
		} else {
			b.WriteString(regexp.QuoteMeta(string(tmpl[i])))
			i++
		}
	}
	b.WriteString("$")
	return regexp.MustCompile(b.String()), names
}

// Match 根据 method + path 匹配端点定义，返回端点、路径参数与是否命中。
func (c *Config) Match(method, path string) (*Endpoint, map[string]string, bool) {
	path = normalizeVersion(path)
	for i := range c.Endpoints {
		ep := &c.Endpoints[i]
		if ep.Method != method {
			continue
		}
		re, names := compilePath(ep.Path)
		m := re.FindStringSubmatch(path)
		if m == nil {
			continue
		}
		params := make(map[string]string, len(names))
		for j, n := range names {
			params[n] = m[j+1]
		}
		return ep, params, true
	}
	return nil, nil, false
}
