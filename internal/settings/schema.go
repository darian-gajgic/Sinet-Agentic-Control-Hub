package settings

import (
	"encoding/json"
	"fmt"
	"strings"
)

// JSONSchema emits the registry as one JSON Schema document — the single
// artifact behind all three S01.10 consumers: validation-on-write, the
// generated settings UI (grouping via the nested object shape; renderer per
// the frontend workshop's pick), and the generated reference docs. Dotted
// keys become nested objects; map-valued keys become objects with typed
// additionalProperties; registry metadata rides the x-sinet annotation.
func (r *Registry) JSONSchema() ([]byte, error) {
	root := newNode()
	root["$schema"] = "https://json-schema.org/draft/2020-12/schema"
	root["title"] = "Sinet settings registry"
	root["description"] = "Generated from the declare-once settings registry (Spec S01.10); key index Spec S18."

	for _, key := range r.order {
		d := r.decls[key]
		segs := strings.Split(d.Key, ".")
		node := root
		for i, seg := range segs {
			props := node["properties"].(map[string]any)
			if i == len(segs)-1 {
				if _, exists := props[seg]; exists {
					return nil, fmt.Errorf("settings: schema path collision at %q", d.Key)
				}
				props[seg] = leafSchema(d)
				break
			}
			child, ok := props[seg].(map[string]any)
			if !ok {
				child = newNode()
				if i == 0 {
					child["title"] = seg + " (" + d.Section + ")"
					child["x-sinet-owner"] = d.Section
				}
				props[seg] = child
			} else if _, isGroup := child["properties"].(map[string]any); !isGroup {
				return nil, fmt.Errorf("settings: schema path collision under %q at segment %q", d.Key, seg)
			}
			node = child
		}
	}
	return json.MarshalIndent(root, "", "  ")
}

func newNode() map[string]any {
	return map[string]any{
		"type":                 "object",
		"properties":           map[string]any{},
		"additionalProperties": false,
	}
}

func leafSchema(d *Decl) map[string]any {
	s := map[string]any{
		"title":       d.Title,
		"description": d.Help,
	}
	switch d.Type {
	case TypeBool:
		s["type"] = "boolean"
		s["default"] = d.Default.(bool)
	case TypeInt, TypeDuration:
		s["type"] = "integer"
		s["default"] = d.Default.(int64)
	case TypeFloat:
		s["type"] = "number"
		s["default"] = d.Default.(float64)
	case TypeString:
		s["type"] = "string"
		s["default"] = d.Default.(string)
	case TypeEnum:
		s["type"] = "string"
		s["enum"] = d.Enum
		s["default"] = d.Default.(string)
	case TypeList:
		s["type"] = "array"
		s["items"] = map[string]any{"type": "string"}
		s["default"] = d.Default.([]string)
	case TypeMap:
		s["type"] = "object"
		s["additionalProperties"] = map[string]any{"type": "string"}
		s["default"] = d.Default.(map[string]string)
	}
	if d.Min != nil {
		if d.MinExclusive {
			s["exclusiveMinimum"] = *d.Min
		} else {
			s["minimum"] = *d.Min
		}
	}
	if d.Max != nil {
		s["maximum"] = *d.Max
	}

	x := map[string]any{
		"key":        d.Key,
		"section":    d.Section,
		"type":       d.Type.String(),
		"ratifiedBy": d.Ratified,
	}
	if d.Unit != "" {
		x["unit"] = d.Unit
	}
	if d.Auto {
		x["auto"] = true
	}
	if d.PerUser {
		x["perUser"] = true
	}
	if d.Restart {
		x["restartRequired"] = true
	}
	if d.Dormant != "" {
		x["dormant"] = d.Dormant
	}
	s["x-sinet"] = x
	return s
}
