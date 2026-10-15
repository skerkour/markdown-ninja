package iron

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"errors"
	"math/big"
	mathrand "math/rand/v2"
	"sync"
	"time"

	"markdown.ninja/stdx/iron/base"
)

// DefaultALPN is the application protocol historically advertised by endpoints.
// It is kept for API compatibility but is no longer placed on the wire: the
// ALPN used for peer connections is the h3 HTTP/3 identifier by default (see
// TLSConfig.ALPN), so traffic blends in with ordinary web HTTP/3. Use
// WithTLSConfig(TLSConfig{ALPN: ...}) to advertise something else.
const DefaultALPN = "iron-example/echo/0"

// defaultALPN is the TLS ALPN identifier advertised on peer connections by
// default, matching what a browser sends for HTTP/3 so iron QUIC connections
// are not distinguishable from ordinary web HTTP/3 at the TLS layer.
const defaultALPN = "h3"

// TLSConfig customizes the TLS settings used for all peer connections. iron
// always negotiates TLS 1.3, whose cipher suites are fixed by the spec, so the
// tunables are the enabled KeyExchange groups, cipher-suite preference order,
// the advertised ALPN (masquerading as HTTP/3 by default) and the pool of
// hostnames used as the per-connection TLS server name (SNI). A zero TLSConfig
// keeps the hardened defaults.
type TLSConfig struct {
	// CurvePreferences lists the KeyExchange groups to offer (client) and
	// accept (server), ordered by preference. nil means the default
	// [X25519, X25519MLKEM768], which looks like a modern browser.
	CurvePreferences []tls.CurveID
	// CipherSuites lists the TLS 1.3 cipher suites to offer, ordered by
	// preference. nil means the default browser-like order
	// [AES-128-GCM, AES-256-GCM, CHACHA20].
	CipherSuites []uint16
	// ALPN is the NextProtos list advertised in the ClientHello. nil means
	// ["h3"]: iron connections are presented as HTTP/3 to blend in with web
	// traffic. Both endpoints must advertise a compatible list.
	ALPN []string
	// SNIHostnames is the pool of hostnames used as the random TLS server
	// name (SNI) on each connection, so the dialed node id never appears on
	// the wire. nil means a built-in pool of realistic-looking hostnames.
	// A random name is picked per connection, including on retries.
	SNIHostnames []string
}

// curvesOrDefault returns cfg.CurvePreferences when non-empty, else the
// default groups [X25519, X25519MLKEM768], mirroring modern browsers. A caller
// that wants to opt out of the hybrid ML-KEM default can pass an explicit
// list.
func curvesOrDefault(cfg TLSConfig) []tls.CurveID {
	if len(cfg.CurvePreferences) > 0 {
		return cfg.CurvePreferences
	}
	return []tls.CurveID{tls.X25519, tls.X25519MLKEM768}
}

// cipherSuitesOrDefault returns cfg.CipherSuites when non-empty, else the
// default TLS 1.3 suites in browser preference order. The negotiated suite is
// chosen by the client's offer; the server's list only constrains what it
// accepts.
func cipherSuitesOrDefault(cfg TLSConfig) []uint16 {
	if len(cfg.CipherSuites) > 0 {
		return cfg.CipherSuites
	}
	return []uint16{
		tls.TLS_AES_128_GCM_SHA256,
		tls.TLS_AES_256_GCM_SHA384,
		tls.TLS_CHACHA20_POLY1305_SHA256,
	}
}

// alpnOrDefault returns cfg.ALPN when non-empty, else ["h3"] so peer
// connections present as HTTP/3.
func alpnOrDefault(cfg TLSConfig) []string {
	if len(cfg.ALPN) > 0 {
		return append([]string(nil), cfg.ALPN...)
	}
	return []string{defaultALPN}
}

// nodeCertificate builds a self-signed X.509 certificate whose key IS the
// node's Ed25519 key. Raw public keys are not supported in Go, so
// authenticating a peer reduces to checking that the presented leaf cert
// carries the expected public key. dnsNames, when given, is set as the
// certificate's DNS subject alternative names (and common name), so the
// certificate matches the SNI it is served for.
func nodeCertificate(secret *base.NodeSecret, dnsNames []string) (tls.Certificate, error) {
	cn := ""
	if len(dnsNames) > 0 {
		cn = dnsNames[0]
	}
	now := time.Now()
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(mathrand.Int64()),
		Subject:      pkix.Name{CommonName: cn},
		DNSNames:     dnsNames,
		NotBefore:    now.Add(-time.Hour),
		NotAfter:     now.Add(90 * 24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, secret.PublicKey(), secret.PrivateKey())
	if err != nil {
		return tls.Certificate{}, err
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: secret.PrivateKey()}, nil
}

// certMinter mints the self-signed certificate for an arbitrary SNI on demand,
// caching per hostname. Every certificate carries the same node Ed25519 key,
// so peer authentication (which only checks the public key) is unaffected.
type certMinter struct {
	secret *base.NodeSecret
	mu     sync.Mutex
	cache  map[string]tls.Certificate
}

// get returns the certificate to present for hello.ServerName, minting it on
// first use. An empty ServerName yields a certificate with no subject alt
// names. The cache is bounded: it is dropped wholesale when it would grow too
// large.
func (m *certMinter) get(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
	name := hello.ServerName
	m.mu.Lock()
	if cert, ok := m.cache[name]; ok {
		m.mu.Unlock()
		return &cert, nil
	}
	m.mu.Unlock()

	var dnsNames []string
	if name != "" {
		dnsNames = []string{name}
	}
	cert, err := nodeCertificate(m.secret, dnsNames)
	if err != nil {
		return nil, err
	}
	m.mu.Lock()
	if len(m.cache) >= 1024 {
		m.cache = make(map[string]tls.Certificate)
	}
	m.cache[name] = cert
	m.mu.Unlock()
	return &cert, nil
}

// serverTLSConfig configures the server side: it presents a certificate minted
// for the client's requested SNI (so the handshake looks consistent with the
// dialed hostname) and demands a client certificate. Any client proving
// ownership of an Ed25519 key is accepted; that key becomes its identity.
func serverTLSConfig(secret *base.NodeSecret, cfg TLSConfig) (*tls.Config, error) {
	minter := &certMinter{secret: secret, cache: make(map[string]tls.Certificate)}
	return &tls.Config{
		MinVersion:       tls.VersionTLS13,
		CipherSuites:     cipherSuitesOrDefault(cfg),
		CurvePreferences: curvesOrDefault(cfg),
		NextProtos:       alpnOrDefault(cfg),
		GetCertificate:   minter.get,
		ClientAuth:       tls.RequireAnyClientCert,
		VerifyPeerCertificate: func(raw [][]byte, _ [][]*x509.Certificate) error {
			if len(raw) == 0 {
				return errors.New("peer did not present a certificate")
			}
			leaf, err := x509.ParseCertificate(raw[0])
			if err != nil {
				return err
			}
			if _, ok := leaf.PublicKey.(ed25519.PublicKey); !ok {
				return errors.New("peer certificate is not an ed25519 key")
			}
			return nil
		},
	}, nil
}

// clientTLSConfig configures the dial side: it presents the node's cert, sets
// a random SNI from cfg's pool (never the dialed node id), and — instead of
// hostname verification — authenticates the peer by checking that the
// presented cert carries exactly the dialed NodeID.
func clientTLSConfig(secret *base.NodeSecret, peer base.NodeID, cfg TLSConfig) (*tls.Config, error) {
	cert, err := nodeCertificate(secret, nil)
	if err != nil {
		return nil, err
	}
	return &tls.Config{
		MinVersion:         tls.VersionTLS13,
		CipherSuites:       cipherSuitesOrDefault(cfg),
		CurvePreferences:   curvesOrDefault(cfg),
		NextProtos:         alpnOrDefault(cfg),
		Certificates:       []tls.Certificate{cert},
		ServerName:         randomSNI(cfg),
		InsecureSkipVerify: true, // authentication happens in VerifyConnection below
		VerifyConnection: func(cs tls.ConnectionState) error {
			got, err := peerIDFromTLS(cs)
			if err != nil {
				return err
			}
			if got != peer {
				return errors.New("connected peer does not match the dialed node id")
			}
			return nil
		},
	}, nil
}

// sniPrefixes and sniTLDs build the default pool of realistic-looking
// hostnames used as the per-connection SNI. The names mimic CDN/edge
// hostnames so a connection is not singled out as a custom protocol.
var (
	sniPrefixes = []string{
		"api", "cdn", "cdn2", "edge", "edge2", "static", "media", "img",
		"assets", "files", "gateway", "proxy", "web", "www", "cache",
	}
	sniTLDs = []string{".com", ".net", ".org", ".io", ".co"}
)

// randomSNI returns a random hostname to use as the TLS server name: picked
// from cfg.SNIHostnames when configured, else generated from the built-in
// pool. A fresh name is returned on every call (i.e. per dial attempt).
func randomSNI(cfg TLSConfig) string {
	if len(cfg.SNIHostnames) > 0 {
		return cfg.SNIHostnames[mathrand.IntN(len(cfg.SNIHostnames))]
	}
	var b [3]byte
	if _, err := rand.Read(b[:]); err != nil {
		b = [3]byte{}
	}
	return sniPrefixes[mathrand.IntN(len(sniPrefixes))] +
		"-" + hex.EncodeToString(b[:]) +
		sniTLDs[mathrand.IntN(len(sniTLDs))]
}

// peerIDFromTLS extracts the peer's NodeID from the presented certificate.
func peerIDFromTLS(cs tls.ConnectionState) (base.NodeID, error) {
	if len(cs.PeerCertificates) == 0 {
		return base.NodeID{}, errors.New("peer did not present a certificate")
	}
	pub, ok := cs.PeerCertificates[0].PublicKey.(ed25519.PublicKey)
	if !ok {
		return base.NodeID{}, errors.New("peer certificate is not an ed25519 key")
	}
	return base.NodeIDFromBytes(pub)
}
