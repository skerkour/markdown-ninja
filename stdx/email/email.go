// Package email provides an easy to use and hard to misuse email API
package email

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net/mail"
	"net/smtp"
	"net/textproto"
	"strings"
	"time"

	"markdown.ninja/stdx/crypto"
)

const (
	// MaxLineLength is the maximum line length per RFC 2045
	MaxLineLength = 76
	// defaultContentType is the default Content-Type according to RFC 2045, section 5.2
	defaultContentType = "text/plain; charset=us-ascii"
)

// ErrMissingBoundary is returned when there is no boundary given for a multipart entity
var ErrMissingBoundary = errors.New("No boundary found for multipart entity")

// ErrMissingContentType is returned when there is no "Content-Type" header for a MIME entity
var ErrMissingContentType = errors.New("No Content-Type found for MIME entity")

var defaultMailer *SMTPMailer

// Email is an email...
// either Text or HTML must be provided
type Email struct {
	ReplyTo     []mail.Address
	From        mail.Address
	To          []mail.Address
	Bcc         []mail.Address
	Cc          []mail.Address
	Subject     string
	Text        []byte // Plaintext message
	HTML        []byte // Html message
	Headers     textproto.MIMEHeader
	Attachments []Attachment
	// ReadReceipt []string
}

// Bytes returns the content of the email in the bytes form
func (email *Email) Bytes() ([]byte, error) {
	buffer := bytes.NewBuffer([]byte{})
	hasAttachements := len(email.Attachments) > 0
	isAlternative := len(email.Text) > 0 && len(email.HTML) > 0
	var multipartWriter *multipart.Writer

	headers, err := email.headers()
	if err != nil {
		return nil, err
	}

	if hasAttachements || isAlternative {
		multipartWriter = multipart.NewWriter(buffer)
	}
	switch {
	case hasAttachements:
		headers.Set("Content-Type", "multipart/mixed;\r\n boundary="+multipartWriter.Boundary())
	case isAlternative:
		headers.Set("Content-Type", "multipart/alternative;\r\n boundary="+multipartWriter.Boundary())
	case len(email.HTML) > 0:
		headers.Set("Content-Type", "text/html; charset=UTF-8")
		headers.Set("Content-Transfer-Encoding", "quoted-printable")
	default:
		headers.Set("Content-Type", "text/plain; charset=UTF-8")
		headers.Set("Content-Transfer-Encoding", "quoted-printable")
	}
	headersToBytes(buffer, headers)
	_, err = io.WriteString(buffer, "\r\n")
	if err != nil {
		return nil, err
	}

	// Check to see if there is a Text or HTML field
	if len(email.Text) > 0 || len(email.HTML) > 0 {
		var subWriter *multipart.Writer

		if hasAttachements && isAlternative {
			// Create the multipart alternative part
			subWriter = multipart.NewWriter(buffer)
			header := textproto.MIMEHeader{
				"Content-Type": {"multipart/alternative;\r\n boundary=" + subWriter.Boundary()},
			}
			if _, err := multipartWriter.CreatePart(header); err != nil {
				return nil, err
			}
		} else {
			subWriter = multipartWriter
		}
		// Create the body sections
		if len(email.Text) > 0 {
			// Write the text
			if err := writeMessage(buffer, email.Text, hasAttachements || isAlternative, "text/plain", subWriter); err != nil {
				return nil, err
			}
		}
		if len(email.HTML) > 0 {
			// Write the HTML
			if err := writeMessage(buffer, email.HTML, hasAttachements || isAlternative, "text/html", subWriter); err != nil {
				return nil, err
			}
		}
		if hasAttachements && isAlternative {
			if err := subWriter.Close(); err != nil {
				return nil, err
			}
		}
	}
	// Create attachment part, if necessary
	for _, a := range email.Attachments {
		ap, err := multipartWriter.CreatePart(a.Header)
		if err != nil {
			return nil, err
		}
		// Write the base64Wrapped content to the part
		base64Wrap(ap, a.Content)
	}
	if hasAttachements || isAlternative {
		if err := multipartWriter.Close(); err != nil {
			return nil, err
		}
	}
	return buffer.Bytes(), nil
}

func (email *Email) headers() (textproto.MIMEHeader, error) {
	res := textproto.MIMEHeader{}

	// Set default headers
	if len(email.ReplyTo) > 0 {
		res.Set("Reply-To", strings.Join(mailAddressesToStrings(email.ReplyTo), ", "))
	}
	if len(email.To) > 0 {
		res.Set("To", strings.Join(mailAddressesToStrings(email.To), ", "))
	}
	if len(email.Cc) > 0 {
		res.Set("Cc", strings.Join(mailAddressesToStrings(email.Cc), ", "))
	}

	res.Set("Subject", email.Subject)

	id, err := generateMessageID()
	if err != nil {
		return nil, err
	}
	res.Set("Message-Id", id)

	// Set required headers.
	res.Set("From", email.From.String())
	res.Set("Date", time.Now().Format(time.RFC1123Z))
	res.Set("MIME-Version", "1.0")

	// overwrite with user provided headers
	for key, value := range email.Headers {
		res[key] = value
	}
	return res, nil
}

// Attachment is an email attachment.
// Based on the mime/multipart.FileHeader struct, Attachment contains the name, MIMEHeader, and content of the attachment in question
type Attachment struct {
	Filename string
	Header   textproto.MIMEHeader
	Content  []byte
}

// Mailer are used to send email
type SMTPMailer struct {
	smtpAuth    smtp.Auth
	smtpAddress string
}

// SMTPConfig is used to configure an email
type SMTPConfig struct {
	Host     string
	Port     uint16
	Username string
	Password string
}

// Send an email
func (mailer *SMTPMailer) Send(email Email) error {
	if len(email.HTML) == 0 && len(email.Text) == 0 {
		return errors.New("email: either HTML or Text must be provided")
	}

	// Merge the To, Cc, and Bcc fields
	to := make([]mail.Address, 0, len(email.To)+len(email.Cc)+len(email.Bcc))
	to = append(to, email.To...)
	to = append(to, email.Bcc...)
	to = append(to, email.Cc...)

	// Check to make sure there is at least one recipient
	if len(to) == 0 {
		return errors.New("email: Must specify at least one From address and one To address")
	}

	rawEmail, err := email.Bytes()
	if err != nil {
		return err
	}

	toAddresses := make([]string, len(to))
	for i, recipient := range to {
		toAddresses[i] = recipient.Address
	}

	return smtp.SendMail(mailer.smtpAddress, mailer.smtpAuth, email.From.Address, toAddresses, rawEmail)
}

// NewMailer returns a new mailer
func NewSMTPMailer(config SMTPConfig) SMTPMailer {
	smtpAuth := smtp.PlainAuth("", config.Username, config.Password, config.Host)
	return SMTPMailer{
		smtpAuth:    smtpAuth,
		smtpAddress: fmt.Sprintf("%s:%d", config.Host, config.Port),
	}
}

// InitDefaultMailer set the default, global mailer
func InitDefaultMailer(config SMTPConfig) {
	mailer := NewSMTPMailer(config)
	defaultMailer = &mailer
}

// Send an email using the default mailer
func Send(email Email) error {
	if defaultMailer == nil {
		return errors.New("email: defaultMailer has not been initialized")
	}
	return defaultMailer.Send(email)
}

// headersToBytes renders "header" to "buff". If there are multiple values for a
// field, multiple "Field: value\r\n" lines will be emitted.
func headersToBytes(buff io.Writer, headers textproto.MIMEHeader) {
	for field, vals := range headers {
		for _, subval := range vals {
			// bytes.Buffer.Write() never returns an error.
			io.WriteString(buff, field)
			io.WriteString(buff, ": ")
			// Write the encoded header if needed
			switch {
			case field == "Content-Type" || field == "Content-Disposition":
				buff.Write([]byte(subval))
			default:
				buff.Write([]byte(mime.QEncoding.Encode("UTF-8", subval)))
			}
			io.WriteString(buff, "\r\n")
		}
	}
}

// generateMessageID generates and returns a string suitable for an RFC 2822
// compliant Message-ID, email.g.:
// <1444789264909237300.3464.1819418242800517193@DESKTOP01>
//
// The following parameters are used to generate a Message-ID:
// - The nanoseconds since Epoch
// - The calling PID
// - A cryptographically random int64
// - The sending hostname
func generateMessageID() (string, error) {
	randSource := crypto.NewRandomGenerator()
	t := time.Now().UnixNano()

	pid := randSource.Int64N(999)
	rint := randSource.Int64N(999)

	hostname := "localhost.localdomain"
	msgid := fmt.Sprintf("<%d.%d.%d@%s>", t, pid, rint, hostname)
	return msgid, nil
}

func writeMessage(buffer io.Writer, msg []byte, multipart bool, mediaType string, w *multipart.Writer) error {
	if multipart {
		header := textproto.MIMEHeader{
			"Content-Type":              {mediaType + "; charset=UTF-8"},
			"Content-Transfer-Encoding": {"quoted-printable"},
		}
		if _, err := w.CreatePart(header); err != nil {
			return err
		}
	}

	qp := quotedprintable.NewWriter(buffer)
	// Write the text
	if _, err := qp.Write(msg); err != nil {
		return err
	}
	return qp.Close()
}

// base64Wrap encodes the attachment content, and wraps it according to RFC 2045 standards (every 76 chars)
// The output is then written to the specified io.Writer
func base64Wrap(writer io.Writer, b []byte) {
	// 57 raw bytes per 76-byte base64 linemail.
	const maxRaw = 57
	// Buffer for each line, including trailing CRLF.
	buffer := make([]byte, MaxLineLength+len("\r\n"))
	copy(buffer[MaxLineLength:], "\r\n")
	// Process raw chunks until there's no longer enough to fill a linemail.
	for len(b) >= maxRaw {
		base64.StdEncoding.Encode(buffer, b[:maxRaw])
		writer.Write(buffer)
		b = b[maxRaw:]
	}
	// Handle the last chunk of bytes.
	if len(b) > 0 {
		out := buffer[:base64.StdEncoding.EncodedLen(len(b))]
		base64.StdEncoding.Encode(out, b)
		out = append(out, "\r\n"...)
		writer.Write(out)
	}
}

func mailAddressesToStrings(addresses []mail.Address) []string {
	ret := make([]string, len(addresses))

	for i, address := range addresses {
		ret[i] = address.String()
	}
	return ret
}
