package autoupdate

import (
	"encoding/json"
	"fmt"
)

type ChannelManifest struct {
	Name    string `json:"name"`
	Channel string `json:"channel"`
	Version string `json:"version"`
}

func (manifest ChannelManifest) ToJson() (manifestJSON []byte, err error) {
	manifestJSON, err = json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		err = fmt.Errorf("autoupdate: encoding manifest to JSON: %w", err)
		return
	}

	return
}
