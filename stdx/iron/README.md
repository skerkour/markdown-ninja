# iron — peer-to-peer networking dialed by public key

`iron` is a Peer-to-Peer (p2p) networking layer for Go. It lets nodes on different
networks find each other and communicate **without a public IP address and
without configuring port forwarding**, using a **relay** for connectivity and
**hole punching** to upgrade to direct, low-latency connections when possible.

Every node is identified by a **NodeID**: the public half of an Ed25519 key
pair. You **dial by public key** — you never need to know a peer's IP address
in advance. Connections are authenticated peer-to-peer: each side proves it
owns the public key it claims, over QUIC (via quic-go), using self-signed
X.509 certificates whose key *is* the node's signing key.

Built on top of [QUIC](https://quic-go.dev), `iron` gives you reliable,
authenticated, multiplexed streams and unreliable datagrams between two
endpoints.

---

## How it works

- **Identity.** A node is an Ed25519 key pair (`base.NodeSecret`). Its public
  key is the `NodeID` you share and dial.
- **Relay.** Every node keeps a persistent authenticated WebSocket connection
  to one relay server (its lowest-latency one). The relay forwards opaque QUIC
  packets between nodes that cannot reach each other directly, so two peers
  behind NATs can still talk. When two peers live on different relays, the
  relays forward between themselves over a **backbone** link, so a node still
  needs only that single relay connection. Each relay pair keeps exactly one
  bidirectional backbone connection: the relay with the smaller address dials,
  the larger invites itself with a periodic Hello, and links are kept alive
  with a ping/pong keepalive (3s/9s) and re-established if a peer drops.
- **Directory.** In the relay handshake each node publishes the UDP addresses
  it is reachable at (LAN, same-host, and its relay-observed public address)
  to its connected relay. The directory entry lives only as long as the relay
  connection, so it is purged the moment the node disconnects.
- **Lookup.** When dialing, the endpoint asks **its connected relay** for the
  peer with a signed HTTP `POST /relay/api`. The relay answers from its own
  clients and, when needed, **broadcasts the same lookup over HTTP to its full
  dynamic peer list** (its `-relays` peers plus any learned via Hello,
  authenticated with the shared secret), then aggregates the answers. The
  client never has to contact the other relays itself — useful on restricted
  networks that can only reach their own relay. The endpoint then races a
  **direct** connection (using the peer's announced addresses, and **hole
  punching** through NATs when necessary) against a **relay** connection, and
  uses whichever establishes first.
- **Path maintenance.** A connection **upgrades** from relay to direct once a
  direct path becomes reachable, and a dialed connection **transparently
  falls back** to the relay if the direct path dies — so the application keeps
  its streams without noticing.

### Data flow

```
   dialer A                                  relay1                  relay2                          listener B
      |                                          |                      |                               |
      | 1. connect (WebSocket)                   |                      |                               |
      |----------------------------------------->|                      |  1. connect (WebSocket)        |
      |                                          |<---------------------|-------------------------------|
      | 2. announce addrs in handshake           |                      |                               |
      |----------------------------------------->|                      |                               |
      | 3. HTTP lookup B (POST /relay/api)       |                      |                               |
      |----------------------------------------->|                      |                               |
      |                                          | 4. HTTP lookup B     |                               |
      |                                          |  (Bearer secret)     |                               |
      |                                          |--------------------->|                               |
      |                                          |<---------------------|-------------------------------|
      | 5. aggregated answer (addrs + which      |                      |                               |
      |    relay found B)                        |                      |                               |
      |<-----------------------------------------|                      |                               |
      |                                          |                      |                               |
      | 6. QUIC packets over relay1 WS           | 7. forward to relay2  | 8. deliver to B (RelayToClient)|
      |----------------------------------------->| (RelayToRelayBatch) ->|------------------------------>|
      |                                          |                      |                               |
      | 6'. reply packets (B's QUIC)             |<-- relay2 forwards --|-------------------------------|
      |<-----------------------------------------|                      |                               |
```

**Step by step:**

1. Each endpoint connects (WebSocket, authenticated) to its chosen relay — the
   first reachable in its `WithRelayURLs` list.
2. In the handshake each endpoint announces the UDP addresses it is reachable
   at (LAN, same-host, relay-observed public). Disconnecting purges the entry
   immediately.
3. Dialer A `POST`s a signed `APILookup` for B's `NodeID` to **its own relay**
   (relay1) — the only relay A ever contacts.
4. Relay1 answers from its own clients and, when B isn't there, broadcasts the
   same lookup over HTTP to its federated peers (relay2), authenticating with
   the shared secret (`Authorization: Bearer <secret>`). Peers answer locally
   only, so the broadcast never recurses.
5. Relay1 aggregates: B's direct addresses, its observed public IP, and the
   relays that reported it (`FoundRelays`). A uses that to decide where to
   route.
6. A's QUIC packets go over A's relay1 WebSocket; because B is on relay2, A's
   packets are tagged with B's relay and relay1 forwards them over the
   **backbone** (`RelayToRelayBatch`) to relay2, which delivers them to B
   (step 7–8). While doing so, relay2 learns that A is reachable *via relay1*,
   so B's reply packets (which carry no hint) are routed back over the
   backbone too.
7. If A and B are directly reachable (LAN, same host, or after a successful
   hole punch), the connection upgrades to a direct one and the relays are no
   longer on the data path.

The relay control plane (handshake, keepalives, restart advisories, hole-punch
coordination) uses [CBOR](https://github.com/fxamacker/cbor)-tagged messages;
the HTTP directory lookup (`POST /relay/api`) uses the same signed CBOR
messages.

---

## Running a relay

A relay is a standalone server you (or others) run on a public IP. Peers
connect to it over `http://` (plain) or `https://` (TLS). Relay URLs are
always `http(s)://`; the WebSocket tunnel is derived from them internally. See
`iron/example/relay`:

```sh
# a standalone relay
go run ./iron/example/relay -addr :3333 -url http://203.0.113.5:3333

# a federated relay: broadcast lookups to (and receive them from) other relays.
# -url must be this relay's own reachable URL; -secret must match the peers'.
# The relay with the smaller address dials the other; the larger one invites
# itself (a Hello) so the smaller dials — each pair keeps exactly one persistent
# backbone connection, even if only one side lists the other.
go run ./iron/example/relay -addr :3333 -secret <shared-secret> \
    -url http://203.0.113.5:3333 \
    -relays http://203.0.113.10:3333 -relays http://203.0.113.20:3333
```

You can pass an `http://` URL to this address from any endpoint, e.g.
`http://203.0.113.5:3333` or `http://127.0.0.1:3333` for local testing. Two
relays configured with the same `-secret`, each other's URL in `-relays` and
their own `-url` federate: they broadcast lookups to each other over HTTP
(Bearer-authenticated), keep a single persistent backbone connection, and
forward data between their clients over it. The relay logs each peer relay
connect/disconnect like it does for clients.

**Dynamic federation.** The peer list is dynamic: relays connect to each other
eagerly at startup, and peers can be added or removed at runtime without
restarting (`s.SetPeers(...)` on `relayserver.Server`, or by passing `-relays`
on the command line at start). Because the smaller-address relay dials while
the larger one invites itself with a periodic Hello, a single backbone
connection is formed even when only one side lists the other — so you can bring
up relays one at a time. Backbone links are kept alive with a ping/pong
keepalive (3s/9s); a silently-dead peer is detected and the link re-established.

Under the hood this is `relayserver.Server.ListenAndServe`:

```go
logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
s := relayserver.NewServer()
s.Log = logger
s.Secret = "shared-secret"        // shared with the relays you federate with
s.Self = "http://203.0.113.5:3333" // this relay's own URL (backbone election)
s.Peers = []string{"http://203.0.113.10:3333"} // relays to federate with
if err := s.ListenAndServe(":3333"); err != nil {
    log.Fatal(err)
}
```

The server serves:
- `/relay` — the WebSocket datagram tunnel (and the backbone link when the
  peer presents the shared secret), and
- `/relay/api` — the HTTP directory: signed `APILookup` requests from clients,
  and Bearer-authenticated broadcasts between federated relays.

---

## Using the library to connect endpoints

The `iron` package exposes the endpoint API. The simplest end-to-end example
(copy of `iron/example/echo`): two endpoints meet through a relay, and the
dialer opens a QUIC stream to the listener's NodeID, sends a message, and gets
it echoed back.

### 1. Give each node an identity

```go
import "markdown.ninja/stdx/iron/base"

secret, err := base.NewNodeSecret() // a fresh Ed25519 identity
// keep it stable across restarts by persisting it:
// secret, err = base.NewNodeSecretFromBytes(privKeyBytes)
```

### 2. Create an endpoint

```go
import "markdown.ninja/stdx/iron"

const relayURL = "http://127.0.0.1:3333"

// context cancels the outgoing dials, not the endpoint's lifetime.
ctx := context.Background()

ep, err := iron.NewEndpoint(ctx, secret, "", iron.WithRelayURLs(relayURL))
if err != nil {
    log.Fatal(err)
}
defer ep.Close()

log.Printf("node id: %s", ep.NodeID()) // share this with your peer
```

`NewEndpoint` binds a UDP socket for direct connections, connects to the relay
(if any are configured), announces this node's direct addresses, and starts
accepting inbound connections. Relevant `EndpointOption`s:

- `iron.WithRelayURLs(urls ...string)` — one or more relay `ws(s)://` URLs to
  use (dial/announce/lookup). Without any, the endpoint is relay-free: it only
  connects directly via announced addresses or discovery channels.
- `iron.WithRelayOnly()` — never open a direct socket; only ever connect
  through the relay.
- `iron.WithTLSConfig(...)` — customize the TLS used for all peer connections
  (see below). By default connections are hardened to blend in with web
  HTTP/3 traffic.
- `iron.WithSkipAnnounce()`, `iron.WithAnnouncers(...)`,
  `iron.WithRelayBatching(...)`, `iron.WithRelayWaitTimeout(...)`,
  `iron.WithLogger(...)` — see the package docs.

### 3. Listener: accept connections and echo

```go
// accept the next inbound connection; it is authenticated (the peer's
// identity comes from the TLS certificate)
conn, err := ep.Accept(ctx)
if err != nil {
    log.Fatal(err)
}
remote, err := ep.PeerID(conn)       // the dialer's NodeID
log.Printf("connection from %s via %s", remote, conn.Path())

// echo every stream the peer opens
for {
    st, err := conn.AcceptStream(ctx)
    if err != nil {
        return
    }
    go func() { defer st.Close(); io.Copy(st, st) }()
}
```

### 4. Dialer: connect by NodeID and open a stream

```go
peer, err := base.NodeIDFromString("...the listener's node id...")
if err != nil {
    log.Fatal(err)
}

conn, err := ep.Connect(ctx, peer) // dial by public key
if err != nil {
    log.Fatal(err)
}
defer conn.CloseWithError(0, "")

st, err := conn.OpenStreamSync(ctx)
if err != nil {
    log.Fatal(err)
}
if _, err := st.Write([]byte("hello")); err != nil {
    log.Fatal(err)
}
st.Close() // finish the send half; the read half stays open

reply, err := io.ReadAll(st)
if err != nil {
    log.Fatal(err)
}
fmt.Printf("echo: %s\n", reply)
```

### The `Connection` API

A `*iron.Connection` wraps quic-go's connection. It transparently keeps
**your** dialed connection alive across network changes (falling back to the
relay if a direct path dies, and upgrading relay → direct when possible), so
you operate on streams and datagrams without managing paths:

- **Streams** — `OpenStream`, `OpenStreamSync`, `OpenStreamSync(ctx)`,
  `OpenUniStream`, `AcceptStream`, `AcceptUniStream` (bidirectional and
  unidirectional).
- **Datagrams** — `SendDatagram`, `ReceiveDatagram` (unreliable).
- **Introspection** — `Path()` (`"direct"` or `"relay"`), `PeerID()`,
  `State()`, `Context()`.
- `conn.Path()` tells you whether the connection is currently direct or
  relayed, e.g. for logging or metrics.

### Peer discovery without the relay

The relay is not the only way to find peers. The `iron/discovery` package
defines two roles you can plug into an endpoint:

```go
import "markdown.ninja/stdx/iron/discovery"

// lookup a peer's direct addresses (Discoverer), and/or announce ours
// (Announcer). Implement the interfaces for mDNS, a DHT, your own directory,
// etc.
ep, err := iron.NewEndpoint(ctx, secret, "", iron.WithRelayURLs(relayURL))
    iron.WithAnnouncers(myAnnouncer),
)

conn, err := ep.Connect(ctx, peer, iron.WithDiscoverers(myDiscoverer))
```

---

## Traffic hardening (HTTP/3 masquerade)

Peer-to-peer connections are QUIC with TLS 1.3; everything after the handshake
is encrypted. To keep iron traffic from standing out at the TLS layer, the
defaults make each connection look like ordinary web HTTP/3:

- **ALPN `h3`** — the TLS ClientHello advertises `h3`, the HTTP/3 protocol
  identifier, exactly like a browser. The node id or an iron-specific
  application protocol never appears on the wire.
- **Browser-like ClientHello** — the three TLS 1.3 cipher suites are offered in
  browser order (`AES-128-GCM`, `AES-256-GCM`, `CHACHA20`) and the KeyExchange
  groups default to `[X25519, X25519MLKEM768]`, matching modern browsers.
- **Random SNI** — instead of a synthetic `<nodeid>.iron.invalid` name (which
  leaked the node id and the iron TLD), each connection uses a random
  realistic-looking hostname from a built-in pool, so the dialed identity never
  appears in the cleartext Server Name Indication.
- **Certificate matches the SNI** — the server mints its self-signed
  certificate on the fly with the requested SNI as its DNS subject alternative
  name, so the handshake is internally consistent. Authentication is unchanged:
  the peer's certificate still carries exactly its Ed25519 public key
  (`VerifyConnection`), so a middlebox cannot impersonate anyone.

These are tunable via `iron.WithTLSConfig(...)`:

```go
iron.WithTLSConfig(iron.TLSConfig{
    ALPN:             []string{"h3"},                 // advertise something else
    CipherSuites:     []uint16{tls.TLS_AES_128_GCM_SHA256},
    CurvePreferences: []tls.CurveID{tls.X25519},
    SNIHostnames:     []string{"cdn.example.net"},    // SNI pool
})
```

Both endpoints must advertise a compatible `ALPN` list or the handshake fails
(see `TestMismatchedALPNFails`).

Known limitations: the QUIC transport parameters and the exact ClientHello
extension set still fingerprint `quic-go`/Go's TLS stack (there is no GREASE or
Encrypted ClientHello in Go), and the relay path itself is plaintext
HTTP/1.1 + WebSocket. The hardened defaults only mask the peer-to-peer QUIC
layer.

---

## Try it end-to-end

```sh
# terminal 1 — a relay
go run ./iron/example/relay

# terminal 2 — the listener (note the printed node id)
go run ./iron/example/echo -relay http://127.0.0.1:3333 -mode listen

# terminal 3 — the dialer
go run ./iron/example/echo -relay http://127.0.0.1:3333 \
    -mode connect -peer <the-listener-node-id>
```

You should see `echo round-trip OK` on the dialer, and the listener will report
that the connection started relayed and upgraded to direct (both endpoints are
on the same host).

### Backbone test across two relays

Run two federated relays and put each endpoint on a different one; the dialer
finds the listener through the relay-to-relay lookup broadcast and the data
travels over the backbone:

```sh
# terminal 1 — relay 1
go run ./iron/example/relay -addr :3333 -secret s3cret -url http://127.0.0.1:3333 \
    -relays http://127.0.0.1:4444

# terminal 2 — relay 2 (federated with relay 1)
go run ./iron/example/relay -addr :4444 -secret s3cret -url http://127.0.0.1:4444 \
    -relays http://127.0.0.1:3333

# terminal 3 — the listener, connected to relay 2 only
go run ./iron/example/echo -relay http://127.0.0.1:4444 -mode listen

# terminal 4 — the dialer, connected to relay 1 only (note the node id)
go run ./iron/example/echo -relay http://127.0.0.1:3333 \
    -mode connect -peer <the-listener-node-id>
```

Add `-relay-only` to both echo endpoints to force all traffic through the
relays, so you can observe the pure relayed path end to end.

Running all on one machine works because same-host peers get a fast loopback
connection; across the internet they connect through the relay (and directly
when hole punching succeeds).


## TODO

- make relay a binary, not a library?
- ensure that client batching is really usefull
- quic / or http3 connection from endpoints to relay (will need to handle when clients migrate IP), fallback to websocket.

