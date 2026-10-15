package base

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strconv"
	"strings"
)

// httpRequestSignatureDomain separates HTTP request signatures from other uses
// of the node's key (e.g. the relay WebSocket handshake).
const httpRequestSignatureDomain = "iron-relay http request signature v1"

// HTTPAuthScheme is the scheme prefix of the Authorization header used for
// signed HTTP requests to a relay.
const HTTPAuthScheme = "Iron"

// HTTPRequestSignatureTTL is how long a signed request stays valid (replay
// window). Both the client and the relay use it.
const HTTPRequestSignatureTTL = 300

// HTTPRequestMessage builds the canonical message signed for an HTTP request.
// It binds the method, the request URI (path + query), the request body and a
// timestamp, so a valid signature can only apply to the exact request.
func HTTPRequestMessage(method, requestURI string, body []byte, ts int64) []byte {
	sum := sha256.Sum256(body)
	return []byte(httpRequestSignatureDomain + "\n" +
		method + "\n" +
		requestURI + "\n" +
		hex.EncodeToString(sum[:]) + "\n" +
		strconv.FormatInt(ts, 10))
}

// SignHTTPRequest signs an HTTP request with the node's secret key, returning
// the Ed25519 signature. Use with BuildAuthHeader to form the Authorization
// header value.
func SignHTTPRequest(secret *NodeSecret, method, requestURI string, body []byte, ts int64) []byte {
	msg := HTTPRequestMessage(method, requestURI, body, ts)
	sig := secret.Sign(msg)
	return sig[:]
}

// VerifyHTTPRequest checks that sig is a valid signature over the given HTTP
// request by the node identified by id.
func VerifyHTTPRequest(id NodeID, method, requestURI string, body []byte, ts int64, sig []byte) bool {
	if len(sig) != 64 {
		return false
	}
	return Verify(id, HTTPRequestMessage(method, requestURI, body, ts), sig)
}

// BuildAuthHeader renders the Authorization header value for a signed request:
//
//	Iron <nodeID> <unix-seconds> <base64url(signature)>
func BuildAuthHeader(id NodeID, ts int64, sig []byte) string {
	return HTTPAuthScheme + " " +
		id.String() + " " +
		strconv.FormatInt(ts, 10) + " " +
		base64.RawURLEncoding.EncodeToString(sig)
}

// ParseAuthHeader parses the Authorization header produced by BuildAuthHeader,
// returning the node id, the timestamp and the signature.
func ParseAuthHeader(value string) (id NodeID, ts int64, sig []byte, err error) {
	fields := strings.Fields(value)
	if len(fields) != 4 || fields[0] != HTTPAuthScheme {
		return NodeID{}, 0, nil, errors.New("bad authorization header")
	}
	id, err = NodeIDFromString(fields[1])
	if err != nil {
		return NodeID{}, 0, nil, errors.New("bad authorization node id")
	}
	ts, err = strconv.ParseInt(fields[2], 10, 64)
	if err != nil {
		return NodeID{}, 0, nil, errors.New("bad authorization timestamp")
	}
	sig, err = base64.RawURLEncoding.DecodeString(fields[3])
	if err != nil {
		return NodeID{}, 0, nil, errors.New("bad authorization signature")
	}
	return id, ts, sig, nil
}
