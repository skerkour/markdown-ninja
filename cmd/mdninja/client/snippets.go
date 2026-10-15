package client

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/zeebo/blake3"
	"markdown.ninja/pkg/services/content"
	"markdown.ninja/stdx/guid"
)

const SNIPPETS_DIR = "snippets"

type localSnippet struct {
	Path    string
	Name    string
	Content string
	Hash    []byte
}

func (client *Client) updateSnippets(ctx context.Context, websiteID guid.GUID) (err error) {
	websiteSnippets, err := client.apiClient.ListSnippets(ctx, content.ListSnippetsInput{WebsiteID: websiteID})
	if err != nil {
		err = fmt.Errorf("publish: Fetching snippets: %w", err)
		return
	}

	directoryInfo, err := os.Stat(SNIPPETS_DIR)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			client.logger.Debug("No snippets directory found.")
			err = nil
			return
		}
		return
	}
	if !directoryInfo.IsDir() {
		client.logger.Warn(fmt.Sprintf("Snippets folder (%s) is not a directory.", SNIPPETS_DIR))
		err = nil
		return
	}

	localSnippets, err := client.walkSnippets(SNIPPETS_DIR)
	if err != nil {
		return
	}

	existingSnippetsByName := make(map[string]content.Snippet, len(websiteSnippets.Data))
	for _, snippet := range websiteSnippets.Data {
		existingSnippetsByName[snippet.Name] = snippet
	}

	for _, localSnippet := range localSnippets {
		existingSnippet, exists := existingSnippetsByName[localSnippet.Name]

		if !exists {
			var newSnippet content.Snippet
			apiInput := content.CreateSnippetInput{
				WebsiteID:      websiteID,
				Name:           localSnippet.Name,
				Content:        localSnippet.Content,
				RenderInEmails: nil,
			}
			newSnippet, err = client.apiClient.CreateSnippet(ctx, apiInput)
			if err != nil {
				return
			}
			websiteSnippets.Data = append(websiteSnippets.Data, newSnippet)
			client.logger.Info(fmt.Sprintf("Snippet created: %s", localSnippet.Path))
		} else if exists && !bytes.Equal(existingSnippet.Hash, localSnippet.Hash) {
			var snippet content.Snippet
			apiInput := content.UpdateSnippetInput{
				ID:             existingSnippet.ID,
				Name:           localSnippet.Name,
				Content:        localSnippet.Content,
				RenderInEmails: nil,
			}
			snippet, err = client.apiClient.UpdateSnippet(ctx, apiInput)
			if err != nil {
				return
			}
			client.updateWebsiteSnippet(websiteSnippets.Data, snippet)
			client.logger.Info(fmt.Sprintf("Snippet updated: %s", localSnippet.Path))
		}
	}

	return
}

func (client *Client) walkSnippets(snippetsDirectory string) (localSnippets []localSnippet, err error) {
	localSnippets = make([]localSnippet, 0, 100)

	fileSystem := os.DirFS(snippetsDirectory)
	err = fs.WalkDir(fileSystem, ".", func(path string, file fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if !fs.ValidPath(path) {
			return nil
		}
		if strings.Contains(path, "..") {
			return nil
		}

		fileType := file.Type()
		if fileType.IsDir() || !fileType.IsRegular() {
			return nil
		}

		info, err := file.Info()
		if err != nil {
			return err
		}
		pathOnFilesystem := filepath.Join(snippetsDirectory, path)
		nameWithExtension := file.Name()
		extension := filepath.Ext(nameWithExtension)
		name := strings.TrimSuffix(nameWithExtension, extension)

		if extension != ".html" {
			return nil
		}

		if info.Size() > content.SnippetContentMaxLength {
			client.logger.Warn(fmt.Sprintf("Ignoring %s: snippet is too large", pathOnFilesystem))
			return nil
		}

		contentBytes, err := fs.ReadFile(fileSystem, path)
		if err != nil {
			return err
		}

		content := strings.TrimSpace(string(contentBytes))
		contentHash := blake3.Sum256([]byte(content))

		localSnippet := localSnippet{
			Name:    name,
			Path:    pathOnFilesystem,
			Content: string(content),
			Hash:    contentHash[:],
		}
		localSnippets = append(localSnippets, localSnippet)

		return nil
	})

	if err != nil {
		client.logger.Error(fmt.Sprintf("walking snippets dir (%s): %v\n", snippetsDirectory, err))
		return
	}
	return
}

func (client *Client) updateWebsiteSnippet(websiteSnippets []content.Snippet, updatedSnippet content.Snippet) {
	if websiteSnippets == nil {
		return
	}

	for i, snippet := range websiteSnippets {
		if snippet.ID.Equal(updatedSnippet.ID) {
			websiteSnippets[i] = updatedSnippet
			return
		}
	}
}
