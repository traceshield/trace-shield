package helpers

import (
	"context"
	"fmt"

	kratos "github.com/ory/kratos-client-go"
	"github.com/traceshield/trace-shield/graph/common"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

// function that gets a user from the Kratos API
func GetUserFromId(ctx context.Context, id string) (*kratos.Identity, error) {
	clients := common.GetContext(ctx)
	log := clients.Log.WithName("User").WithValues("ID", id)
	ctx, span := clients.Tracer.Start(ctx, "GetUserFromId")
	defer span.End()

	if span.IsRecording() {
		span.SetAttributes(
			attribute.String("user_id", id),
		)
	}

	user, resp, err := clients.KratosAdminClient.IdentityApi.GetIdentity(ctx, id).Execute()
	if err != nil || resp.StatusCode != 200 {
		log.Error(err, "failed to get user")
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	log.Info("Success getting User")
	return user, nil
}

// function that gets a user from an email
func GetUserFromEmail(ctx context.Context, email string) (*kratos.Identity, error) {
	clients := common.GetContext(ctx)
	log := clients.Log.WithName("User").WithValues("email", email)

	ctx, span := clients.Tracer.Start(ctx, "GetUserFromEmail")
	defer span.End()

	if span.IsRecording() {
		span.SetAttributes(
			attribute.String("user_email", email),
		)
	}

	users, resp, err := clients.KratosAdminClient.IdentityApi.ListIdentities(ctx).CredentialsIdentifier(email).Execute()
	if err != nil || resp.StatusCode != 200 {
		log.Error(err, "failed to get user")
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}

	if len(users) == 0 {
		return nil, nil
	} else if len(users) > 1 {
		err := fmt.Errorf("multiple users found with email %s", email)
		log.Error(err, "failed to get user")
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	} else if len(users) == 1 {
		log.Info("Success getting User")
		return &users[0], nil
	}
	return nil, nil
}
