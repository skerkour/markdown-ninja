// Package base holds the core identity types shared across iron.
package base

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base32"
	"errors"
	"strings"
)

// NodeIDLen is the size in bytes of a NodeID (an Ed25519 public key).
const NodeIDLen = 32

var encoding = base32.HexEncoding.WithPadding(base32.NoPadding)

// NodeID is the public identity of a node: its Ed25519 public key.
// This is the value you "dial" in iron.
type NodeID [NodeIDLen]byte

// String returns the lower-case base32hex encoding of the node id.
func (id NodeID) String() string {
	return strings.ToLower(encoding.EncodeToString(id[:]))
}

// Name returns the synthetic DNS name used as the TLS server name (SNI)
// when dialing this node. Keeping the node id in the SNI buckets 0-RTT
// session tickets per peer.
func (id NodeID) Name() string { return id.String() + ".iron.invalid" }

// NodeIDFromBytes parses a raw 32-byte node id.
func NodeIDFromBytes(b []byte) (NodeID, error) {
	if len(b) != NodeIDLen {
		return NodeID{}, errors.New("node id must be exactly 32 bytes")
	}
	var id NodeID
	copy(id[:], b)
	return id, nil
}

// NodeIDFromString parses the base32hex encoding produced by NodeID.String.
func NodeIDFromString(s string) (NodeID, error) {
	b, err := encoding.DecodeString(strings.ToUpper(s))
	if err != nil {
		return NodeID{}, err
	}
	return NodeIDFromBytes(b)
}

// NodeSecret is a node's secret identity: an Ed25519 key pair.
type NodeSecret struct {
	priv ed25519.PrivateKey
}

// NewNodeSecret generates a fresh random identity.
func NewNodeSecret() (*NodeSecret, error) {
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	return &NodeSecret{priv: priv}, nil
}

// NewNodeSecretFromBytes restores a node secret from its 64-byte Ed25519
// private key (seed || public key).
func NewNodeSecretFromBytes(priv []byte) (*NodeSecret, error) {
	if len(priv) != ed25519.PrivateKeySize {
		return nil, errors.New("private key must be 64 bytes")
	}
	return &NodeSecret{priv: ed25519.PrivateKey(append([]byte(nil), priv...))}, nil
}

// Public returns the node's public identity.
func (n *NodeSecret) Public() NodeID {
	var id NodeID
	copy(id[:], n.priv.Public().(ed25519.PublicKey))
	return id
}

// Sign signs msg with the node's private key.
func (n *NodeSecret) Sign(msg []byte) [64]byte {
	var sig [64]byte
	copy(sig[:], ed25519.Sign(n.priv, msg))
	return sig
}

// PublicKey exposes the underlying Ed25519 public key (for X.509 certs).
func (n *NodeSecret) PublicKey() ed25519.PublicKey {
	return n.priv.Public().(ed25519.PublicKey)
}

// PrivateKey exposes the underlying Ed25519 private key (for X.509 certs).
func (n *NodeSecret) PrivateKey() ed25519.PrivateKey {
	return n.priv
}

// Verify checks a signature against a peer's public identity.
func Verify(id NodeID, msg, sig []byte) bool {
	return ed25519.Verify(id[:], msg, sig)
}
