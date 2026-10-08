package config

import (
	"testing"
)

func TestLoadEndpointsYaml(t *testing.T) {
	cfg, err := Load("../../endpoints.yaml")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(cfg.Endpoints) != 72 {
		t.Fatalf("expected 72 endpoints, got %d", len(cfg.Endpoints))
	}
	if cfg.Socket == "" {
		t.Fatal("socket empty")
	}
	// 校验 enabled 的端点都有对应处理器（在 api 包中维护，这里仅确认数量合理）
	var enabled int
	for _, e := range cfg.Endpoints {
		if e.Enabled {
			enabled++
		}
	}
	if enabled < 20 {
		t.Fatalf("expected at least 20 enabled endpoints, got %d", enabled)
	}
}

func TestByID(t *testing.T) {
	cfg, err := Load("../../endpoints.yaml")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	m := cfg.ByID()
	if len(m) != 72 {
		t.Fatalf("ByID should have 72 entries, got %d", len(m))
	}
	if _, ok := m[1]; !ok {
		t.Fatal("missing endpoint #1")
	}
}

func TestMatch(t *testing.T) {
	cfg, err := Load("../../endpoints.yaml")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	// 带 /v1.56 前缀的版本协商应正常匹配
	ep, params, ok := cfg.Match("GET", "/v1.56/containers/abc123/json")
	if !ok {
		t.Fatal("expected match for /containers/{id}/json")
	}
	if ep.ID != 11 {
		t.Fatalf("expected endpoint #11, got #%d", ep.ID)
	}
	if params["id"] != "abc123" {
		t.Fatalf("expected id=abc123, got %q", params["id"])
	}
	// 未匹配
	if _, _, ok := cfg.Match("DELETE", "/containers"); ok {
		t.Fatal("DELETE /containers should not match any endpoint")
	}
}
