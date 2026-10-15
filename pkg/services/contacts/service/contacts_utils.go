package service

import (
	"fmt"
	"net/url"
	"strings"
	"time"

	"markdown.ninja/pkg/jwt"
	"markdown.ninja/pkg/services/contacts"
	"markdown.ninja/stdx/guid"
)

const (
	jwtActionUnsubscribe = "unsubscribe"
	jwtActionUpdateEmail = "update_email"
)

type jwtClaimsUnsubscribe struct {
	Action    string    `json:"action"`
	ContactID guid.GUID `json:"contact_id"`
}

type jwtClaimsUpdateEmail struct {
	Action    string    `json:"action"`
	ContactID guid.GUID `json:"contact_id"`
	OldEmail  string    `json:"old_email"`
	NewEmail  string    `json:"new_email"`
}

func (service *ContactsService) GenerateUnsubscribeLink(websiteDomain string, contactID guid.GUID) (unsubscribeLink string, err error) {
	jwtClaims := jwtClaimsUnsubscribe{
		Action:    jwtActionUnsubscribe,
		ContactID: contactID,
	}
	expiresAt := time.Now().UTC().Add(time.Hour * 96)
	jwt, err := service.jwtProvider.NewSignedToken(jwtClaims, &jwt.TokenOptions{
		ExpirationTime: &expiresAt,
	})
	if err != nil {
		err = fmt.Errorf("contacts: generating unsubscribe token: %w", err)
		return
	}

	query := url.Values{}
	query.Add("token", jwt)

	linkUrl := url.URL{
		Scheme:   service.httpConfig.WebsitesBaseUrl.Scheme,
		Host:     fmt.Sprintf("%s%s", websiteDomain, service.httpConfig.WebsitesPort),
		Path:     "/unsubscribe",
		RawQuery: query.Encode(),
	}
	return linkUrl.String(), nil
}

func (service *ContactsService) ParseAndVerifyUnsubscribeToken(token string) (contactID guid.GUID, err error) {
	var jwtClaims jwtClaimsUnsubscribe

	err = service.jwtProvider.ParseAndVerifyToken(token, &jwtClaims)
	if err != nil {
		err = contacts.ErrUnsubscribeLinkIsNotValid
		return
	}

	if jwtClaims.Action != jwtActionUnsubscribe {
		err = contacts.ErrUnsubscribeLinkIsNotValid
		return
	}

	return jwtClaims.ContactID, nil
}

// func (service *ContactsService) GenerateUnsubscribeLink(websiteDomain string, contactID guid.GUID, contactMasterKey [crypto.Size256]byte) (url string, err error) {
// 	unsubscribeTokenBytes, err := generateUnsubscribeToken(contactID, contactMasterKey)
// 	if err != nil {
// 		return
// 	}

// 	return fmt.Sprintf("%s://%s/unsubscribe?contact=%s&token=%s",
// 		service.kernel.HttpProtocol(), websiteDomain, contactID.String(),
// 		hex.EncodeToString(unsubscribeTokenBytes[:])), nil
// }

// func generateUnsubscribeToken(contactID guid.GUID, contactMasterKey [crypto.Size256]byte) (unsubscribeToken []byte, err error) {
// 	var unsubscribeHmacKey [crypto.KeySize256]byte

// 	hkdf := hkdf.New(sha256.New, contactMasterKey[:], contactID.Bytes(), []byte(contacts.KeyInfoUnsubscribe))
// 	_, err = io.ReadFull(hkdf, unsubscribeHmacKey[:])
// 	if err != nil {
// 		return
// 	}

// 	unsubscribeHmac := hmac.New(sha256.New, unsubscribeHmacKey[:])
// 	unsubscribeHmac.Write(contactID.Bytes())
// 	unsubscribeToken = unsubscribeHmac.Sum(nil)
// 	return
// }

// func (service *ContactsService) VerifyUnsubscribeToken(contactID guid.GUID, contactMasterKey [crypto.Size256]byte, token string) (err error) {
// 	inputTokenBytes, err := hex.DecodeString(token)
// 	if err != nil {
// 		err = contacts.ErrUnsubscribeTokenIsNotValid
// 		return
// 	}

// 	unsubscribeToken, err := generateUnsubscribeToken(contactID, contactMasterKey)
// 	if err != nil {
// 		err = contacts.ErrUnsubscribeTokenIsNotValid
// 		return
// 	}

// 	if !crypto.ConstantTimeCompare(inputTokenBytes, unsubscribeToken[:]) {
// 		err = contacts.ErrUnsubscribeTokenIsNotValid
// 		return
// 	}

// 	return
// }

func (service *ContactsService) GenerateVerifyEmailLink(websiteDomain string, contactID guid.GUID, oldEmail, newEmail string) (link string, err error) {
	jwtClaims := jwtClaimsUpdateEmail{
		Action:    jwtActionUpdateEmail,
		ContactID: contactID,
		NewEmail:  newEmail,
		OldEmail:  oldEmail,
	}
	expiresAt := time.Now().UTC().Add(1 * time.Hour)
	jwt, err := service.jwtProvider.NewSignedToken(jwtClaims, &jwt.TokenOptions{
		ExpirationTime: &expiresAt,
	})
	if err != nil {
		err = fmt.Errorf("contacts: generating update email token: %w", err)
		return
	}

	query := url.Values{}
	query.Add("update-email-token", jwt)

	linkUrl := url.URL{
		Scheme:   service.httpConfig.WebsitesBaseUrl.Scheme,
		Host:     fmt.Sprintf("%s%s", websiteDomain, service.httpConfig.WebsitesPort),
		Path:     "/account",
		RawQuery: query.Encode(),
	}
	return linkUrl.String(), nil
}

func (service *ContactsService) extractNameFromEmail(email string) string {
	emailParts := strings.Split(email, "@")
	return emailParts[0]
}
