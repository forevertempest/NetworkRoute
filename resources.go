// Package resources contains offline manuals and the optional release server bundle.
package resources

import (
	"embed"
)

//go:embed docs/*.md tools/setup-wireguard.md tools/install-relay.sh tools/install-wireguard.sh .bundle/*
var Files embed.FS
