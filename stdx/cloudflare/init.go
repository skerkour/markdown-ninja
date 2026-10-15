package cloudflare

import "fmt"

// TODO: keep as init?
func init() {
	err := loadCloudflareIps()
	if err != nil {
		panic(fmt.Errorf("cloudflare: error loading IPs: %w", err))
	}
}
