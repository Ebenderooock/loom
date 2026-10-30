package collections

import (
	"os"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestOpenAPICollectionsContract(t *testing.T) {
	body, err := os.ReadFile("../../api/openapi/loom.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := yaml.Unmarshal(body, &doc); err != nil {
		t.Fatalf("OpenAPI YAML: %v", err)
	}
	paths := doc["paths"].(map[string]any)
	for path, method := range map[string]string{
		"/api/v1/collections/":                      "get",
		"/api/v1/collections/discover":              "post",
		"/api/v1/collections/scan":                  "get",
		"/api/v1/collections/{id}":                  "patch",
		"/api/v1/collections/{id}/sources":          "post",
		"/api/v1/collections/{id}/refresh":          "post",
		"/api/v1/collections/{id}/merge":            "post",
		"/api/v1/collections/{id}/members":          "post",
		"/api/v1/collections/{id}/members/{member}": "patch",
	} {
		entry, ok := paths[path].(map[string]any)
		if !ok || entry[method] == nil {
			t.Errorf("missing collection endpoint %s %s", method, path)
		}
	}
	var visit func(any)
	refs := 0
	visit = func(value any) {
		switch v := value.(type) {
		case []any:
			for _, item := range v {
				visit(item)
			}
		case map[string]any:
			for key, item := range v {
				if key == "$ref" {
					ref, ok := item.(string)
					if ok && strings.HasPrefix(ref, "#/") {
						var node any = doc
						for _, part := range strings.Split(ref[2:], "/") {
							mapping, ok := node.(map[string]any)
							if !ok || mapping[part] == nil {
								t.Errorf("unresolved OpenAPI reference %s", ref)
								break
							}
							node = mapping[part]
						}
						refs++
					}
				}
				visit(item)
			}
		}
	}
	// Scope this contract to Collections. Legacy routes have their own schema
	// contracts (and may use JSON pointers into unrelated path parameters).
	for path, entry := range paths {
		if strings.HasPrefix(path, "/api/v1/collections") {
			visit(entry)
		}
	}
	for name, schema := range doc["components"].(map[string]any)["schemas"].(map[string]any) {
		if strings.HasPrefix(name, "Collection") || name == "LoomCollection" {
			visit(schema)
		}
	}
	if refs == 0 {
		t.Fatal("no OpenAPI references checked")
	}
}
