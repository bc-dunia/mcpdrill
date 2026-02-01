package e2e

import (
	"github.com/bc-dunia/mcpdrill/internal/auth"
	"github.com/bc-dunia/mcpdrill/internal/controlplane/api"
)

// ConfigureTestServer configures a server for testing by disabling auth.
// Call this immediately after api.NewServer() and before server.Start().
func ConfigureTestServer(server *api.Server) {
	server.SetAuthConfig(&auth.Config{
		Mode:      auth.AuthModeNone,
		SkipPaths: []string{"/healthz", "/readyz"},
	})
	server.SetAllowPrivateNetworks(true)
	server.SetWorkerAuthEnabled(false)
}
