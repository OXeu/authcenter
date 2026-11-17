package server

import (
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
)

func validateToolValue(value any, definition map[string]any, path string) error {
	switch definition["type"] {
	case "object":
		object, ok := value.(map[string]any)
		if !ok {
			return fmt.Errorf("%s must be an object", path)
		}
		properties, ok := definition["properties"].(map[string]any)
		if !ok {
			return fmt.Errorf("%s has invalid schema properties", path)
		}
		required, ok := definition["required"].([]string)
		if !ok {
			return fmt.Errorf("%s has invalid schema requirements", path)
		}
		for _, name := range required {
			if _, exists := object[name]; !exists {
				return fmt.Errorf("%s.%s is required", path, name)
			}
		}
		for name, child := range object {
			property, ok := properties[name].(map[string]any)
			if !ok {
				return fmt.Errorf("%s contains unknown property %s", path, name)
			}
			if err := validateToolValue(child, property, path+"."+name); err != nil {
				return err
			}
		}
	case "array":
		array, ok := value.([]any)
		if !ok {
			return fmt.Errorf("%s must be an array", path)
		}
		minimum, constrained := definition["minItems"].(int)
		maximum, limited := definition["maxItems"].(int)
		if (constrained && len(array) < minimum) || (limited && len(array) > maximum) {
			return fmt.Errorf("%s has an invalid number of items", path)
		}
		item, ok := definition["items"].(map[string]any)
		if !ok {
			return fmt.Errorf("%s has invalid item schema", path)
		}
		for index, child := range array {
			if err := validateToolValue(child, item, fmt.Sprintf("%s[%d]", path, index)); err != nil {
				return err
			}
		}
	case "string":
		text, ok := value.(string)
		if !ok {
			return fmt.Errorf("%s must be a string", path)
		}
		if enum, ok := definition["enum"].([]string); ok && !slices.Contains(enum, text) {
			return fmt.Errorf("%s has an unsupported value", path)
		}
	case "integer":
		number, ok := value.(json.Number)
		if !ok {
			return fmt.Errorf("%s must be an integer", path)
		}
		integer, err := strconv.ParseUint(string(number), 10, 64)
		if err != nil || integer == 0 {
			return fmt.Errorf("%s must be a positive integer", path)
		}
	case "boolean":
		if _, ok := value.(bool); !ok {
			return fmt.Errorf("%s must be a boolean", path)
		}
	default:
		return fmt.Errorf("%s has an unsupported schema", path)
	}
	return nil
}
