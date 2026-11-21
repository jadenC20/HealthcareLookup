package insurance

import (
	"reflect"
	"testing"
)

func TestConfigureOpenRouterSearchPlugin(t *testing.T) {
	p := map[string]any{
		"model":       "google/gemma-4-31b-it:free",
		"input":       "Find official SBC documents",
		"tools":       []any{map[string]any{"type": "web_search"}},
		"tool_choice": "required",
		"include":     []string{"web_search_call.action.sources"},
	}
	configureOpenRouterPayload(p)
	if p["model"] != "google/gemma-4-31b-it:free" || p["input"] != "Find official SBC documents" {
		t.Fatal("selected model or search input changed")
	}
	for _, name := range []string{"tools", "tool_choice", "include"} {
		if _, exists := p[name]; exists {
			t.Errorf("OpenAI search parameter %q was retained", name)
		}
	}
	want := []any{map[string]any{"id": "web", "engine": "exa", "max_results": 3}}
	if !reflect.DeepEqual(p["plugins"], want) {
		t.Fatalf("unexpected search plugins: %#v", p["plugins"])
	}
	provider := map[string]any{"require_parameters": true, "data_collection": "deny", "max_price": map[string]any{"prompt": 0, "completion": 0}}
	if !reflect.DeepEqual(p["provider"], provider) {
		t.Fatalf("inference constraints changed: %#v", p["provider"])
	}
}

func TestConfigureOpenRouterMergesSearchAndPDFPlugins(t *testing.T) {
	p := map[string]any{
		"plugins": []any{map[string]any{"id": "existing", "enabled": true}},
		"tools":   []any{map[string]any{"type": "web_search"}},
		"input": []any{map[string]any{"role": "user", "content": []any{
			map[string]any{"type": "input_file", "filename": "document.pdf", "file_data": "data:application/pdf;base64,fixture"},
			map[string]any{"type": "input_file", "filename": "second.pdf", "file_data": "data:application/pdf;base64,fixture"},
		}}},
	}
	configureOpenRouterPayload(p)
	want := []any{
		map[string]any{"id": "existing", "enabled": true},
		map[string]any{"id": "web", "engine": "exa", "max_results": 3},
		map[string]any{"id": "file-parser", "pdf": map[string]any{"engine": "cloudflare-ai"}},
	}
	if !reflect.DeepEqual(p["plugins"], want) {
		t.Fatalf("plugins were overwritten or duplicated: %#v", p["plugins"])
	}
	configureOpenRouterPayload(p)
	if !reflect.DeepEqual(p["plugins"], want) {
		t.Fatalf("repeated configuration duplicated plugins: %#v", p["plugins"])
	}
}

func TestConfigureOpenRouterPreservesOtherTools(t *testing.T) {
	tools := []any{map[string]any{"type": "function", "name": "lookup"}}
	p := map[string]any{"tools": tools, "tool_choice": "auto", "include": []string{"other"}, "plugins": []any{map[string]any{"id": "existing"}}}
	configureOpenRouterPayload(p)
	if !reflect.DeepEqual(p["tools"], tools) || p["tool_choice"] != "auto" || !reflect.DeepEqual(p["include"], []string{"other"}) {
		t.Fatal("non-search tool configuration changed")
	}
	if !reflect.DeepEqual(p["plugins"], []any{map[string]any{"id": "existing"}}) {
		t.Fatal("existing plugin changed")
	}
}

func TestConfigureOpenRouterReplacesExistingParserConfiguration(t *testing.T) {
	p := map[string]any{
		"plugins": []any{map[string]any{"id": "file-parser", "pdf": map[string]any{"engine": "paid-parser"}}},
		"input":   []any{map[string]any{"content": []any{map[string]any{"type": "input_file"}}}},
	}
	configureOpenRouterPayload(p)
	want := []any{map[string]any{"id": "file-parser", "pdf": map[string]any{"engine": "cloudflare-ai"}}}
	if !reflect.DeepEqual(p["plugins"], want) {
		t.Fatalf("free PDF parser was not enforced: %#v", p["plugins"])
	}
}
