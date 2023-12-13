package helpers

import (
	"context"
	"fmt"

	hydra "github.com/ory/hydra-client-go/v2"
	"github.com/traceshield/trace-shield/graph/common"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

// GetOAuth2Client is the resolver for the getOAuth2Client field.
func GetOAuth2ClientFromId(ctx context.Context, clientID string) (*hydra.OAuth2Client, error) {
	clients := common.GetContext(ctx)
	log := clients.Log.WithName("GetOAuth2Client")

	ctx, span := clients.Tracer.Start(ctx, "GetOAuth2Client")
	defer span.End()

	if span.IsRecording() {
		span.SetAttributes(
			attribute.String("client_id", clientID),
		)
	}

	oauth2Client, resp, err := clients.HydraClient.OAuth2Api.GetOAuth2Client(ctx, clientID).Execute()
	if err != nil || resp.StatusCode != 200 {
		log.Error(err, "failed to list oauth2 clients")
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, fmt.Errorf("failed to list oauth2 clients: %w", err)
	}
	if oauth2Client == nil {
		err = fmt.Errorf("client not found")
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}

	return oauth2Client, nil
}
