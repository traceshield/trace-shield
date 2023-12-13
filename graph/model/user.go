package model

import (
	"context"
	"encoding/json"
	"fmt"

	rts "github.com/ory/keto/proto/ory/keto/relation_tuples/v1alpha2"
	kratos "github.com/ory/kratos-client-go"
	px "github.com/ory/x/pointerx"
	"github.com/traceshield/trace-shield/consts"
	"github.com/traceshield/trace-shield/graph/common"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

// function that will return the relation tuple for the user on a ObservabilityTenant
func (u *User) GetTenantTuple(tenant string, relation consts.ObservabilityTenantRelation) *rts.RelationTuple {
	return &rts.RelationTuple{
		Namespace: consts.ObservabilityTenantNamespace.String(),
		Object:    tenant,
		Relation:  relation.String(),
		Subject: rts.NewSubjectSet(
			consts.UserNamespace.String(),
			u.ID,
			"",
		),
	}
}

// function that will return the relation tuple for the user on an organization
func (u *User) GetOrganizationTuple() *rts.RelationTuple {
	return &rts.RelationTuple{
		Namespace: consts.UserNamespace.String(),
		Object:    u.ID,
		Relation:  consts.ObjectRelationOrganizations.String(),
		Subject: rts.NewSubjectSet(
			consts.OrganizationNamespace.String(),
			consts.MainOrganizationName, //TODO: decide whether to hardcode this or not
			"",
		),
	}
}

// function that will return the relation query for the groups a user belongs to
func (u *User) GetGroupsQuery() *rts.RelationQuery {
	return &rts.RelationQuery{
		Namespace: px.Ptr(consts.GroupNamespace.String()),
		Relation:  px.Ptr(consts.GroupRelationMembers.String()),
		Subject:   rts.NewSubjectSet(consts.UserNamespace.String(), u.ID, ""),
	}
}

// function that unmarshals the user traits from kratos into the User struct
func (u *User) FromKratos(ctx context.Context, user *kratos.Identity) error {

	clients := common.GetContext(ctx)

	log := clients.Log.WithName("UnmarshalUserTraits")

	ctx, span := clients.Tracer.Start(ctx, "UnmarshalUserTraits")
	defer span.End()

	if span.IsRecording() {
		span.SetAttributes(
			attribute.String("user_id", user.Id),
		)
	}

	byteData, err := json.Marshal(user.Traits)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		log.Error(err, "Error when marshalling user traits")
		return err
	}

	err = json.Unmarshal(byteData, u)
	if err != nil {
		log.Error(err, "Error when unmarshalling user traits")
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	return nil
}

// function that creates a recovery link for a user and adds it to the user struct
func (u *User) CreateRecoveryLink(ctx context.Context) error {

	clients := common.GetContext(ctx)

	log := clients.Log.WithName("CreateRecoveryLinkForIdentity").WithValues("ID", u.ID)

	ctx, span := clients.Tracer.Start(ctx, "CreateRecoveryLinkForIdentity")
	defer span.End()

	if span.IsRecording() {
		span.SetAttributes(
			attribute.String("user_id", u.ID),
		)
	}

	link, resp, err := clients.KratosAdminClient.IdentityApi.CreateRecoveryLinkForIdentity(ctx).CreateRecoveryLinkForIdentityBody(
		kratos.CreateRecoveryLinkForIdentityBody{
			IdentityId: u.ID,
		},
	).Execute()
	if err != nil || resp.StatusCode != 200 {
		log.Error(err, "failed to create recovery link for identity")
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}

	u.RecoveryLink = &link.RecoveryLink

	log.Info("Success creating recovery link for identity")
	return nil
}

// function that checks if a user exists in Keto
func (u *User) ExistsInKeto(ctx context.Context) (bool, error) {
	clients := common.GetContext(ctx)

	log := clients.Log.WithName("UserExistsInKeto").WithValues("ID", u.ID)

	ctx, span := clients.Tracer.Start(ctx, "UserExistsInKeto")
	defer span.End()

	if span.IsRecording() {
		span.SetAttributes(
			attribute.String("user_id", u.ID),
		)
	}

	query := rts.RelationQuery{
		Namespace: px.Ptr(consts.UserNamespace.String()),
		Object:    px.Ptr(u.ID),
		Relation:  px.Ptr(consts.ObjectRelationOrganizations.String()),
		Subject: rts.NewSubjectSet(
			consts.OrganizationNamespace.String(),
			consts.MainOrganizationName,
			"",
		),
	}

	respTuples, err := clients.KetoClient.QueryAllTuples(ctx, &query, 100)
	if err != nil {
		log.Error(err, "Failed to query tuples")
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return false, fmt.Errorf("failed to query tuples: %w", err)
	}
	if len(respTuples) == 0 {
		return false, nil
	}
	return true, nil
}

// function that creates a user in Keto
func (u *User) CreateInKeto(ctx context.Context) error {
	clients := common.GetContext(ctx)

	log := clients.Log.WithName("CreateUserInKeto").WithValues("ID", u.ID)

	ctx, span := clients.Tracer.Start(ctx, "CreateUserInKeto")
	defer span.End()

	if span.IsRecording() {
		span.SetAttributes(
			attribute.String("user_id", u.ID),
		)
	}

	err := clients.KetoClient.CreateTuple(ctx, u.GetOrganizationTuple())
	if err != nil {
		log.Error(err, "failed to create tuple")
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return fmt.Errorf("failed to create tuple: %w", err)
	}

	log.Info("Success creating user in keto")
	return nil
}

// function that deletes a user in Keto
func (u *User) DeleteInKeto(ctx context.Context) error {

	clients := common.GetContext(ctx)

	log := clients.Log.WithName("DeleteUserInKeto").WithValues("ID", u.ID)

	ctx, span := clients.Tracer.Start(ctx, "DeleteUserInKeto")
	defer span.End()

	if span.IsRecording() {
		span.SetAttributes(
			attribute.String("user_id", u.ID),
		)
	}

	err := clients.KetoClient.DeleteTuple(ctx, u.GetOrganizationTuple())
	if err != nil {
		log.Error(err, "Failed to delete tuple")
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return fmt.Errorf("failed to delete tuple: %w", err)
	}

	log.Info("Success deleting user in keto")
	return nil
}

// function that checks if a user ID is in a []*User
func UserIdInListOfUsers(users []*User, userId string) bool {
	for _, user := range users {
		if user.ID == userId {
			return true
		}
	}
	return false
}

// function that return a *User for a given id
func NewUser(id string) *User {
	return &User{
		ID: id,
	}
}
