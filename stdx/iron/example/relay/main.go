// Command relay runs a minimal iron relay server.
//
//	go run ./iron/example/relay -addr :3333 -url http://203.0.113.5:3333
//
// Pass -relays to federate with other relays, so lookups are broadcast between
// them and persistent backbone links are maintained (relay-to-relay backbone):
//
//	go run ./iron/example/relay -addr :3333 -secret s3cret \
//	    -url http://203.0.113.5:3333 \
//	    -relays http://203.0.113.10:3333 -relays http://203.0.113.20:3333
package main

import (
	"flag"
	"log/slog"
	"os"
	"strings"

	"markdown.ninja/stdx/iron/relayserver"
)

// stringSlice implements flag.Value for repeatable string flags.
type stringSlice []string

func (s *stringSlice) String() string {
	return strings.Join(*s, ",")
}

func (s *stringSlice) Set(v string) error {
	*s = append(*s, v)
	return nil
}

func main() {
	addr := flag.String("addr", ":3333", "listen address")
	url := flag.String("url", "", "this relay's own http(s):// URL (used for backbone election)")
	secret := flag.String("secret", "", "shared secret for relay-to-relay backbone links and HTTP lookups (optional)")
	var peers stringSlice
	flag.Var(&peers, "relays", "other relay http(s):// URLs to federate with (repeatable)")
	flag.Parse()

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	s := relayserver.NewServer(logger)
	s.Secret = *secret
	s.Self = *url
	s.SetPeers(peers)

	logger.Info("relay listening", "addr", *addr, "url", s.Self, "peers", strings.Join(s.Peers, ","))
	if err := s.ListenAndServe(*addr); err != nil {
		logger.Error("relay failed", "err", err)
		os.Exit(1)
	}
}
