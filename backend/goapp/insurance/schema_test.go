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
