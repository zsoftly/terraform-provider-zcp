package provider

import "github.com/zsoftly/zcp-cli/pkg/httpclient"

// ProviderData is passed to every resource and data source via Configure.
// Resources retrieve it from req.ProviderData and cast to *ProviderData.
type ProviderData struct {
	Client         *httpclient.Client
	DefaultProject string
}
