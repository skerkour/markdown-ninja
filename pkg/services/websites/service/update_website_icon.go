package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"image/png"
	"io"
	"net/http"
	"time"

	"github.com/zeebo/blake3"
	"markdown.ninja/pkg/errs"
	"markdown.ninja/pkg/services/websites"
	"markdown.ninja/pkg/storage"
	"markdown.ninja/stdx/db"
	"markdown.ninja/stdx/guid"
	"markdown.ninja/stdx/httpx"
	"markdown.ninja/stdx/imaging"
	"markdown.ninja/stdx/retry"
)

func (service *WebsitesService) UpdateWebsiteIcon(ctx context.Context, input websites.UpdateWebsiteIconInput) (err error) {
	actorID, err := service.kernel.CurrentUserID(ctx)
	if err != nil {
		return err
	}

	website, err := service.repo.FindWebsiteByID(ctx, service.db, input.WebsiteID, false)
	if err != nil {
		return err
	}

	_, err = service.organizationsService.CheckUserIsStaff(ctx, service.db, actorID, website.OrganizationID)
	if err != nil {
		return err
	}

	// load image in memory
	imageBuffer := bytes.NewBuffer(make([]byte, 0, 500_000))

	originalImageBytesSize, err := io.CopyN(imageBuffer, input.Data, websites.WebsiteIconMaxSize+1)
	if err == nil && originalImageBytesSize != websites.WebsiteIconMaxSize {
		return errs.InvalidArgument(fmt.Sprintf("Image is too large. Max size: %d MB", websites.WebsiteIconMaxSize/1_000_000))
	} else if err != nil && err != io.EOF {
		return fmt.Errorf("website.UpdateWebsiteIcon: reading uploaded file: %w", err)
	}
	err = nil

	originalImageHash := blake3.Sum256(imageBuffer.Bytes())

	resizedImages := make(map[int][]byte, len(websites.WebsiteIconSizes))

	// validate image
	contentTypeFromHttp := http.DetectContentType(imageBuffer.Bytes())
	if contentTypeFromHttp != httpx.MediaTypePNG {
		return websites.ErrWebsiteIconIsNotValid
	}

	pngImage, err := png.Decode(bytes.NewReader(imageBuffer.Bytes()))
	if err != nil {
		return websites.ErrWebsiteIconIsNotValid
	}

	pngImageBounds := pngImage.Bounds()
	if pngImageBounds.Dx() != pngImageBounds.Dy() {
		return websites.ErrWebsiteIconIsNotValid
	}
	if pngImageBounds.Dx() < 256 {
		return websites.ErrWebsiteIconIsNotValid
	}

	// resize image to multiple dimensions
	for size := range websites.WebsiteIconSizes.Iter() {
		resizedImage := imaging.Resize(pngImage, size, 0, imaging.Lanczos)
		// resizedImage = imaging.AdjustGamma(resizedImage, 1.25)

		resizedImageBuffer := bytes.NewBuffer(make([]byte, 0, imageBuffer.Len()))
		err = png.Encode(resizedImageBuffer, resizedImage)
		if err != nil {
			return fmt.Errorf("resizing image: %w", err)
		}
		resizedImages[size] = resizedImageBuffer.Bytes()
	}

	// upload everything to S3
	for iconSize, resizedImage := range resizedImages {
		// used for S3 data-integrity checks
		imaheSha256ForS3 := sha256.Sum256(resizedImage)
		putObjectOptions := &storage.PutObjectOptions{
			// Metadata:    objectMetadata,
			HashSha256: imaheSha256ForS3[:],
		}
		storageKey := generateStorageKeyForWebsiteIcon(website.ID, iconSize)

		err = retry.Do(func() (retryErr error) {
			resizedImageReader := bytes.NewReader(resizedImage)
			return service.storage.PutObject(ctx, storageKey, int64(len(resizedImage)), resizedImageReader, putObjectOptions)
		}, retry.Context(ctx), retry.Attempts(3), retry.Delay(50*time.Millisecond))
		if err != nil {
			return fmt.Errorf("writing file to storage: %w", err)
		}
	}

	// update website
	err = service.db.Transaction(ctx, func(tx db.Tx) (txErr error) {
		website, txErr = service.repo.FindWebsiteByID(ctx, tx, website.ID, true)
		if txErr != nil {
			return txErr
		}

		website.UpdatedAt = time.Now().UTC()
		website.CustomIcon = true
		website.CustomIconHash = originalImageHash[:]
		return service.repo.UpdateWebsite(ctx, tx, website)
	})
	if err != nil {
		return
	}

	return
}

func generateStorageKeyForWebsiteIcon(websiteID guid.GUID, iconSize int) string {
	return fmt.Sprintf("websites/%s/icon-%d.png", websiteID.String(), iconSize)
}
