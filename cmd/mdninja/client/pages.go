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
	"time"

	"markdown.ninja/pkg/services/content"
	"markdown.ninja/stdx/guid"
)

type localPage struct {
	LocalPath string

	Url               string
	Title             string
	Type              content.PageType
	Draft             bool
	Date              time.Time
	UpdatedAt         *time.Time
	Tags              []string
	FrontMatterSource string
	Language          string
	Description       string
	BodyMarkdown      string
	BodyHash          []byte
	MetadataHash      [32]byte
	SendAsNewsletter  bool
}

func (client *Client) uploadPages(ctx context.Context, websiteID guid.GUID, pageDirs []string) (err error) {
	pagesFromApi, err := client.apiClient.ListPages(ctx, content.ListPagesInput{WebsiteID: websiteID})
	if err != nil {
		err = fmt.Errorf("pages: error fetching pages: %w", err)
		return
	}
	postsFromApi, err := client.apiClient.ListPosts(ctx, content.ListPagesInput{WebsiteID: websiteID})
	if err != nil {
		err = fmt.Errorf("pages: error fetching posts: %w", err)
		return
	}

	pagesFromApi.Data = append(pagesFromApi.Data, postsFromApi.Data...)
	pagesFromApiByUrl := make(map[string]content.PageMetadata, len(pagesFromApi.Data))
	for _, page := range pagesFromApi.Data {
		pagesFromApiByUrl[page.Path] = page
	}

	localPages := make([]localPage, 0, len(pagesFromApi.Data))
	for _, folder := range pageDirs {
		var pages []localPage
		pages, err = client.loadLocalPages(ctx, folder, pagesFromApiByUrl)
		if err != nil {
			return
		}
		localPages = append(localPages, pages...)
	}

	// check for local pages with same URL
	localPagesUniqueByUrl := make(map[string]localPage, len(pagesFromApi.Data))
	for _, page := range localPages {
		if existingPage, exists := localPagesUniqueByUrl[page.Url]; exists {
			err = fmt.Errorf("pages: Pages with same URL found: %s and %s", existingPage.LocalPath, page.LocalPath)
			return
		}
		localPagesUniqueByUrl[page.Url] = page
	}

	for _, localPage := range localPages {
		if websitePage, exists := pagesFromApiByUrl[localPage.Url]; exists {
			if !bytes.Equal(websitePage.BodyHash, localPage.BodyHash) || !bytes.Equal(websitePage.MetadataHash, localPage.MetadataHash[:]) {
				updatePageInput := content.UpdatePageInput{
					PageID:           websitePage.ID,
					Date:             localPage.Date,
					UpdatedAt:        localPage.UpdatedAt,
					Title:            localPage.Title,
					Path:             localPage.Url,
					BodyMarkdown:     &localPage.BodyMarkdown,
					Draft:            localPage.Draft,
					Description:      &localPage.Description,
					Language:         localPage.Language,
					Tags:             localPage.Tags,
					SendAsNewsletter: localPage.SendAsNewsletter,
				}
				_, err = client.apiClient.UpdatePage(ctx, updatePageInput)
				if err != nil {
					err = fmt.Errorf("pages: error Updating page %s: %w", localPage.Url, err)
					return
				}
				client.logger.Info(fmt.Sprintf("Page updated: %s", localPage.Url))
			}
		} else {
			createPageInput := content.CreatePageInput{
				WebsiteID:        websiteID,
				Date:             localPage.Date,
				Type:             localPage.Type,
				Title:            localPage.Title,
				Path:             localPage.Url,
				BodyMarkdown:     localPage.BodyMarkdown,
				Description:      localPage.Description,
				Language:         localPage.Language,
				Tags:             localPage.Tags,
				Draft:            localPage.Draft,
				SendAsNewsletter: localPage.SendAsNewsletter,
			}
			_, err = client.apiClient.CreatePage(ctx, createPageInput)
			if err != nil {
				err = fmt.Errorf("pages: Error creating page %s: %w", localPage.Url, err)
				return
			}
			client.logger.Info(fmt.Sprintf("Page created: %s", localPage.Url))
		}
	}

	return
}

func (client *Client) loadLocalPages(ctx context.Context, folder string, pagesFromApi map[string]content.PageMetadata) (localPages []localPage, err error) {
	localPages = make([]localPage, 0, 100)

	directoryInfo, err := os.Stat(folder)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("pages: %s directory does not exist", folder)
		}
		return nil, fmt.Errorf("pages: error getting assets folder info (%s): %w", folder, err)
	}
	if !directoryInfo.IsDir() {
		return nil, fmt.Errorf("pages: %s is not a folder", folder)
	}

	fileSystem := os.DirFS(folder)
	err = fs.WalkDir(fileSystem, ".", func(path string, file fs.DirEntry, err error) error {
		realPath := filepath.Join(folder, path)
		if err != nil {
			return err
		}
		if !fs.ValidPath(path) {
			return fmt.Errorf("pages: %s is not a valid path", realPath)
		}
		if strings.Contains(path, "..") {
			return fmt.Errorf("pages: %s is not a valid path", realPath)
		}

		fileType := file.Type()
		if fileType.IsDir() || !fileType.IsRegular() {
			return nil
		}

		// discard files that are not markdown
		extension := filepath.Ext(path)
		if extension != ".md" {
			return nil
		}

		info, walkErr := file.Info()
		if walkErr != nil {
			return fmt.Errorf("pages: error getting info for file %s: %w", realPath, walkErr)
		}

		if info.Size() > content.PageBodyMarkdownMaxSize+512 {
			client.logger.Warn(fmt.Sprintf("pages: Ignoring %s: file is too large", realPath))
			return nil
		}

		localPage, walkErr := client.readAndParseMarkdownFile(ctx, fileSystem, path, realPath, pagesFromApi)
		if walkErr != nil {
			return walkErr
		}
		localPages = append(localPages, localPage)

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("pages: error walking folder [%s]: %v", folder, err)
	}

	return localPages, nil
}
