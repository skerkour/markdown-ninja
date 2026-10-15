package organizations

import (
	"markdown.ninja/pkg/services/content"
	"markdown.ninja/stdx/guid"
)

type BillingGatedAction interface {
	isBillingGated()
}

type BillingGatedActionCreateWebsite struct{}

func (BillingGatedActionCreateWebsite) isBillingGated() {}

type BillingGatedActionUpdateWebsite struct {
	PoweredBy bool
	Ad        *string
}

func (BillingGatedActionUpdateWebsite) isBillingGated() {}

type BillingGatedActionInviteStaffs struct {
	// The number of new staffs to invite
	NewStaffs int64
}

func (BillingGatedActionInviteStaffs) isBillingGated() {}

type BillingGatedActionSendNewsletter struct{}

func (BillingGatedActionSendNewsletter) isBillingGated() {}

type BillingGatedActionAddWebsiteCustomDomain struct {
	WebsiteID guid.GUID
}

func (BillingGatedActionAddWebsiteCustomDomain) isBillingGated() {}

type BillingGatedActionSetupCustomEmailDomain struct{}

func (BillingGatedActionSetupCustomEmailDomain) isBillingGated() {}

type BillingGatedActionUploadAsset struct {
	NewAssetSize int64
	WebsiteID    guid.GUID
	AssetType    content.AssetType
}

func (BillingGatedActionUploadAsset) isBillingGated() {}

type BillingGatedActionCreatePage struct {
	WebsiteID guid.GUID
}

func (BillingGatedActionCreatePage) isBillingGated() {}

// const (
// 	BillingGatedActionCreateWebsite BillingGatedAction = iota
// 	BillingGatedActionInviteStaffs
// 	BillingGatedActionUploadAsset
// )
