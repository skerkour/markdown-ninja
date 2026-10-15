package cloudflare

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"sync"
)

//go:embed ips.json
var IpsJson []byte

var cloudflareIps *cloudflareIpsRanges
var cloudflareIpsMutex sync.RWMutex

type cloudflareIpsRanges struct {
	Ipv4 []net.IPNet
	Ipv6 []net.IPNet
}

// CustomHostnameOwnershipVerificationHTTP represents a response from the Custom Hostnames endpoints.
type CloudflareIpsResult struct {
	Ipv4Cidrs []string `json:"ipv4_cidrs"`
	Ipv6Cidrs []string `json:"ipv6_cidrs"`
	Etag      string   `json:"etag"`
}

// TODO: return error?
func IsCloudflareIP(ipAddress string) (ret bool, err error) {
	ip := net.ParseIP(ipAddress)
	if ip == nil {
		err = fmt.Errorf("IP address is not valid: %s", ipAddress)
		return
	}

	cloudflareIpsMutex.RLock()
	defer cloudflareIpsMutex.RUnlock()

	// Is IPv4
	var networks []net.IPNet
	if ip.To4() != nil {
		networks = cloudflareIps.Ipv4
	} else {
		networks = cloudflareIps.Ipv6
	}

	for _, network := range networks {
		if network.Contains(ip) {
			ret = true
			return
		}
	}

	return
}

func loadCloudflareIps() (err error) {
	var cloudflareIpsData CloudflareIpsResult

	err = json.Unmarshal(IpsJson, &cloudflareIpsData)
	if err != nil {
		err = fmt.Errorf("cloudflare: Error parsing Cloduflare IPs ranges: %w", err)
		return
	}

	ips := cloudflareIpsRanges{
		Ipv4: make([]net.IPNet, len(cloudflareIpsData.Ipv4Cidrs)),
		Ipv6: make([]net.IPNet, len(cloudflareIpsData.Ipv6Cidrs)),
	}

	for i, cidr := range cloudflareIpsData.Ipv4Cidrs {
		var network *net.IPNet
		_, network, err = net.ParseCIDR(cidr)
		if err != nil {
			err = fmt.Errorf("cloudflare: CIDR (%s) is not valid: %w", cidr, err)
			return
		}
		ips.Ipv4[i] = *network
	}

	for i, cidr := range cloudflareIpsData.Ipv6Cidrs {
		var network *net.IPNet
		_, network, err = net.ParseCIDR(cidr)
		if err != nil {
			err = fmt.Errorf("cloudflare: CIDR (%s) is not valid: %w", cidr, err)
			return
		}
		ips.Ipv6[i] = *network
	}

	cloudflareIpsMutex.Lock()
	cloudflareIps = &ips
	cloudflareIpsMutex.Unlock()
	return
}

// Fetch the IPs used on the Cloudflare network
// Ips are fetched from https://api.cloudflare.com/client/v4/ips and can be formated using | python3 -m json.tool
// Changelog: https://www.cloudflare.com/en-gb/ips/
// API Docs: https://developers.cloudflare.com/api/operations/cloudflare-i-ps-cloudflare-ip-details
func (client *Client) GetCloudflareIps(ctx context.Context) (res CloudflareIpsResult, err error) {
	err = client.request(ctx, requestParams{
		Method: http.MethodGet,
		URL:    "/client/v4/ips",
	}, &res)
	if err != nil {
		return
	}

	return
}
