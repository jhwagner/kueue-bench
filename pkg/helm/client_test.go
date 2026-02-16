package helm

import (
	"testing"
)

func TestParseSetValues(t *testing.T) {
	tests := []struct {
		name      string
		setValues map[string]string
		wantErr   bool
		checkFunc func(t *testing.T, got map[string]interface{})
	}{
		{
			name:      "empty",
			setValues: map[string]string{},
			wantErr:   false,
			checkFunc: func(t *testing.T, got map[string]interface{}) {
				if len(got) != 0 {
					t.Errorf("expected empty map, got %v", got)
				}
			},
		},
		{
			name: "simple key-value",
			setValues: map[string]string{
				"key1": "val1",
				"key2": "val2",
			},
			wantErr: false,
			checkFunc: func(t *testing.T, got map[string]interface{}) {
				if got["key1"] != "val1" {
					t.Errorf("key1 = %v, want val1", got["key1"])
				}
				if got["key2"] != "val2" {
					t.Errorf("key2 = %v, want val2", got["key2"])
				}
			},
		},
		{
			name: "dot notation for nested values",
			setValues: map[string]string{
				"foo.bar": "baz",
			},
			wantErr: false,
			checkFunc: func(t *testing.T, got map[string]interface{}) {
				foo, ok := got["foo"]
				if !ok {
					t.Fatal("missing 'foo' key")
				}
				fooMap, ok := foo.(map[string]interface{})
				if !ok {
					t.Fatalf("foo is not a map, got %T", foo)
				}
				if fooMap["bar"] != "baz" {
					t.Errorf("foo.bar = %v, want baz", fooMap["bar"])
				}
			},
		},
		{
			name: "multiple levels of nesting",
			setValues: map[string]string{
				"a.b.c": "value",
			},
			wantErr: false,
			checkFunc: func(t *testing.T, got map[string]interface{}) {
				a, ok := got["a"].(map[string]interface{})
				if !ok {
					t.Fatal("a is not a map")
				}
				b, ok := a["b"].(map[string]interface{})
				if !ok {
					t.Fatal("a.b is not a map")
				}
				if b["c"] != "value" {
					t.Errorf("a.b.c = %v, want value", b["c"])
				}
			},
		},
		{
			name: "array notation",
			setValues: map[string]string{
				"list[0]": "item1",
				"list[1]": "item2",
			},
			wantErr: false,
			checkFunc: func(t *testing.T, got map[string]interface{}) {
				list, ok := got["list"].([]interface{})
				if !ok {
					t.Fatalf("list is not an array, got %T", got["list"])
				}
				if len(list) != 2 {
					t.Fatalf("list length = %d, want 2", len(list))
				}
				if list[0] != "item1" || list[1] != "item2" {
					t.Errorf("list = %v, want [item1 item2]", list)
				}
			},
		},
		{
			name: "mixed nested and flat",
			setValues: map[string]string{
				"replicas":         "3",
				"image.tag":        "v1.2.3",
				"image.pullPolicy": "Always",
			},
			wantErr: false,
			checkFunc: func(t *testing.T, got map[string]interface{}) {
				if got["replicas"] == nil {
					t.Error("replicas key is missing")
				}
				image, ok := got["image"].(map[string]interface{})
				if !ok {
					t.Fatalf("image is not a nested map")
				}
				if image["tag"] != "v1.2.3" {
					t.Errorf("image.tag = %v, want v1.2.3", image["tag"])
				}
				if image["pullPolicy"] != "Always" {
					t.Errorf("image.pullPolicy = %v, want Always", image["pullPolicy"])
				}
			},
		},
		{
			name: "numeric and boolean strings are parsed",
			setValues: map[string]string{
				"port":    "8080",
				"enabled": "true",
			},
			wantErr: false,
			checkFunc: func(t *testing.T, got map[string]interface{}) {
				if got["port"] == nil {
					t.Error("port key is missing")
				}
				if got["enabled"] == nil {
					t.Error("enabled key is missing")
				}
				// Verify they're not strings (strvals should convert them)
				if _, ok := got["port"].(string); ok {
					t.Error("port should not be a string")
				}
				if _, ok := got["enabled"].(string); ok {
					t.Error("enabled should not be a string")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseSetValues(tt.setValues)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseSetValues() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.checkFunc != nil {
				tt.checkFunc(t, got)
			}
		})
	}
}
