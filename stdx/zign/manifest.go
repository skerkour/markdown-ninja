package zign

import (
	"encoding/json"
	"fmt"
)

const (
	Version1 = 1

	DefaultManifestFilename = "zign.json"
)

type Manifest struct {
	Version uint64       `json:"version"`
	Files   []SignOutput `json:"files"`
}

func (manifest Manifest) ToJson() (manifestJSON []byte, err error) {
	manifestJSON, err = json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		err = fmt.Errorf("zign: encoding manifest to JSON: %w", err)
		return
	}

	return
}

func GenerateManifest(signOutput []SignOutput) (manifest Manifest) {
	manifest = Manifest{
		Version: Version1,
		Files:   signOutput,
	}

	return
}
