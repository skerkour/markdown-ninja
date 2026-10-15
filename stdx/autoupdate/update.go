package autoupdate

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"markdown.ninja/stdx/httpx"
	"markdown.ninja/stdx/log/slogx"
	"markdown.ninja/stdx/semver"
)

func (updater *Updater) CheckUpdate(ctx context.Context) (manifest ChannelManifest, err error) {
	logger := slogx.FromCtx(ctx)

	manifestUrl := fmt.Sprintf("%s/%s.json", updater.baseUrl, updater.releaseChannel)

	logger.Debug("autoupdate.CheckUpdate: fetching channel manifest", slog.String("release_manifest_url", manifestUrl))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, manifestUrl, nil)
	if err != nil {
		err = fmt.Errorf("autoupdate.CheckUpdate: creating channel manifest HTTP request: %w", err)
		return
	}

	req.Header.Add(httpx.HeaderAccept, httpx.MediaTypeJson)
	req.Header.Add(httpx.HeaderUserAgent, updater.userAgent)

	res, err := updater.httpClient.Do(req)
	if err != nil {
		err = fmt.Errorf("autoupdate.CheckUpdate: error fetching channel manifest (%s): %w", manifestUrl, err)
		return
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		err = fmt.Errorf("autoupdate.CheckUpdate: Status code is not 200 when fetching channel manifest (%s): %d", manifestUrl, res.StatusCode)
		return

	}

	mainfestData, err := io.ReadAll(res.Body)
	if err != nil {
		err = fmt.Errorf("autoupdate.CheckUpdate: Reading manifest response (%s): %w", manifestUrl, err)
		return
	}

	err = json.Unmarshal(mainfestData, &manifest)
	if err != nil {
		err = fmt.Errorf("autoupdate.CheckUpdate: parsing manifest: %w", err)
		return
	}

	if !semver.IsValid(manifest.Version) {
		err = fmt.Errorf("autoupdate.CheckUpdate: version (%s) is not a valid semantic version string", manifest.Version)
		return
	}

	updater.latestVersionAvailable = manifest.Version

	return
}

func (updater *Updater) Update(ctx context.Context, channelManifest ChannelManifest) (err error) {
	updater.updateInProgress.Lock()
	defer updater.updateInProgress.Unlock()

	releaseManifest, err := updater.fetchReleaseManifest(ctx, channelManifest)
	if err != nil {
		return
	}

	tmpDir, err := os.MkdirTemp("", channelManifest.Name+"_autoupdate_"+channelManifest.Version)
	if err != nil {
		err = fmt.Errorf("autoupdate: creating temporary directory: %w", err)
		return
	}
	defer os.RemoveAll(tmpDir)

	destPath := filepath.Join(tmpDir, channelManifest.Name)

	platform := runtime.GOOS + "_" + runtime.GOARCH
	updateFilename := fmt.Sprintf("%s_%s_%s_%s", channelManifest.Name, channelManifest.Version, runtime.GOOS, runtime.GOARCH)

	artifactExists := false
	var artifactToDownload ReleaseFile
	for _, artifact := range releaseManifest.Files {
		artifactExtension := filepath.Ext(artifact.Filename)
		if updateFilename == strings.TrimSuffix(artifact.Filename, artifactExtension) {
			artifactExists = true
			artifactToDownload = artifact
		}
	}
	if !artifactExists {
		err = fmt.Errorf("autoupdate: No file found for platform: %s", platform)
		return
	}

	artifactUrl := updater.baseUrl + "/" + channelManifest.Version + "/" + artifactToDownload.Filename

	res, err := updater.httpClient.Get(artifactUrl)
	if err != nil {
		err = fmt.Errorf("autoupdate: fetching artifact: %w", err)
		return
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		err = fmt.Errorf("autoupdate: Status code is not 200 when fetching artifact: %d", res.StatusCode)
		return
	}

	artifactFile, err := io.ReadAll(res.Body)
	if err != nil {
		err = fmt.Errorf("autoupdate: reading artifact's response: %d", res.StatusCode)
		return
	}

	artifactFileReader := bytes.NewReader(artifactFile)

	verifyInput := VerifyInput{
		Reader:    artifactFileReader,
		Sha256:    artifactToDownload.Sha256,
		Signature: artifactToDownload.Signature,
	}
	err = Verify(updater.publicKey, verifyInput)
	if err != nil {
		err = fmt.Errorf("autoupdate: verifying signature: %w", err)
		return
	}

	artifactFileReader.Seek(0, io.SeekStart)

	// handle both .tar.gz and .zip artifacts
	if strings.HasSuffix(artifactToDownload.Filename, ".tar.gz") {
		err = updater.extractTarGzArchive(artifactFileReader, destPath)
	} else if strings.HasSuffix(artifactToDownload.Filename, ".zip") {
		err = updater.extractZipArchive(artifactFileReader, int64(artifactFileReader.Len()), destPath)
	} else {
		err = fmt.Errorf("autoupdate: unsupported archive format: %s", filepath.Ext(artifactToDownload.Filename))
	}
	if err != nil {
		return
	}

	execPath, err := os.Executable()
	if err != nil {
		err = fmt.Errorf("autoupdate: getting current executable path: %w", err)
		return
	}

	err = os.Rename(destPath, execPath)
	if err != nil {
		err = fmt.Errorf("autoupdate: moving update to executable path: %w", err)
		return
	}

	updater.latestVersionInstalled = channelManifest.Version

	return
}

func (updater *Updater) extractTarGzArchive(dataReader io.Reader, destPath string) (err error) {
	gzipReader, err := gzip.NewReader(dataReader)
	if err != nil {
		err = fmt.Errorf("autoupdate: creating gzip reader: %w", err)
		return
	}
	defer gzipReader.Close()

	tarReader := tar.NewReader(gzipReader)

	fileToExtractHeader, err := tarReader.Next()
	if fileToExtractHeader == nil || err == io.EOF {
		err = errors.New("autoupdate: no file inside .tar.gz archive")
		return
	} else if err != nil {
		err = fmt.Errorf("autoupdate: reading .tar.gz archive: %w", err)
		return
	}

	if fileToExtractHeader.Typeflag != tar.TypeReg {
		err = fmt.Errorf("autoupdate: reading .tar.gz archive: %s is not a regular file", fileToExtractHeader.Name)
		return
	}

	updatedExecutable, err := os.OpenFile(destPath, updatedExecutableOpenFlags, fileToExtractHeader.FileInfo().Mode())
	if err != nil {
		err = fmt.Errorf("autoupdate: creating dest file (%s): %w", destPath, err)
		return
	}
	defer updatedExecutable.Close()

	_, err = io.Copy(updatedExecutable, tarReader)
	if err != nil {
		err = fmt.Errorf("autoupdate: extracting .tar.gzipped file (%s): %w", fileToExtractHeader.Name, err)
		return
	}

	return
}

func (updater *Updater) extractZipArchive(dataReader io.ReaderAt, dataLen int64, destPath string) (err error) {
	zipReader, err := zip.NewReader(dataReader, dataLen)
	if err != nil {
		err = fmt.Errorf("autoupdate: creating zip reader: %w", err)
		return
	}

	zippedFiles := zipReader.File
	if len(zippedFiles) != 1 {
		err = fmt.Errorf("autoupdate: zip archive contains more than 1 file (%d)", len(zippedFiles))
		return
	}

	zippedFileToExtract := zippedFiles[0]

	srcFile, err := zippedFileToExtract.Open()
	if err != nil {
		err = fmt.Errorf("autoupdate: Opening zipped file (%s): %w", zippedFileToExtract.Name, err)
		return
	}
	defer srcFile.Close()

	updatedExecutable, err := os.OpenFile(destPath, updatedExecutableOpenFlags, zippedFileToExtract.Mode())
	if err != nil {
		err = fmt.Errorf("autoupdate: creating dest file (%s): %w", destPath, err)
		return
	}
	defer updatedExecutable.Close()

	_, err = io.Copy(updatedExecutable, srcFile)
	if err != nil {
		err = fmt.Errorf("autoupdate: extracting zipped file (%s): %w", zippedFileToExtract.Name, err)
		return
	}

	return
}

func (updater *Updater) fetchReleaseManifest(ctx context.Context, channelManifest ChannelManifest) (releaseManifest ReleaseManifest, err error) {
	logger := slogx.FromCtx(ctx)

	releaseManifestUrl := fmt.Sprintf("%s/%s/%s", updater.baseUrl, channelManifest.Version, ReleaseManifestFilename)

	logger.Debug("autoupdate.fetchReleaseManifest: fetching release manifest", slog.String("url", releaseManifestUrl))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, releaseManifestUrl, nil)
	if err != nil {
		err = fmt.Errorf("autoupdate.fetchReleaseManifest: creating release manifest HTTP request: %w", err)
		return
	}

	req.Header.Add(httpx.HeaderAccept, httpx.MediaTypeJson)
	req.Header.Add(httpx.HeaderUserAgent, updater.userAgent)

	res, err := updater.httpClient.Do(req)
	if err != nil {
		err = fmt.Errorf("autoupdate.fetchReleaseManifest: fetching release manifest: %w", err)
		return
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		err = fmt.Errorf("autoupdate.fetchReleaseManifest: Status code is not 200 when fetching release manifest: %d", res.StatusCode)
		return

	}

	mainfestData, err := io.ReadAll(res.Body)
	if err != nil {
		err = fmt.Errorf("autoupdate.fetchReleaseManifest: Reading release manifest response: %d", res.StatusCode)
		return
	}

	err = json.Unmarshal(mainfestData, &releaseManifest)
	if err != nil {
		err = fmt.Errorf("autoupdate.fetchReleaseManifest: parsing release manifest: %w", err)
		return
	}
	return
}
