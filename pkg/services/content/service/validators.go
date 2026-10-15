package service

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"markdown.ninja/pkg/errs"
	"markdown.ninja/pkg/services/content"
	"markdown.ninja/pkg/services/websites"
	"markdown.ninja/stdx/languages"
	"markdown.ninja/stdx/stringsx"
)

var allowedSpecialCharactersForAssetFilename = "._-()[]"
var allowedSpecialCharactersForAssetFoldername = "._-"

func (service *ContentService) validateAssetFileName(name string) error {
	if !utf8.ValidString(name) {
		return content.ErrAssetNameIsNotValid
	}

	if len(name) > content.AssetNameMaxSize {
		return errs.InvalidArgument(fmt.Sprintf("asset name is too long. max: %d characters", content.AssetNameMaxSize))
	}

	for _, character := range name {
		if !unicode.IsLetter(character) && !unicode.IsDigit(character) &&
			!strings.ContainsRune(allowedSpecialCharactersForAssetFilename, character) {
			return content.ErrAssetNameIsNotValid
		}
	}

	return nil
}

func (service *ContentService) validateAssetFolderName(name string) error {
	if !utf8.ValidString(name) {
		return content.ErrAssetNameIsNotValid
	}

	if len(name) > content.AssetNameMaxSize {
		return errs.InvalidArgument(fmt.Sprintf("Folder name is too long. Max: %d characters", content.AssetNameMaxSize))
	}

	for _, character := range name {
		if !unicode.IsLetter(character) && !unicode.IsDigit(character) &&
			!strings.ContainsRune(allowedSpecialCharactersForAssetFoldername, character) {
			return content.ErrAssetNameIsNotValid
		}
	}

	return nil
}

func (service *ContentService) validateAssetFolder(path string) error {
	if !utf8.ValidString(path) {
		return content.ErrPathIsNotValid
	}

	if !strings.HasPrefix(path, "/assets") {
		return content.ErrPathShouldStartWithAssets
	}

	if len(path) > content.AssetPathMaxSize {
		return errs.InvalidArgument(fmt.Sprintf("path is too long. max: %d characters", content.AssetPathMaxSize))
	}

	if strings.HasSuffix(path, "/") {
		return content.ErrPathShouldNotFinishBySlash
	}

	if strings.Contains(path, "//") {
		return content.ErrPathIsNotValid
	}

	if strings.Count(path, "/") > content.MaxAssetNestedFolders {
		return content.ErrPathIsTooDeep
	}

	folders := strings.Split(path, "/")
	for _, folderName := range folders {
		err := service.validateAssetFolderName(folderName)
		if err != nil {
			return err
		}
	}

	return nil
}

func (service *ContentService) ValidatePageTitle(title string) error {
	if !utf8.ValidString(title) {
		return content.ErrPageTitleIsNotValid
	}

	if len(title) < content.PageTitleMinSize {
		return content.ErrPageTitleIsTooShort
	}

	if len(title) > content.PageTitleMaxSize {
		return content.ErrPageTitleIsTooLong
	}

	return nil
}

// TODO
func (service *ContentService) validatePagePath(path string) error {
	if len(path) == 0 || path[0] != '/' ||
		// disallow trailing slashes
		(len(path) > 1 && strings.HasSuffix(path, "/")) {
		return content.ErrPageUrlIsNotValid
	}

	if !utf8.ValidString(path) {
		return content.ErrPageUrlIsNotValid
	}

	if len(path) > content.PagePathMaxSize {
		return errs.InvalidArgument(fmt.Sprintf("page path is too long. max: %d characters", content.PagePathMaxSize))
	}

	if strings.Contains(path, "//") || strings.Contains(path, "..") || strings.Contains(path, "\\") {
		return content.ErrPageUrlIsNotValid
	}

	if service.pageUrlBlocklist.Contains(path) {
		return content.ErrPageUrlIsNotValid
	}

	if strings.HasPrefix(path, websites.MarkdownNinjaPathPrefix) ||
		strings.HasPrefix(path, "/assets") ||
		strings.HasPrefix(path, "/theme/") ||
		strings.HasPrefix(path, "/api/") {
		return content.ErrPageUrlIsNotValid
	}

	if strings.HasPrefix(path, "/favicon.") || strings.HasPrefix(path, "/favicon_") {
		return content.ErrPageUrlIsNotValid
	}

	for _, char := range path {
		if !strings.Contains(content.ContentPathAlphabet, string(char)) {
			return content.ErrPageUrlIsNotValid
		}
	}

	return nil
}

func (service *ContentService) validateLanguage(language string) error {
	langs := languages.Get()

	if language == "" {
		return nil
	}

	_, exists := langs[language]
	if !exists {
		return websites.ErrInvalidLanguage
	}

	return nil
}

func (service *ContentService) validatePageDescription(description string) error {
	if len(description) > content.ContentDescriptionMaxSize {
		return content.ErrPageDescriptionIsTooLong
	}

	if !utf8.ValidString(description) {
		return content.ErrPageDescriptionIsNotValid
	}

	return nil
}

func (service *ContentService) validateTagName(name string) error {
	if len(name) < content.TagNameMinSize {
		return content.ErrTagNameIsTooShort
	}

	if len(name) > content.TagNameMaxSize {
		return content.ErrTagNameIsTooLong
	}

	if strings.TrimSpace(name) != name {
		return content.ErrTagNameIsNotValid
	}

	if !utf8.ValidString(name) {
		return content.ErrTagNameIsNotValid
	}

	if !stringsx.IsLower(name) {
		return content.ErrTagNameMustBeLower
	}

	for _, char := range name {
		if !strings.ContainsRune(content.TagNameAlphabet, char) {
			return content.ErrTagNameIsNotValid
		}
	}

	return nil
}

func (service *ContentService) validateTagDescription(description string) error {
	if len(description) > content.TagDescriptionMaxSize {
		return content.ErrTagDescriptionIsTooLong
	}

	if !utf8.ValidString(description) {
		return content.ErrTagDescriptionIsNotValid
	}

	return nil
}

func (service *ContentService) validateSnippetName(name string) error {
	if len(name) < content.SnippetNameMinLength {
		return content.ErrSnippetNameIsNotValid
	}

	if len(name) > content.SnippetNameMaxLength {
		return content.ErrSnippetNameIsNotValid
	}

	if strings.HasPrefix(name, "mdninja") || strings.HasPrefix(name, "markdown_ninja") {
		return content.ErrSnippetNameIsNotValid
	}

	if service.snippetNameBlocklist.Contains(name) {
		return content.ErrSnippetNameIsNotValid
	}

	if !utf8.ValidString(name) {
		return content.ErrSnippetNameIsNotValid
	}

	for _, char := range name {
		if !strings.ContainsRune(content.SnippetNameAlphabet, char) {
			return content.ErrSnippetNameIsNotValid
		}
	}

	if strings.Contains(name, "--") || strings.Contains(name, "__") {
		return content.ErrSnippetNameIsNotValid
	}

	if strings.HasPrefix(name, "-") || strings.HasSuffix(name, "-") ||
		strings.HasPrefix(name, "_") || strings.HasSuffix(name, "_") {
		return content.ErrSnippetNameIsNotValid
	}

	return nil
}

func (service *ContentService) validateSnippetContent(snippetContent string) error {
	if len(snippetContent) < content.SnippetContentMinLength {
		return content.ErrSnippetContentIsNotValid
	}

	if len(snippetContent) > content.SnippetContentMaxLength {
		return content.ErrSnippetContentIsNotValid
	}

	if !utf8.ValidString(snippetContent) {
		return content.ErrSnippetContentIsNotValid
	}

	return nil
}

func (service *ContentService) ValidatePageBodyMarkdown(body string) error {
	if len(body) > content.PageBodyMarkdownMaxSize {
		return content.ErrPageBodyIsTooLarge
	}

	if !utf8.ValidString(body) {
		return content.ErrPageBodyIsNotValid
	}

	return nil
}

func (service *ContentService) validatePageSendAsNewsletter(sendAsNewsletter bool, pageType content.PageType) error {
	if sendAsNewsletter && pageType != content.PageTypePost {
		return content.ErrOnlyPostsCanBeSentAsNewsletter
	}

	return nil
}
