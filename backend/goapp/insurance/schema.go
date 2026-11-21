package insurance

import (
	"bytes"
	"encoding/json"
	"io"
	"math/big"
	"strconv"
	"strings"
)

// validateJSONSchema supports the bounded schema vocabulary used by our AI
// requests. Unknown keywords and malformed schemas fail closed.
func validateJSONSchema(data []byte, schema any) bool {
	encoded, err := json.Marshal(schema)
	if err != nil {
		return false
	}
	var definition, value any
	if !decodeSchemaJSON(encoded, &definition) || !decodeSchemaJSON(data, &value) {
		return false
	}
	s, ok := definition.(map[string]any)
	return ok && validBoundedSchema(s) && matchesBoundedSchema(value, s)
}

func decodeSchemaJSON(data []byte, value *any) bool {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if decoder.Decode(value) != nil {
		return false
	}
	var trailing any
	return decoder.Decode(&trailing) == io.EOF
}

func validBoundedSchema(s map[string]any) bool {
	for key := range s {
		switch key {
		case "type", "properties", "required", "additionalProperties", "enum", "items", "minItems":
		default:
			return false
		}
	}
	types, ok := boundedSchemaTypes(s["type"])
	if !ok || len(types) == 0 {
		return false
	}
	if properties, exists := s["properties"]; exists {
		p, ok := properties.(map[string]any)
		if !ok {
			return false
		}
		for _, definition := range p {
			child, ok := definition.(map[string]any)
			if !ok || !validBoundedSchema(child) {
				return false
			}
		}
	}
	if required, exists := s["required"]; exists {
		keys, ok := required.([]any)
		if !ok {
			return false
		}
		seen := map[string]bool{}
		for _, key := range keys {
			name, ok := key.(string)
			if !ok || seen[name] {
				return false
			}
			seen[name] = true
		}
	}
	if additional, exists := s["additionalProperties"]; exists {
		if _, ok := additional.(bool); !ok {
			return false
		}
	}
	if items, exists := s["items"]; exists {
		child, ok := items.(map[string]any)
		if !ok || !validBoundedSchema(child) {
			return false
		}
	}
	if minimum, exists := s["minItems"]; exists {
		number, ok := minimum.(json.Number)
		if !ok {
			return false
		}
		value, err := number.Int64()
		if err != nil || value < 0 {
			return false
		}
	}
	if enum, exists := s["enum"]; exists {
		values, ok := enum.([]any)
		if !ok || len(values) == 0 {
			return false
		}
	}
	return true
}

func boundedSchemaTypes(value any) ([]string, bool) {
	var values []any
	if name, ok := value.(string); ok {
		values = []any{name}
	} else {
		values, _ = value.([]any)
	}
	types := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, value := range values {
		name, ok := value.(string)
		if !ok || seen[name] {
			return nil, false
		}
		switch name {
		case "string", "number", "integer", "boolean", "null", "array", "object":
		default:
			return nil, false
		}
		seen[name] = true
		types = append(types, name)
	}
	return types, len(types) > 0
}

func matchesBoundedSchema(value any, s map[string]any) bool {
	types, _ := boundedSchemaTypes(s["type"])
	matched := false
	for _, name := range types {
		switch name {
		case "null":
			matched = matched || value == nil
		case "string":
			_, ok := value.(string)
			matched = matched || ok
		case "boolean":
			_, ok := value.(bool)
			matched = matched || ok
		case "number", "integer":
			if n, ok := value.(json.Number); ok {
				rational, valid := boundedSchemaNumber(n)
				matched = matched || valid && (name == "number" || rational.IsInt())
			}
		case "array":
			_, ok := value.([]any)
			matched = matched || ok
		case "object":
			_, ok := value.(map[string]any)
			matched = matched || ok
		}
	}
	if !matched {
		return false
	}
	if enum, exists := s["enum"]; exists {
		found := false
		for _, candidate := range enum.([]any) {
			found = found || schemaJSONEqual(value, candidate)
		}
		if !found {
			return false
		}
	}
	if object, ok := value.(map[string]any); ok {
		properties, _ := s["properties"].(map[string]any)
		required, _ := s["required"].([]any)
		for _, key := range required {
			if _, exists := object[key.(string)]; !exists {
				return false
			}
		}
		for key, child := range object {
			definition, exists := properties[key]
			if !exists {
				if additional, exists := s["additionalProperties"]; exists && !additional.(bool) {
					return false
				}
				continue
			}
			if !matchesBoundedSchema(child, definition.(map[string]any)) {
				return false
			}
		}
	}
	if array, ok := value.([]any); ok {
		if minimum, exists := s["minItems"]; exists {
			count, _ := minimum.(json.Number).Int64()
			if int64(len(array)) < count {
				return false
			}
		}
		if items, exists := s["items"]; exists {
			for _, child := range array {
				if !matchesBoundedSchema(child, items.(map[string]any)) {
					return false
				}
			}
		}
	}
	return true
}

func schemaJSONEqual(a, b any) bool {
	switch value := a.(type) {
	case json.Number:
		other, ok := b.(json.Number)
		if !ok {
			return false
		}
		x, xOK := boundedSchemaNumber(value)
		y, yOK := boundedSchemaNumber(other)
		return xOK && yOK && x.Cmp(y) == 0
	case []any:
		other, ok := b.([]any)
		if !ok || len(value) != len(other) {
			return false
		}
		for i := range value {
			if !schemaJSONEqual(value[i], other[i]) {
				return false
			}
		}
		return true
	case map[string]any:
		other, ok := b.(map[string]any)
		if !ok || len(value) != len(other) {
			return false
		}
		for key, child := range value {
			candidate, exists := other[key]
			if !exists || !schemaJSONEqual(child, candidate) {
				return false
			}
		}
		return true
	default:
		return a == b
	}
}

func boundedSchemaNumber(number json.Number) (*big.Rat, bool) {
	text := string(number)
	if len(text) > 4096 {
		return nil, false
	}
	if i := strings.IndexAny(text, "eE"); i >= 0 {
		exponent, err := strconv.ParseInt(text[i+1:], 10, 32)
		if err != nil || exponent < -4096 || exponent > 4096 {
			return nil, false
		}
	}
	return new(big.Rat).SetString(text)
}
