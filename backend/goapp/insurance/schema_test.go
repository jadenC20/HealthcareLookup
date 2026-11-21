package insurance

import "testing"

func TestValidateJSONSchemaRequiredFieldsAndTypes(t *testing.T) {
	schema := object(map[string]any{
		"name":   map[string]any{"type": "string", "enum": []string{"plan"}},
		"amount": map[string]any{"type": []string{"number", "null"}},
		"pages":  map[string]any{"type": "array", "items": map[string]any{"type": "integer"}},
		"valid":  map[string]any{"type": "boolean"},
	})
	for _, data := range []string{
		`{"name":"plan","amount":null,"pages":[1,2],"valid":true}`,
		`{"name":"plan","amount":12.5,"pages":[1e2],"valid":false}`,
	} {
		if !validateJSONSchema([]byte(data), schema) {
			t.Errorf("valid output rejected: %s", data)
		}
	}
	for _, data := range []string{
		`{"name":"plan","amount":null,"pages":[1]}`,
		`{"name":"plan","amount":null,"pages":[1],"valid":true,"extra":0}`,
		`{"name":"other","amount":null,"pages":[1],"valid":true}`,
		`{"name":"plan","amount":"12","pages":[1],"valid":true}`,
		`{"name":"plan","amount":null,"pages":[1.2],"valid":true}`,
		`{"name":"plan","amount":null,"pages":[1.0000000000000001],"valid":true}`,
		`{"name":"plan","amount":null,"pages":[1],"valid":"true"}`,
		`{"name":"plan","amount":null,"pages":[1],"valid":true} {}`,
		`{"name":`,
		`null`,
	} {
		if validateJSONSchema([]byte(data), schema) {
			t.Errorf("invalid output accepted: %s", data)
		}
	}
}

func TestValidateJSONSchemaNestedAndUnsupportedDefinitions(t *testing.T) {
	nested := object(map[string]any{"details": object(map[string]any{"name": stringSchema()})})
	if validateJSONSchema([]byte(`{"details":{}}`), nested) || validateJSONSchema([]byte(`{"details":{"name":"plan","extra":true}}`), nested) {
		t.Fatal("nested required or additional-property constraint ignored")
	}
	for _, schema := range []any{
		map[string]any{"type": "string", "minLength": 1},
		map[string]any{"type": "object", "properties": map[string]any{"optional": map[string]any{"type": "string", "pattern": "x"}}},
		map[string]any{"type": "array", "items": map[string]any{"type": "integer", "minimum": 0}},
		map[string]any{"type": "object", "required": []any{1}},
		map[string]any{"type": "object", "additionalProperties": map[string]any{"type": "string"}},
		map[string]any{"type": []string{"unknown", "null"}},
	} {
		if validateJSONSchema([]byte(`{}`), schema) {
			t.Fatalf("unsupported or malformed schema accepted: %#v", schema)
		}
	}
}

func TestValidateJSONSchemaEnumsCompareJSONValues(t *testing.T) {
	schema := map[string]any{"type": "number", "enum": []any{1}}
	if !validateJSONSchema([]byte(`1.0`), schema) || validateJSONSchema([]byte(`1.0000000000000001`), schema) {
		t.Fatal("numeric enum equality lost precision")
	}
}

func TestValidateJSONSchemaMinimumArrayItemsAndAllowedReferences(t *testing.T) {
	schema := map[string]any{"type": "array", "minItems": 1, "items": map[string]any{"type": "string", "enum": []string{"L1", "L2"}}}
	for _, data := range []string{`["L1"]`, `["L2","L1"]`} {
		if !validateJSONSchema([]byte(data), schema) {
			t.Errorf("valid references rejected: %s", data)
		}
	}
	for _, data := range []string{`[]`, `["invented"]`, `[null]`, `[1]`} {
		if validateJSONSchema([]byte(data), schema) {
			t.Errorf("missing or malformed references accepted: %s", data)
		}
	}
	for _, minimum := range []any{-1, 0.5, "1", true, nil} {
		malformed := map[string]any{"type": "array", "minItems": minimum, "items": stringSchema()}
		if validateJSONSchema([]byte(`["L1"]`), malformed) {
			t.Errorf("malformed minItems accepted: %#v", minimum)
		}
	}
	if !validateJSONSchema([]byte(`[]`), map[string]any{"type": "array", "minItems": 0, "items": stringSchema()}) {
		t.Fatal("nonnegative zero minimum rejected")
	}
}

func TestValidateJSONSchemaMaximumArrayItems(t *testing.T) {
	schema := map[string]any{"type": "array", "minItems": 1, "maxItems": 2, "items": map[string]any{"type": "integer", "enum": []int{0, 1}}}
	for _, data := range []string{`[0]`, `[1,0]`} {
		if !validateJSONSchema([]byte(data), schema) {
			t.Errorf("bounded array rejected: %s", data)
		}
	}
	for _, data := range []string{`[]`, `[0,1,0]`, `[2]`} {
		if validateJSONSchema([]byte(data), schema) {
			t.Errorf("out-of-bounds array accepted: %s", data)
		}
	}
	for _, maximum := range []any{-1, 0.5, "1", true, nil} {
		if validateJSONSchema([]byte(`[]`), map[string]any{"type": "array", "maxItems": maximum, "items": stringSchema()}) {
			t.Errorf("malformed maxItems accepted: %#v", maximum)
		}
	}
	if validateJSONSchema([]byte(`[0]`), map[string]any{"type": "array", "minItems": 2, "maxItems": 1, "items": map[string]any{"type": "integer"}}) {
		t.Fatal("contradictory array bounds accepted")
	}
	empty := map[string]any{"type": "array", "maxItems": 0, "items": map[string]any{"type": "integer"}}
	if !validateJSONSchema([]byte(`[]`), empty) || validateJSONSchema([]byte(`[0]`), empty) {
		t.Fatal("zero maximum failed to prohibit unavailable benefit references")
	}
}
