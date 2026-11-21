package insurance

// configureOpenRouterPayload adapts Responses requests to OpenRouter without
// changing the selected model or loosening the inference routing constraints.
func configureOpenRouterPayload(p map[string]any) {
	p["provider"] = map[string]any{
		"require_parameters": true,
		"data_collection":    "deny",
		"max_price":          map[string]any{"prompt": 0, "completion": 0},
	}
	plugins, _ := p["plugins"].([]any)
	tools, _ := p["tools"].([]any)
	for _, tool := range tools {
		t, _ := tool.(map[string]any)
		if t["type"] == "web_search" {
			delete(p, "tools")
			delete(p, "tool_choice")
			delete(p, "include")
			plugins = setOpenRouterPlugin(plugins, map[string]any{"id": "web", "engine": "exa", "max_results": 3})
			break
		}
	}
	input, _ := p["input"].([]any)
	for _, message := range input {
		m, _ := message.(map[string]any)
		content, _ := m["content"].([]any)
		for _, part := range content {
			c, _ := part.(map[string]any)
			if c["type"] == "input_file" {
				plugins = setOpenRouterPlugin(plugins, map[string]any{"id": "file-parser", "pdf": map[string]any{"engine": "cloudflare-ai"}})
			}
		}
	}
	if len(plugins) > 0 {
		p["plugins"] = plugins
	}
}

func setOpenRouterPlugin(plugins []any, plugin map[string]any) []any {
	for i, existing := range plugins {
		p, _ := existing.(map[string]any)
		if p["id"] == plugin["id"] {
			plugins[i] = plugin
			return plugins
		}
	}
	return append(plugins, plugin)
}
