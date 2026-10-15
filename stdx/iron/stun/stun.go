// Package stun implements the subset of RFC 5389 needed for NAT address
// discovery: a Binding request and the XOR-MAPPED-ADDRESS attribute of the
// Binding response.
//
// A client sends a Binding request from its direct UDP socket to a STUN
// server (here: the relay's UDP listener). The server replies with the
// address it observed, i.e. the client's post-NAT IP:port. That is the
// address peers must punch to.
package stun

import (
	"crypto/rand"
	"encoding/binary"
	"errors"
	"net"
)

const (
	// magicCookie is the fixed value every STUN message carries after the
	// type and length fields; it is also the key for the XOR transforms.
	magicCookie = 0x2112a442

	// bindingRequest and bindingResponse are the STUN message types used here.
	bindingRequest  = 0x0001
	bindingResponse = 0x0101

	// attrXORMappedAddress is the attribute carrying the observed address.
	attrXORMappedAddress = 0x0020

	// headerLen is the STUN message header size.
	headerLen = 20
)

// TransactionID is the 96-bit STUN transaction identifier.
type TransactionID [12]byte

// NewTransactionID returns a fresh random transaction identifier.
func NewTransactionID() (TransactionID, error) {
	var id TransactionID
	if _, err := rand.Read(id[:]); err != nil {
		return TransactionID{}, err
	}
	return id, nil
}

// EncodeBindingRequest builds a Binding request carrying the given
// transaction identifier.
func EncodeBindingRequest(id TransactionID) []byte {
	b := make([]byte, headerLen)
	binary.BigEndian.PutUint16(b[0:2], bindingRequest)
	binary.BigEndian.PutUint32(b[4:8], magicCookie)
	copy(b[8:20], id[:])
	return b
}

// ParseHeader extracts the message type and transaction identifier from a
// STUN message.
func ParseHeader(msg []byte) (msgType uint16, id TransactionID, err error) {
	if len(msg) < headerLen {
		return 0, TransactionID{}, errors.New("stun: message too short")
	}
	if binary.BigEndian.Uint32(msg[4:8]) != magicCookie {
		return 0, TransactionID{}, errors.New("stun: bad magic cookie")
	}
	msgType = binary.BigEndian.Uint16(msg[0:2])
	copy(id[:], msg[8:20])
	return msgType, id, nil
}

// IsBindingRequest reports whether msg is a STUN Binding request.
func IsBindingRequest(msg []byte) bool {
	t, _, err := ParseHeader(msg)
	return err == nil && t == bindingRequest
}

// IsBindingResponse reports whether msg is a STUN Binding response.
func IsBindingResponse(msg []byte) bool {
	t, _, err := ParseHeader(msg)
	return err == nil && t == bindingResponse
}

// EncodeXORMappedAddress builds a Binding response to the given request,
// carrying the observed address XOR-masked per RFC 5389 section 15.2.
func EncodeXORMappedAddress(id TransactionID, addr *net.UDPAddr) []byte {
	value := make([]byte, 0, 20)
	if ip4 := addr.IP.To4(); ip4 != nil {
		value = append(value, 0, 0x01)
		value = binary.BigEndian.AppendUint16(value, uint16(addr.Port)^(magicCookie>>16))
		key := binary.BigEndian.AppendUint32(nil, magicCookie)
		for i, b := range ip4 {
			value = append(value, b^key[i])
		}
	} else {
		value = append(value, 0, 0x02)
		value = binary.BigEndian.AppendUint16(value, uint16(addr.Port)^(magicCookie>>16))
		var key [16]byte
		binary.BigEndian.PutUint32(key[:4], magicCookie)
		copy(key[4:], id[:])
		ip6 := addr.IP.To16()
		for i, b := range ip6 {
			value = append(value, b^key[i])
		}
	}

	msg := make([]byte, headerLen+4+len(value))
	binary.BigEndian.PutUint16(msg[0:2], bindingResponse)
	binary.BigEndian.PutUint16(msg[2:4], uint16(4+len(value)))
	binary.BigEndian.PutUint32(msg[4:8], magicCookie)
	copy(msg[8:20], id[:])
	binary.BigEndian.PutUint16(msg[20:22], attrXORMappedAddress)
	binary.BigEndian.PutUint16(msg[22:24], uint16(len(value)))
	copy(msg[24:], value)
	return msg
}

// ParseXORMappedAddress decodes the XOR-MAPPED-ADDRESS attribute of a Binding
// response. tid is the transaction identifier from the message header.
func ParseXORMappedAddress(msg []byte, tid TransactionID) (*net.UDPAddr, error) {
	if len(msg) < headerLen {
		return nil, errors.New("stun: message too short")
	}
	attr := headerLen
	for attr+4 <= len(msg) {
		attrType := binary.BigEndian.Uint16(msg[attr : attr+2])
		attrLen := int(binary.BigEndian.Uint16(msg[attr+2 : attr+4]))
		if attrStart := attr + 4; attrStart+attrLen <= len(msg) {
			if attrType == attrXORMappedAddress {
				return decodeXORAddr(msg[attrStart:attrStart+attrLen], tid)
			}
		}
		// Attributes are padded to a 4-byte boundary.
		attr += 4 + (attrLen+3)&^3
	}
	return nil, errors.New("stun: no XOR-MAPPED-ADDRESS attribute")
}

func decodeXORAddr(value []byte, tid TransactionID) (*net.UDPAddr, error) {
	if len(value) < 4 {
		return nil, errors.New("stun: malformed XOR-MAPPED-ADDRESS")
	}
	family := value[1]
	port := int(binary.BigEndian.Uint16(value[2:4]) ^ (magicCookie >> 16))
	switch family {
	case 0x01:
		if len(value) < 8 {
			return nil, errors.New("stun: short ipv4 XOR address")
		}
		key := binary.BigEndian.AppendUint32(nil, magicCookie)
		ip := make(net.IP, 4)
		for i := range ip {
			ip[i] = value[4+i] ^ key[i]
		}
		return &net.UDPAddr{IP: ip, Port: port}, nil
	case 0x02:
		if len(value) < 20 {
			return nil, errors.New("stun: short ipv6 XOR address")
		}
		var key [16]byte
		binary.BigEndian.PutUint32(key[:4], magicCookie)
		copy(key[4:], tid[:])
		ip := make(net.IP, 16)
		for i := range ip {
			ip[i] = value[4+i] ^ key[i]
		}
		return &net.UDPAddr{IP: ip, Port: port}, nil
	default:
		return nil, errors.New("stun: unknown address family")
	}
}
