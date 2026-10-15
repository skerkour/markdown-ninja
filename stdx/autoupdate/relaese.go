package autoupdate

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"markdown.ninja/stdx/byteshex"
	"markdown.ninja/stdx/filex"
)

type CreateReleaseInput struct {
	// Name of the project. e.g. myapp
	Name string
	// Version of the release of the project. e.g. 1.1.52
	Version string
	Channel string
	Files   []string
	// PrivateKeyPrivateKey is the base64 encoded privateKey, encrypted with password
	PrivateKey         string
	PrivateKeyPassword string
}

type Release struct {
	Name            string
	ChannelManifest ChannelManifest
	ReleaseManifest ReleaseManifest
}

type ReleaseManifest struct {
	Name    string        `json:"name"`
	Version string        `json:"version"`
	Files   []ReleaseFile `json:"files"`
}

type ReleaseFile struct {
	Filename  string         `json:"file"`
	Sha256    byteshex.Bytes `json:"sha256"`
	Signature []byte         `json:"signature"`
}

func (manifest ReleaseManifest) ToJson() (manifestJSON []byte, err error) {
	manifestJSON, err = json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		err = fmt.Errorf("autoupdate: encoding release manifest to JSON: %w", err)
		return
	}

	return
}

func CreateRelease(ctx context.Context, info CreateReleaseInput) (release Release, err error) {
	release = Release{
		Name: info.Name,
		ChannelManifest: ChannelManifest{
			Name:    info.Name,
			Channel: info.Channel,
			Version: info.Version,
		},
	}

	signInput := make([]SignInput, len(info.Files))
	for index, file := range info.Files {
		fileExists := false
		var fileHandle *os.File
		filename := filepath.Base(file)

		fileExists, err = filex.Exists(file)
		if err != nil {
			err = fmt.Errorf("autoupdate: checking if file exists (%s): %w", file, err)
			return
		}

		if !fileExists {
			err = fmt.Errorf("autoupdate: file does not exist: %s", file)
			return
		}

		fileHandle, err = os.Open(file)
		if err != nil {
			err = fmt.Errorf("autoupdate: error opening file (%s): %w", file, err)
			return
		}
		defer func(fileToClose *os.File) {
			fileToClose.Close()
		}(fileHandle)
		fileSignInput := SignInput{
			Filename: filename,
			Reader:   fileHandle,
		}
		signInput[index] = fileSignInput
	}

	signatures, err := SignMany(info.PrivateKey, info.PrivateKeyPassword, signInput)
	if err != nil {
		return
	}

	release.ReleaseManifest = ReleaseManifest{
		Name:    info.Name,
		Version: info.Version,
		Files:   signatures,
	}

	return
}
