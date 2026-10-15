package semver

import "golang.org/x/mod/semver"

func IsValid(version string) bool {
	return semver.IsValid("v" + version)
}

func Compare(v, w string) int {
	return semver.Compare("v"+v, "v"+w)
}
