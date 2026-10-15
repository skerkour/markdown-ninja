package languages

import (
	_ "embed"
	"encoding/json"
	"fmt"
)

//go:embed languages.json
var Bytes []byte

type Language struct {
	Code       string `json:"code"`
	Name       string `json:"name"`
	NativeName string `json:"native_name"`
}

var langs map[string]Language

func Get() map[string]Language {
	var err error

	if langs == nil {
		langs = map[string]Language{}
		err = json.Unmarshal(Bytes, &langs)
		if err != nil {
			err = fmt.Errorf("languages: parsing languages JSON file: %w", err)
			panic(err)
		}
	}

	return langs
}
