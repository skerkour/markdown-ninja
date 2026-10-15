package contacts

import (
	"context"
	"net/http"

	"markdown.ninja/pkg/services/kernel"
	"markdown.ninja/stdx/db"
	"markdown.ninja/stdx/guid"
)

type Service interface {
	// Contacts
	CurrentContact(ctx context.Context) (contact *Contact)
	CreateContact(ctx context.Context, input CreateContactInput) (contact Contact, err error)
	CreateContactInternal(ctx context.Context, db db.Queryer, input CreateContactInternalInput) (contact Contact, err error)
	UpdateContact(ctx context.Context, input UpdateContactInput) (contact Contact, err error)
	UpdateContactInternal(ctx context.Context, db db.Queryer, contact *Contact, input UpdateContactInput) (err error)
	DeleteContact(ctx context.Context, input DeleteContactInput) (err error)
	ListContacts(ctx context.Context, input ListContactsInput) (contacts kernel.PaginatedResult[Contact], err error)
	GetContact(ctx context.Context, input GetContactInput) (contact Contact, err error)
	ImportContacts(ctx context.Context, input ImportContactsInput) (contacts []Contact, err error)
	FindVerifiedAndSubscribedToNewsletterContacts(ctx context.Context, db db.Queryer, websiteID guid.GUID) (contacts []Contact, err error)
	GetVerifiedAndSubscribedToNewsletterContactsCount(ctx context.Context, db db.Queryer, websiteID guid.GUID) (count int64, err error)
	FindContactByEmail(ctx context.Context, db db.Queryer, websiteID guid.GUID, email string) (contact Contact, err error)
	FindContact(ctx context.Context, db db.Queryer, contactID guid.GUID) (contact Contact, err error)
	FindOrCreateContact(ctx context.Context, db db.Queryer, websiteID guid.GUID, email string, subscribedToNewsletter bool) (contact Contact, err error)
	FindContactsByEmail(ctx context.Context, db db.Queryer, websiteID guid.GUID, emails []string) (contacts []Contact, err error)
	ValidateContactName(name string) (err error)
	GenerateUnsubscribeLink(websiteDomain string, contactID guid.GUID) (url string, err error)
	// VerifyUnsubscribeToken(contactID guid.GUID, contactMasterKey [crypto.Size256]byte, token string) (err error)
	GenerateVerifyEmailLink(websiteDomain string, contactID guid.GUID, oldEmail, newEmail string) (link string, err error)
	ExportContacts(ctx context.Context, input ExportContactsInput) (contacts ExportContactsOutput, err error)
	// ExportContactsForProduct export the list of email of all contacts who have access to the given product
	ExportContactsForProduct(ctx context.Context, input ExportContactsForProductInput) (res ExportContactsForProductOutput, err error)
	VerifyEmail(ctx context.Context, input VerifyEmailInput) (err error)
	BlockContact(ctx context.Context, input BlockContactInput) (contact Contact, err error)
	UnblockContact(ctx context.Context, input UnblockContactInput) (contact Contact, err error)
	ParseAndVerifyUnsubscribeToken(token string) (contactID guid.GUID, err error)
	DeleteContactInternal(ctx context.Context, db db.Queryer, contactID, websiteID guid.GUID) (err error)

	// Sessions
	VerifySessionToken(ctx context.Context, token string) (contactAndSession ContactAndSession, err error)
	GenerateLogoutCookie() (cookie http.Cookie)
	FindSessionByID(ctx context.Context, db db.Queryer, sessionID guid.GUID) (session Session, err error)
	DeleteSession(ctx context.Context, db db.Queryer, sessionID guid.GUID) (err error)
	// CreateSession returns cookie only if session is verified
	CreateSession(ctx context.Context, db db.Queryer, input CreateSessionInput) (session Session, cookie *http.Cookie, err error)
	MarkSessionAsVerified(ctx context.Context, db db.Queryer, input *Session) (sessionCookie http.Cookie, err error)
	FailSessionLoginAttempt(ctx context.Context, db db.Queryer, session Session) (err error)
	// DeleteOlderVerifiedSessionsForContact deletes the oldest N sessions for the given contact to avoid
	// having too much active sessions and avoid spam
	DeleteOlderVerifiedSessionsForContact(ctx context.Context, db db.Queryer, contactID guid.GUID) (err error)

	// Jobs
	// JobDeleteOldUnverifiedContacts(ctx context.Context, data JobDeleteOldUnverifiedContacts) (err error)
	JobDeleteOldUnverifiedSessions(ctx context.Context, data JobDeleteOldUnverifiedSessions) (err error)
	JobUpdateStripeContact(ctx context.Context, data JobUpdateStripeContact) (err error)
	JobSendVerifyEmailEmail(ctx context.Context, data JobSendVerifyEmailEmail) (err error)
	JobSyncUnsubscribedContacts(ctx context.Context, data JobSyncUnsubscribedContacts) (err error)

	// Tasks
	// TaskDeleteOldUnverifiedContacts(ctx context.Context) (err error)
	TaskDeleteOldUnverifiedSessions(ctx context.Context)
	TaskSyncUnsubscribedContacts(ctx context.Context)
}
