package bunny

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

type VideoStatus int64

const (
	VideoStatusCreated = iota
	VideoStatusUploaded
	VideoStatusProcessing
	VideoStatusTranscoding
	// Video is ready
	VideoStatusFinished
	VideoStatusError
	VideoStatusUploadFailed
)

type FetchVideoInput struct {
	Url string `json:"url"`
}

type FetchVideoOutput struct {
	Success    bool   `json:"success"`
	Message    string `json:"message"`
	StatusCode int64  `json:"statusCode"`

	ID string `json:"id"`
}

type UploadVideoOutput struct {
	Success    bool   `json:"success"`
	Message    string `json:"message"`
	StatusCode int64  `json:"statusCode"`
}

// See https://docs.bunny.net/reference/video_getvideo for details
type Video struct {
	VideoLibraryID int64  `json:"videoLibraryId"`
	Guid           string `json:"guid"`
	Title          string `json:"title"`
	// "dateUploaded": "2023-08-17T12:45:11.381",
	Views    int64 `json:"views"`
	IsPublic bool  `json:"isPublic"`
	// Length is the duration of the video in seconds
	Length    int64       `json:"length"`
	Status    VideoStatus `json:"status"`
	Framerate float64     `json:"framerate"`
	// "rotation": 0,
	Width  int64 `json:"width"`
	Height int64 `json:"height"`
	// The available resolutions of the video. "360p,720p,1080p" for example
	AvailableResolutions string `json:"availableResolutions"`
	ThumbnailCount       int64  `json:"thumbnailCount"`
	// The current encode progress of the video
	EncodeProgress int64 `json:"encodeProgress"`
	// The amount of storage used by this video
	StorageSize int64 `json:"storageSize"`
	// "captions": [],
	HasMP4Fallback bool `json:"hasMP4Fallback"`
	// The ID of the collection where the video belongs. Can be empty.
	CollectionID string `json:"collectionId"`
	// The file name of the thumbnail inside of the storage
	ThumbnailFileName string `json:"thumbnailFileName"`
	// The average watch time of the video in seconds
	AverageWatchTime int64 `json:"averageWatchTime"`
	// The total video watch time in seconds
	TotalWatchTime int64 `json:"totalWatchTime"`
	// The automatically detected category of the video. Default: "unknown"
	Category string         `json:"category"`
	Chapters []VideoChapter `json:"chapters"`
	Moments  []VideoMoment  `json:"moments"`
	MetaTags []VideoMetaTag `json:"metaTags"`
	// "transcodingMessages": []
}

type VideoChapter struct {
	Title string `json:"title"`
	Start int64  `json:"start"`
	End   int64  `json:"end"`
}

type VideoMoment struct {
	Label     string `json:"label"`
	Timestamp int64  `json:"timestamp"`
}

type VideoMetaTag struct {
	Property string `json:"property"`
	Value    string `json:"value"`
}

type UpdateVideoInput struct {
	LibraryID    string         `json:"-"`
	VideoID      string         `json:"-"`
	Title        *string        `json:"title,omitempty"`
	CollectionID *string        `json:"collectionId,omitempty"`
	Chapters     []VideoChapter `json:"chapters,omitempty"`
	Moments      []VideoMoment  `json:"moments,omitempty"`
	MetaTags     []VideoMetaTag `json:"metaTags,omitempty"`
}

type CreateVideoInput struct {
	LibraryID     string  `json:"-"`
	Title         string  `json:"title"`
	CollectionID  *string `json:"collectionId,omitempty"`
	ThumbnailTime *int64  `json:"thumbnailTime,omitempty"`
}

type UploadVideoInput struct {
	LibraryID string
	VideoID   string
	Data      io.Reader
}

// See https://docs.bunny.net/docs/stream-embed-token-authentication
func (client *Client) SignVideoUrl(videoId string, expiresAt time.Time) (token, expiresAtStr string) {
	expiresAtStr = strconv.FormatInt(expiresAt.Unix(), 10)

	data := append([]byte(client.streamApiKey), []byte(videoId)...)
	data = append(data, []byte(expiresAtStr)...)
	signature := sha256.Sum256(data)

	token = hex.EncodeToString(signature[:])
	return
}

// See here for the avilable options: https://docs.bunny.net/docs/stream-embedding-videos#parameters
type GenerateIframeVideoUrlOptions struct {
	Signed bool
	// If Signed is true and Expires is nil we use a default value of 24 hours
	Expires  *time.Time
	Autoplay *bool
	// We recommend to set to false
	TrackView *bool
	// We recommend to set to true
	Preload        *bool
	Captions       *string
	VideoStartTime *uint64
	Refresh        *bool
}

// GenerateIframeVideoUrl generates the URL of the given video to be embeded with an iframe
// See https://docs.bunny.net/docs/stream-embedding-videos for more information
// See https://docs.bunny.net/docs/stream-embed-token-authentication for documentation about authentication
func (client *Client) GenerateIframeVideoUrl(libraryID string, videoID string, options *GenerateIframeVideoUrlOptions) (videoUrl string) {
	var queryValue string
	videoUrl = fmt.Sprintf("https://iframe.mediadelivery.net/embed/%s/%s", libraryID, videoID)

	if options != nil {
		queryParms := url.Values{}

		if options.TrackView != nil {
			queryParms.Set("trackView", strconv.FormatBool(*options.TrackView))
		}
		if options.Refresh != nil {
			queryParms.Set("refresh", strconv.FormatBool(*options.Refresh))
		}
		if options.Autoplay != nil {
			queryParms.Set("autoplay", strconv.FormatBool(*options.Autoplay))
		}
		if options.Preload != nil {
			queryParms.Set("preload", strconv.FormatBool(*options.Preload))
		}
		if options.Captions != nil {
			queryParms.Set("captions", *options.Captions)
		}
		if options.VideoStartTime != nil {
			queryParms.Set("t", strconv.FormatUint(*options.VideoStartTime, 10))
		}

		if options.Signed {
			var expiresAt time.Time
			if options.Expires != nil {
				expiresAt = *options.Expires
			} else {
				expiresAt = time.Now().UTC().Add(24 * time.Hour)
			}
			token, expiresAtStr := client.SignVideoUrl(videoID, expiresAt)
			queryParms.Set("token", token)
			queryParms.Set("expires", expiresAtStr)
		}

		queryValue = queryParms.Encode()
	}

	if queryValue != "" {
		videoUrl += "?" + queryValue
	}

	return
}

// https://docs.bunny.net/reference/video_fetchnewvideo
func (client *Client) FetchVideo(ctx context.Context, libraryID, videoUrl string) (output FetchVideoOutput, err error) {
	err = client.request(ctx, requestParams{
		Payload: FetchVideoInput{
			Url: videoUrl,
		},
		Method:          http.MethodPost,
		URL:             fmt.Sprintf("%s/library/%s/videos/fetch", client.streamApiBaseUrl, libraryID),
		useStreamApiKey: true,
	}, &output)

	return
}

// https://docs.bunny.net/reference/video_deletevideo
func (client *Client) DeleteVideo(ctx context.Context, libraryID, videoID string) (err error) {
	err = client.request(ctx, requestParams{
		Payload:         nil,
		Method:          http.MethodDelete,
		URL:             fmt.Sprintf("%s/library/%s/videos/%s", client.streamApiBaseUrl, libraryID, videoID),
		useStreamApiKey: true,
	}, nil)

	return
}

// https://docs.bunny.net/reference/video_reencodevideo
func (client *Client) ReencodeVideo(ctx context.Context, libraryID, videoID string) (video Video, err error) {
	err = client.request(ctx, requestParams{
		Payload:         nil,
		Method:          http.MethodPost,
		URL:             fmt.Sprintf("%s/library/%s/videos/%s/reencode", client.streamApiBaseUrl, libraryID, videoID),
		useStreamApiKey: true,
	}, &video)

	return
}

// https://docs.bunny.net/reference/video_getvideo
func (client *Client) GetVideo(ctx context.Context, libraryID, videoID string) (video Video, err error) {
	err = client.request(ctx, requestParams{
		Payload:         nil,
		Method:          http.MethodGet,
		URL:             fmt.Sprintf("%s/library/%s/videos/%s", client.streamApiBaseUrl, libraryID, videoID),
		useStreamApiKey: true,
	}, &video)

	return
}

// https://docs.bunny.net/reference/video_updatevideo
func (client *Client) UpdateVideo(ctx context.Context, input UpdateVideoInput) (err error) {
	err = client.request(ctx, requestParams{
		Payload:         nil,
		Method:          http.MethodPost,
		URL:             fmt.Sprintf("%s/library/%s/videos/%s", client.streamApiBaseUrl, input.LibraryID, input.VideoID),
		useStreamApiKey: true,
	}, nil)

	return
}

// https://docs.bunny.net/reference/video_createvideo
func (client *Client) CreateVideo(ctx context.Context, input CreateVideoInput) (output Video, err error) {
	err = client.request(ctx, requestParams{
		Payload:         input,
		Method:          http.MethodPost,
		URL:             fmt.Sprintf("%s/library/%s/videos", client.streamApiBaseUrl, input.LibraryID),
		useStreamApiKey: true,
	}, &output)

	return
}

// https://docs.bunny.net/reference/video_createvideo
func (client *Client) UploadVideo(ctx context.Context, input UploadVideoInput) (output UploadVideoOutput, err error) {
	err = client.upload(ctx, uploadParams{
		Data:            input.Data,
		Method:          http.MethodPut,
		URL:             fmt.Sprintf("%s/library/%s/videos/%s", client.streamApiBaseUrl, input.LibraryID, input.VideoID),
		useStreamApiKey: true,
	}, &output)

	return
}
