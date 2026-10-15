package client

import (
	"context"
	"fmt"
	"os"
	"strings"

	"markdown.ninja/pkg/services/websites"
	"markdown.ninja/stdx/orderedmap"
	"markdown.ninja/stdx/yaml"
)

type config struct {
	Site        *string  `yaml:"site"`
	PageDirs    []string `yaml:"pages"`
	Name        *string  `yaml:"name"`
	Description *string  `yaml:"description"`

	Header *string `yaml:"header"`
	Footer *string `yaml:"footer"`

	Navigation   *websites.WebsiteNavigation     `yaml:"navigation"`
	RedirectsMap *orderedmap.Map[string, string] `yaml:"redirects"`
	Redirects    []websites.RedirectInput        `yaml:"-"`

	Ad           *string `yaml:"ad"`
	Announcement *string `yaml:"announcement"`
}

// TODO
func (client *Client) loadConfig(_ context.Context, configPath string) (conf config, err error) {
	// read config file
	configData, err := os.ReadFile(configPath)
	if err != nil {
		err = ErrReadingConfigFile(configPath, err)
		return
	}

	err = yaml.Unmarshal(configData, &conf)
	if err != nil {
		err = ErrParsingConfigFile(configPath, err)
		return
	}

	// TODO: more valdiation...
	if conf.Name != nil {
		name := strings.TrimSpace(*conf.Name)
		conf.Name = &name
	}

	if conf.Description != nil {
		description := strings.TrimSpace(*conf.Description)
		conf.Description = &description
	}

	if conf.Header != nil {
		header := strings.TrimSpace(*conf.Header)
		conf.Header = &header
	}

	if conf.Footer != nil {
		footer := strings.TrimSpace(*conf.Footer)
		conf.Footer = &footer
	}

	if conf.Ad != nil {
		ad := strings.TrimSpace(*conf.Ad)
		conf.Ad = &ad
	} else {
		// empty string to set it as null
		conf.Ad = new("")
	}

	if conf.Announcement != nil {
		announcement := strings.TrimSpace(*conf.Announcement)
		conf.Announcement = &announcement
	} else {
		// empty string to set it as null
		conf.Announcement = new("")
	}

	if len(conf.PageDirs) == 0 {
		err = fmt.Errorf("config: pages folders not valid. At least 1 directory is required")
		return
	}

	// TODO: more validation
	for _, directory := range conf.PageDirs {
		if strings.Contains(directory, "..") {
			err = fmt.Errorf("pages dir's path can't contain \"..\": (%s)", directory)
			return
		}
	}

	// // TODO: more validation
	// if conf.Navigation != nil {
	// 	conf.websiteNavigation = &websites.WebsiteNavigation{}

	// 	primary := conf.Navigation.Primary
	// 	if primary != nil {
	// 		primaryItems := primary.Items()
	// 		conf.websiteNavigation.Primary = make([]websites.WebsiteNavigationItem, len(primaryItems))
	// 		for i, elem := range primaryItems {
	// 			conf.websiteNavigation.Primary[i] = websites.WebsiteNavigationItem{Label: elem.Key, Url: elem.Value}
	// 		}
	// 	}

	// 	secondary := conf.Navigation.Secondary
	// 	if secondary != nil {
	// 		secondaryItems := secondary.Items()
	// 		conf.websiteNavigation.Secondary = make([]websites.WebsiteNavigationItem, len(secondaryItems))
	// 		for i, elem := range secondaryItems {
	// 			conf.websiteNavigation.Secondary[i] = websites.WebsiteNavigationItem{Label: elem.Key, Url: elem.Value}
	// 		}
	// 	}
	// }

	if conf.RedirectsMap != nil {
		conf.Redirects = make([]websites.RedirectInput, len(conf.RedirectsMap.Items()))
		for i, elem := range conf.RedirectsMap.Items() {
			conf.Redirects[i] = websites.RedirectInput{
				Pattern: elem.Key,
				To:      elem.Value,
			}
		}
	}

	return
}
