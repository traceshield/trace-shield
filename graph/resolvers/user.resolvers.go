package resolvers

// This file will be automatically regenerated based on the schema, any resolver implementations
// will be copied through when generating and any unknown code will be moved to the end.
// Code generated by github.com/99designs/gqlgen version v0.17.34

import (
	"context"
	"fmt"

	kratos "github.com/ory/kratos-client-go"
	"github.com/pluralsh/trace-shield/graph/common"
	"github.com/pluralsh/trace-shield/graph/generated"
	"github.com/pluralsh/trace-shield/graph/model"
	"github.com/pluralsh/trace-shield/graph/resolvers/helpers"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

// CreateUser is the resolver for the createUser field.
func (r *mutationResolver) CreateUser(ctx context.Context, email string, name *model.NameInput) (*model.User, error) {
	clients := common.GetContext(ctx)

	log := clients.Log.WithName("CreateUser").WithValues("Email", email)

	ctx, span := clients.Tracer.Start(ctx, "CreateUser")
	defer span.End()

	if span.IsRecording() {
		span.SetAttributes(
			attribute.String("email", email),
		)
	}

	traits := map[string]interface{}{
		"email": email,
	}

	if name != nil {
		traits["name"] = make(map[string]interface{})
		if name.First != nil {
			traits["name"].(map[string]interface{})["first"] = *name.First
		}
		if name.Last != nil {
			traits["name"].(map[string]interface{})["last"] = *name.Last
		}
	}

	kratosUser, resp, err := clients.KratosAdminClient.IdentityApi.CreateIdentity(ctx).CreateIdentityBody(
		kratos.CreateIdentityBody{
			SchemaId: "person",
			Traits:   traits,
		},
	).Execute()

	if err != nil || resp.StatusCode != 201 {
		log.Error(err, "failed to create user")
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}

	user := model.NewUser(kratosUser.Id)

	if err := user.CreateRecoveryLink(ctx); err != nil {
		log.Error(err, "failed to create recovery link for user")
	}

	exits, err := user.ExistsInKeto(ctx)
	if err != nil {
		log.Error(err, "failed to check if user exists in keto")
		// return nil, err
	}

	if !exits {
		err = user.CreateInKeto(ctx)
		if err != nil {
			log.Error(err, "failed to create user in keto")
			return nil, err
		}
	}

	if err := user.FromKratos(ctx, kratosUser); err != nil {
		log.Error(err, "failed to unmarshal user traits")
		return nil, err
	}

	log.Info("Success creating User")
	return user, nil
}

// DeleteUser is the resolver for the deleteUser field.
func (r *mutationResolver) DeleteUser(ctx context.Context, id string) (*model.User, error) {
	clients := common.GetContext(ctx)

	log := clients.Log.WithName("DeleteUser").WithValues("ID", id)

	ctx, span := clients.Tracer.Start(ctx, "DeleteUser")
	defer span.End()

	if span.IsRecording() {
		span.SetAttributes(
			attribute.String("user_id", id),
		)
	}

	user := model.NewUser(id)
	user.Email = "deleted"

	resp, err := clients.KratosAdminClient.IdentityApi.DeleteIdentity(ctx, user.ID).Execute()

	if err != nil || resp.StatusCode != 204 {
		log.Error(err, "failed to delete user")
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}

	exits, err := user.ExistsInKeto(ctx)
	if err != nil {
		log.Error(err, "failed to check if user exists in keto")
		// return nil, err
	}

	if exits {
		err = user.DeleteInKeto(ctx)
		if err != nil {
			log.Error(err, "failed to delete user in keto")
			return nil, err
		}
	}

	log.Info("Success deleting User")
	return user, nil
}

// ListUsers is the resolver for the listUsers field.
func (r *queryResolver) ListUsers(ctx context.Context) ([]*model.User, error) {
	clients := common.GetContext(ctx)

	log := clients.Log.WithName("ListUsers")

	ctx, span := clients.Tracer.Start(ctx, "ListUsers")
	defer span.End()

	users, resp, err := clients.KratosAdminClient.IdentityApi.ListIdentities(ctx).Execute()
	if err != nil || resp.StatusCode != 200 {
		log.Error(err, "failed to list users")
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, fmt.Errorf("failed to list users: %w", err)
	}

	var output []*model.User

	for _, user := range users {
		outUser := model.NewUser(user.Id)
		if err := outUser.FromKratos(ctx, &user); err != nil {
			log.Error(err, "Error when unmarshalling user", "ID", outUser.ID)
			continue
		}
		output = append(output, outUser)
	}
	return output, nil
}

// GetUser is the resolver for the getUser field.
func (r *queryResolver) GetUser(ctx context.Context, id *string, email *string) (*model.User, error) {
	clients := common.GetContext(ctx)

	log := clients.Log.WithName("User")

	var user *kratos.Identity

	if id != nil {
		foundUser, err := helpers.GetUserFromId(ctx, *email)
		if err != nil {
			user = foundUser
		} else {
			return nil, fmt.Errorf("user not found")
		}
	} else if email != nil {
		foundUser, err := helpers.GetUserFromEmail(ctx, *email)
		if err != nil {
			user = foundUser
		} else {
			return nil, fmt.Errorf("user not found")
		}
	} else {
		return nil, fmt.Errorf("id or email must be provided")
	}
	if user != nil {
		outUser := model.NewUser(user.Id)
		if err := outUser.FromKratos(ctx, user); err != nil {
			return nil, err
		}
		log.Info("Success getting User")

		return outUser, nil
	} else {
		return nil, nil
	}
}

// Groups is the resolver for the groups field.
func (r *userResolver) Groups(ctx context.Context, obj *model.User) ([]*model.Group, error) {
	clients := common.GetContext(ctx)
	log := clients.Log.WithName("GetUserGroups").WithValues("ID", obj.ID)

	ctx, span := clients.Tracer.Start(ctx, "GetUserGroups")
	defer span.End()

	if span.IsRecording() {
		span.SetAttributes(
			attribute.String("user_id", obj.ID),
		)
	}

	respTuples, err := clients.KetoClient.QueryAllTuples(ctx, obj.GetGroupsQuery(), 100)
	if err != nil {
		log.Error(err, "Failed to get tuples")
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, fmt.Errorf("failed to get tuples: %w", err)
	}

	var groups []*model.Group

	// TODO: add once group resolver is generated
	for _, tuple := range respTuples {
		_ = model.NewGroup(tuple.Object)
		// group, err := c.GetGroupFromName(ctx, tuple.Object)
		// if err != nil {
		// 	log.Error(err, "failed to get group", "Name", tuple.Object)
		// 	continue
		// }
		// groups = append(groups, group)
	}

	return groups, nil
}

// Mutation returns generated.MutationResolver implementation.
func (r *Resolver) Mutation() generated.MutationResolver { return &mutationResolver{r} }

// Query returns generated.QueryResolver implementation.
func (r *Resolver) Query() generated.QueryResolver { return &queryResolver{r} }

// User returns generated.UserResolver implementation.
func (r *Resolver) User() generated.UserResolver { return &userResolver{r} }

type mutationResolver struct{ *Resolver }
type queryResolver struct{ *Resolver }
type userResolver struct{ *Resolver }
