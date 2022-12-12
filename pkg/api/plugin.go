package api

import (
	"github.com/hashicorp/go-plugin"
)

// HandshakeConfig is common handshake config between Botkube and its plugins.
var HandshakeConfig = plugin.HandshakeConfig{
	// The magic cookie values should NEVER be changed.
	MagicCookieKey:   "BOTKUBE",
	MagicCookieValue: "52ca7b74-28eb-4fac-ae79-31a9cbda2454",
}

// MetadataOutput contains the metadata of a given plugin.
type MetadataOutput struct {
	// Version is a version of a given plugin. It should follow the SemVer syntax.
	Version string
	// Descriptions is a description of a given plugin.
	Description string
}
