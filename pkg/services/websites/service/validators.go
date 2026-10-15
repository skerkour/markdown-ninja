package service

import (
	"net/http"
	"strings"
	"unicode/utf8"

	"markdown.ninja/pkg/errs"
	"markdown.ninja/pkg/services/websites"
	"markdown.ninja/stdx/stringsx"
	"markdown.ninja/stdx/validate"
	"markdown.ninja/themes"
)

func validateWebsiteName(name string) error {
	if len(name) < websites.WebsiteNameMinLength {
		return websites.ErrWebsiteNameIsTooShort
	}

	if len(name) > websites.WebsiteNameMaxLength {
		return websites.ErrWebsiteNameIsTooLong
	}

	if !utf8.ValidString(name) {
		return websites.ErrWebsiteNameIsNotValid
	}

	return nil
}

func validateWebsiteSlug(slug string, isAdmin bool) error {
	if len(slug) < websites.WebsiteSlugMinLength {
		return websites.ErrWebsiteSlugIsTooShort
	}

	if len(slug) > websites.WebsiteSlugMaxLength {
		return websites.ErrWebsiteSlugIsTooLong
	}

	if strings.TrimSpace(slug) != slug {
		return websites.ErrWebsiteSlugIsNotValid
	}

	if !stringsx.IsLower(slug) {
		return websites.ErrWebsiteSlugIsNotValid
	}

	if websites.WebsiteSlugBlocklist.Contains(slug) {
		return websites.ErrWebsiteSlugIsNotValid
	}

	if !validate.IsASCII(slug) {
		return websites.ErrWebsiteSlugIsNotValid
	}

	if !isAdmin && (strings.Contains(slug, "mdninja") ||
		strings.Contains(slug, "markdownninja") || strings.Contains(slug, "markdown-ninja")) {
		return websites.ErrWebsiteSlugIsNotValid
	}

	if strings.Contains(slug, "--") ||
		strings.HasPrefix(slug, "-") || strings.HasSuffix(slug, "-") {
		return websites.ErrWebsiteSlugIsNotValid
	}

	if !websites.WebsiteSlugRegexp.MatchString(slug) {
		return websites.ErrWebsiteSlugIsNotValid
	}

	return nil
}

func validateWebsiteHeader(header string) error {
	if len(header) > websites.WebsiteHeaderMaxLength {
		return websites.ErrWebsiteHeaderIsTooLong
	}

	if !utf8.ValidString(header) {
		return websites.ErrWebsiteHeaderIsNotValid
	}

	return nil
}

func validateWebsiteFooter(footer string) error {
	if len(footer) > websites.WebsiteFooterMaxLength {
		return websites.ErrWebsiteFooterIsTooLong
	}

	if !utf8.ValidString(footer) {
		return websites.ErrWebsiteFooterIsNotValid
	}

	return nil
}

func validateWebsiteDescription(description string) error {
	if len(description) > websites.WebsiteDescriptionMaxLength {
		return websites.ErrWebsiteDescriptionIsTooLong
	}

	if !utf8.ValidString(description) {
		return websites.ErrWebsiteDescriptionIsNotValid
	}

	return nil
}

func validateRedirectStatus(status int) error {
	if status != http.StatusFound && status != http.StatusMovedPermanently {
		return websites.ErrRedirectStatusIsNotValid
	}

	return nil
}

// TODO
func validateRedirectDestination(destination string) error {
	if !utf8.ValidString(destination) {
		return websites.ErrRedirectDestinationIsNotValid
	}

	return nil
}

// TODO
func validateRedirectPattern(pattern string) error {
	if !utf8.ValidString(pattern) {
		return websites.ErrRedirectPatternIsNotValid
	}

	return nil
}

func (service *WebsitesService) validateDomainName(domainName string, isUserAdmin bool) error {
	if domainName == "" {
		return websites.ErrDomainNameIsNotValid
	}

	if !validate.IsASCII(domainName) ||
		!validate.IsDNSName(domainName) ||
		!stringsx.IsLower(domainName) {
		return websites.ErrDomainNameIsNotValid
	}

	if strings.Contains(domainName, "..") ||
		strings.HasPrefix(domainName, ".") || strings.HasSuffix(domainName, ".") {
		return websites.ErrDomainNameIsNotValid
	}

	if !isUserAdmin {
		if strings.HasSuffix(domainName, service.websitesRootDomain) ||
			strings.HasSuffix(domainName, service.config.HTTP.WebappDomain) {
			return websites.ErrDomainNameIsNotValid
		}
	}

	return nil
}

func validateRobotsTxt(robotsTxt string) error {
	if len(robotsTxt) > websites.RobotsTxtMaxLength {
		return websites.ErrRobotsTxtIsTooLong
	}

	if !utf8.ValidString(robotsTxt) {
		return websites.ErrRobotsTxtIsNotValid
	}

	return nil
}

func validateCurrency(currency websites.Currency) error {
	if !websites.AllCurrencies.Contains(currency) {
		return websites.ErrCurrencyIsNotValid
	}

	return nil
}

func validateTheme(themeName string) error {
	if !themes.BuiltInThemes.Contains(themeName) {
		return errs.InvalidArgument("Theme is not valid")
	}

	return nil
}

func validateAd(ad string) error {
	if len(ad) > websites.AdMaxLength {
		return websites.ErrAdIsNotValid
	}

	if !utf8.ValidString(ad) {
		return websites.ErrAdIsNotValid
	}

	return nil
}

func validateAnnouncement(announcement string) error {
	if len(announcement) > websites.AnnouncementMaxLength {
		return websites.ErrAnnouncementIsNotValid
	}

	if !utf8.ValidString(announcement) {
		return websites.ErrAnnouncementIsNotValid
	}

	return nil
}

func validateWebsiteLogo(logoUrl string) error {
	if !strings.HasPrefix(logoUrl, "/assets/") {
		return errs.InvalidArgument("logo URL must start with /assets/")
	}

	if !utf8.ValidString(logoUrl) {
		return websites.ErrLogoUrlisNotValid
	}

	if strings.Contains(logoUrl, "..") || strings.Contains(logoUrl, "//") || strings.ContainsRune(logoUrl, '\\') {
		return websites.ErrLogoUrlisNotValid
	}

	return nil
}

func validateWebsiteNavigation(navigation *websites.WebsiteNavigation) error {
	for _, item := range navigation.Primary {
		err := validateWebsiteNavigationItem(&item, 0)
		if err != nil {
			return err
		}
	}

	for _, item := range navigation.Secondary {
		err := validateWebsiteNavigationItem(&item, 0)
		if err != nil {
			return err
		}
	}

	return nil
}

func validateWebsiteNavigationItem(item *websites.WebsiteNavigationItem, depth uint) error {
	if depth > 1 {
		return errs.InvalidArgument("Navigation children can't have children yet")
	}

	if item.Label == "" {
		return errs.InvalidArgument("Navigation item label is empty")
	}

	if item.Url == nil && len(item.Children) == 0 ||
		item.Url != nil && len(item.Children) != 0 {
		return errs.InvalidArgument("Navigation items should have either an url or children, but not both")
	}

	for _, child := range item.Children {
		err := validateWebsiteNavigationItem(&child, depth+1)
		if err != nil {
			return err
		}
	}

	return nil
}
