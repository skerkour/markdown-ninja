package autoupdate

import "markdown.ninja/stdx/crypto"

const (
	SaltSize = crypto.KeySize256

	ReleaseManifestFilename = "release.json"

	DefaultUserAgent = "Mozilla/5.0 (compatible; +autoupdate)"
)
