package directives

import (
	"context"
	"fmt"

	"github.com/99designs/gqlgen/graphql"
	"github.com/traceshield/trace-shield/graph/common"
	"github.com/traceshield/trace-shield/handlers"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

func CheckPermissions(ctx context.Context, obj interface{}, next graphql.Resolver) (interface{}, error) {
	clients := common.GetContext(ctx)
	log := clients.Log.WithName("CheckPermissions")

	_, span := clients.Tracer.Start(ctx, "CheckPermissions")
	defer span.End()

	// setupLog.Info("Scope directive", "object", graphql.GetFieldContext(ctx).Parent)

	// var namespace string

	// namespace = ""

	// // TODO: check that this still works when no variables are used
	// if graphql.GetFieldContext(ctx).Field.Arguments.ForName("namespace") != nil {
	// 	namespaceArg := graphql.GetFieldContext(ctx).Field.Arguments.ForName("namespace")
	// 	namespaceValue, err := namespaceArg.Value.Value(graphql.GetOperationContext(ctx).Variables)
	// 	if err != nil {
	// 		return nil, err
	// 	}
	// 	namespace = namespaceValue.(string)
	// }

	// var operation string

	// if graphql.GetFieldContext(ctx).Object == "Query" || graphql.GetFieldContext(ctx).Object == "Mutation" {

	// 	operation = graphql.GetFieldContext(ctx).Field.Name
	// } else if graphql.GetFieldContext(ctx).Parent != nil {
	// 	// this is for nested objects
	// 	operation = graphql.GetFieldContext(ctx).Object
	// } else {
	// 	return nil, fmt.Errorf("Access denied: The SubjectAccessReview has not yet been implemented for this scenario")
	// }

	// // get the verb and type by splitting the camelCase
	// // for example, `getStorageClass` becomes `get` and `StorageClass`
	// splitOperation := splitCamelCase(operation)

	// if len(splitOperation) != 2 {
	// 	return nil, fmt.Errorf("Access denied: Something went wrong when parsing the operation for Kubernetes RBAC")
	// }

	// var verb, ObjectType string

	// ObjectType = splitOperation[1]

	// // check if first string is empty
	// if splitOperation[0] == "" {
	// 	verb = "get"
	// } else {
	// 	verb = splitOperation[0]
	// }

	// // check if the ObjectType is a plural
	// // if it is, we need to convert it to a singular
	// // for example, `StorageClasses` becomes `StorageClass`
	// pluralize := pluralize.NewClient()
	// if pluralize.IsPlural(ObjectType) {
	// 	ObjectType = pluralize.Singular(ObjectType)
	// }

	// // get the TypeSar from the ObjectType
	// TypeSar, err := sarLookupFunc(ObjectType)
	// if err != nil {
	// 	return nil, fmt.Errorf("Access denied: Failed to check user permissions. %s", err)
	// }

	// if err := auth.UserAuthz(
	// 	ctx,
	// 	kubeClient,
	// 	setupLog,
	// 	TypeSar.Group,
	// 	verb,
	// 	TypeSar.Resource,
	// 	TypeSar.Version,
	// 	namespace); err != nil {

	// 	// TODO: remove debug log message
	// 	setupLog.Info("permission directive failed")
	// 	return nil, fmt.Errorf("Access denied: User is not allowed to '%s' '%s' in namespace '%s'. Error: %s", verb, TypeSar.Resource, namespace, err)
	// }

	userCtx := handlers.ForContext(ctx)
	if userCtx != nil {
		if !userCtx.IsAdmin {
			err := fmt.Errorf("Access denied: User does not have required permissions.")
			log.Error(err, "permission directive failed", "userId", userCtx.Id, "email", userCtx.Email)
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			if span.IsRecording() {
				span.SetAttributes(
					attribute.String("user_id", userCtx.Id),
					attribute.String("email", userCtx.Email),
				)
			}
			return nil, err
		}
	} else {
		err := fmt.Errorf("Access denied: No known user.")
		log.Error(err, "permission directive failed")
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}

	log.Info("User has required permissions.", "userId", userCtx.Id, "email", userCtx.Email)

	if span.IsRecording() {
		span.SetAttributes(
			attribute.String("user_id", userCtx.Id),
			attribute.String("email", userCtx.Email),
		)
	}

	// or let it pass through
	return next(ctx)
}
