package directives

import (
	"context"
	"fmt"
	"time"

	"github.com/99designs/gqlgen/graphql"
	"github.com/traceshield/trace-shield/graph/common"
	"github.com/traceshield/trace-shield/handlers"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

func IsAuthenticated(ctx context.Context, obj interface{}, next graphql.Resolver) (interface{}, error) {
	clients := common.GetContext(ctx)

	log := clients.Log.WithName("IsAuthenticated")

	_, span := clients.Tracer.Start(ctx, "IsAuthenticated")
	defer span.End()

	userCtx := handlers.ForContext(ctx)
	if userCtx != nil {

		if userCtx.KratosSession != nil && userCtx.Email != "" {

			session := userCtx.KratosSession

			if !session.GetActive() || session.ExpiresAt.Before(time.Now()) {
				err := fmt.Errorf("Access denied: User session not active or has expired.")
				log.Error(err, "auth directive failed", "userId", session.Identity.Id, "email", userCtx.Email)
				if span.IsRecording() {
					span.SetAttributes(
						attribute.String("user_id", session.Identity.Id),
						attribute.String("email", userCtx.Email),
					)
				}
				span.RecordError(err)
				span.SetStatus(codes.Error, err.Error())
				return nil, err
			}
		} else {
			err := fmt.Errorf("Access denied: No known session for user.")
			log.Error(err, "auth directive failed")
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			return nil, err
		}
	} else {
		err := fmt.Errorf("Access denied: No known user.")
		log.Error(err, "auth directive failed")
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}

	// TODO: remove debug log message
	// setupLog.Info("auth directive successful")
	log.Info("Successfully authenticated user.", "userId", userCtx.Id, "email", userCtx.Email)

	if span.IsRecording() {
		span.SetAttributes(
			attribute.String("user_id", userCtx.Id),
			attribute.String("email", userCtx.Email),
		)
	}

	// let it pass through if user is authenticated
	return next(ctx)
}
