package common

import (
	"fmt"
	"net/http"
	"os"

	hydra "github.com/ory/hydra-client-go/v2"
)

const (
	HydraPublicDefault = "http://127.0.0.1:4444"
	HydraAdminDefault  = "http://127.0.0.1:4445"
	HydraEnvPublic     = "HYDRA_PUBLIC_URL"
	HydraEnvAdmin      = "HYDRA_ADMIN_URL"
)

func NewHydraAdminClient(httpClient *http.Client) (*hydra.APIClient, error) {
	hydraAdminUrl := os.Getenv(HydraEnvAdmin)
	if hydraAdminUrl == "" {
		return nil, fmt.Errorf("No admin address configured for hydra")
	}
	hydraAdminConfiguration := hydra.NewConfiguration()
	hydraAdminConfiguration.Scheme = "http"
	hydraAdminConfiguration.Servers = []hydra.ServerConfiguration{
		{
			URL: hydraAdminUrl, // Hydra Admin API
		},
	}
	hydraAdminConfiguration.HTTPClient = httpClient
	hydraAdminClient := hydra.NewAPIClient(hydraAdminConfiguration)
	return hydraAdminClient, nil
}
