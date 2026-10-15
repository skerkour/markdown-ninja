package orderedmap

import (
	"testing"

	"markdown.ninja/stdx/yaml"
)

func TestUnmarshalYAML(t *testing.T) {
	type testStruct struct {
		MyMap Map[string, string] `yaml:"mymap"`
	}
	testData := `
mymap:
  a: "1"
  b: "2"
  c: "3"
  d: "4"
  e: "5"
  f: "6"
`

	expected := []Item[string, string]{
		{"a", "1"},
		{"b", "2"},
		{"c", "3"},
		{"d", "4"},
		{"e", "5"},
		{"f", "6"},
	}

	for range 2000 {
		var testStruct testStruct
		err := yaml.Unmarshal([]byte(testData), &testStruct)
		if err != nil {
			t.Fatal(err)
		}
		items := testStruct.MyMap.Items()
		if len(items) != 6 {
			t.Fatalf("len(items) != 6: %d", len(items))
		}

		for i, item := range items {
			if item.Key != expected[i].Key {
				t.Fatalf("items[%d].Key (%s) != expected[%d].Key (%s)", i, item.Key, i, expected[i].Key)
			}
			if item.Value != expected[i].Value {
				t.Fatalf("items[%d].Value (%s) != expected[%d].Value (%s)", i, item.Value, i, expected[i].Value)
			}
		}
	}
}
