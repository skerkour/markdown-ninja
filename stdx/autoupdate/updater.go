package autoupdate

import (
	"errors"
	"net/http"
	"sync"

	"markdown.ninja/stdx/httpx"
	"markdown.ninja/stdx/semver"
)

type Config struct {
	PublicKey string
	// BaseURL is the URL of the folder containing the manifest
	// e.g. https://downloads.example.com/myapp
	BaseURL        string
	CurrentVersion string
	ReleaseChannel string
	// Interval to check for updates. default: 1800 seconds
	Interval int64
	// Verbose logs actions with the INFO level
	Verbose    bool
	UserAgent  *string
	HttpClient *http.Client
}

type Updater struct {
	httpClient             *http.Client
	baseUrl                string
	publicKey              string
	currentVersion         string
	userAgent              string
	releaseChannel         string
	updateInProgress       sync.Mutex
	latestVersionAvailable string
	latestVersionInstalled string
	autoupdateInterval     int64
	verbose                bool

	Updated chan struct{}
}

func NewUpdater(config Config) (updater *Updater, err error) {
	if config.HttpClient == nil {
		config.HttpClient = httpx.DefaultClient()
	}

	if config.BaseURL == "" {
		err = errors.New("autoupdate: BaseURL is empty")
		return
	}

	if config.PublicKey == "" {
		err = errors.New("autoupdate: PublicKey is empty")
		return
	}

	if config.CurrentVersion == "" {
		err = errors.New("autoupdate: CurrentVersion is empty")
		return
	}

	if config.Interval == 0 {
		config.Interval = 1800
	}

	userAgent := DefaultUserAgent
	if config.UserAgent != nil {
		userAgent = *config.UserAgent
	}

	updater = &Updater{
		httpClient:             config.HttpClient,
		baseUrl:                config.BaseURL,
		publicKey:              config.PublicKey,
		currentVersion:         config.CurrentVersion,
		userAgent:              userAgent,
		releaseChannel:         config.ReleaseChannel,
		updateInProgress:       sync.Mutex{},
		latestVersionAvailable: config.CurrentVersion,
		latestVersionInstalled: config.CurrentVersion,
		autoupdateInterval:     config.Interval,
		verbose:                config.Verbose,
		Updated:                make(chan struct{}),
	}
	return
}

func (updater *Updater) RestartRequired() bool {
	return updater.latestVersionInstalled != updater.currentVersion
}

// UpdateAvailable returns true if the latest avaiable version is > to the latest install version
func (updater *Updater) UpdateAvailable(manifest ChannelManifest) bool {
	return semver.Compare(manifest.Version, updater.latestVersionInstalled) > 0
}
