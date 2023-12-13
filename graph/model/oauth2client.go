package model

import (
	"context"
	"fmt"

	rts "github.com/ory/keto/proto/ory/keto/relation_tuples/v1alpha2"
	px "github.com/ory/x/pointerx"
	"github.com/traceshield/trace-shield/consts"
	"github.com/traceshield/trace-shield/graph/common"
	"github.com/traceshield/trace-shield/utils"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

// function that will return the relation tuple for the oauth2client on a ObservabilityTenant
func (oc *OAuth2Client) GetTenantTuple(tenant string, relation consts.ObservabilityTenantRelation) *rts.RelationTuple {
	return &rts.RelationTuple{
		Namespace: consts.ObservabilityTenantNamespace.String(),
		Object:    tenant,
		Relation:  relation.String(),
		Subject: rts.NewSubjectSet(
			consts.OAuth2ClientNamespace.String(),
			*oc.ClientID,
			"",
		),
	}
}

// function that gets the login binding relation tuple for a group on an oauth2client
func (oc *OAuth2Client) GetGroupTuple(groupName string) *rts.RelationTuple {
	return &rts.RelationTuple{
		Namespace: consts.OAuth2ClientNamespace.String(),
		Object:    *oc.ClientID,
		Relation:  consts.OAuth2ClientRelationLoginBindings.String(),
		Subject: rts.NewSubjectSet(
			consts.GroupNamespace.String(),
			groupName,
			consts.GroupRelationMembers.String(),
		),
	}
}

// function that gets the login binding relation tuple for a user on an oauth2client
func (oc *OAuth2Client) GetUserTuple(userId string) *rts.RelationTuple {
	return &rts.RelationTuple{
		Namespace: consts.OAuth2ClientNamespace.String(),
		Object:    *oc.ClientID,
		Relation:  consts.OAuth2ClientRelationLoginBindings.String(),
		Subject: rts.NewSubjectSet(
			consts.UserNamespace.String(),
			userId,
			"",
		),
	}
}

// function that will return the relation tuple for the oauth2client on an organization
func (oc *OAuth2Client) GetOrganizationTuple() *rts.RelationTuple {
	return &rts.RelationTuple{
		Namespace: consts.OAuth2ClientNamespace.String(),
		Object:    *oc.ClientID,
		Relation:  consts.ObjectRelationOrganizations.String(),
		Subject: rts.NewSubjectSet(
			consts.OrganizationNamespace.String(),
			consts.MainOrganizationName, //TODO: decide whether to hardcode this or not
			"",
		),
	}
}

// function that will return the subject set for the login bindings of the oauth2client
func (oc *OAuth2Client) GetLoginBindingsSubjectSet() *rts.Subject {
	return rts.NewSubjectSet(
		consts.OAuth2ClientNamespace.String(),
		*oc.ClientID,
		consts.OAuth2ClientRelationLoginBindings.String(),
	)
}

// function that updates the login bindings of the oauth2client
func (oc *OAuth2Client) UpdateLoginBindings(ctx context.Context, bindings *LoginBindingsInput) error {
	clients := common.GetContext(ctx)
	log := clients.Log.WithName("UpdateOAuth2ClientLoginBindings").WithValues("ClientID", oc.ClientID)

	ctx, span := clients.Tracer.Start(ctx, "UpdateOAuth2ClientLoginBindings")
	defer span.End()

	if span.IsRecording() {
		span.SetAttributes(
			attribute.String("client_id", *oc.ClientID),
		)
	}

	toAdd, toRemove, err := oc.LoginBindingsChangeset(ctx, bindings)
	if err != nil {
		log.Error(err, "Failed to get oauth2 client changeset")
		return err
	}

	if len(toAdd) == 0 && len(toRemove) == 0 {
		log.Info("No changes to login bindings")
		return nil
	}
	return clients.KetoClient.TransactTuples(ctx, toAdd, toRemove)
}

// function that returns the changeset for the login bindings of the oauth2client
func (oc *OAuth2Client) LoginBindingsChangeset(ctx context.Context, bindings *LoginBindingsInput) (toAdd []*rts.RelationTuple, toRemove []*rts.RelationTuple, err error) {
	clients := common.GetContext(ctx)
	log := clients.Log.WithName("LoginBindingsChangeset").WithValues("ClientID", *oc.ClientID)

	ctx, span := clients.Tracer.Start(ctx, "LoginBindingsChangeset")
	defer span.End()

	if span.IsRecording() {
		span.SetAttributes(
			attribute.String("client_id", *oc.ClientID),
		)
	}

	currentUsers, currentGroups, err := oc.ExpandLoginBindingRelation(ctx)
	if err != nil {
		log.Error(err, "Failed to expand login binding relation")
		return nil, nil, err
	}

	if bindings != nil {
		if bindings.Users != nil {
			userBindings, err := getUserIdsFromUserInputs(ctx, bindings.Users)
			if err != nil {
				log.Error(err, "Failed to get user ids from user inputs")
			}

			for _, userId := range userBindings {
				if !UserIdInListOfUsers(currentUsers, userId) {
					toAdd = append(toAdd, oc.GetUserTuple(userId))
				}
			}

			for _, user := range currentUsers {
				if !utils.StringContains(userBindings, user.ID) {
					toRemove = append(toRemove, oc.GetUserTuple(user.ID))
				}
			}
		}
		if bindings.Groups != nil {
			for _, group := range bindings.Groups {
				if !GroupNameInListOfGroups(currentGroups, group.Name) {
					toAdd = append(toAdd, oc.GetGroupTuple(group.Name))
				}
			}

			for _, group := range currentGroups {
				var groupBindings []string
				for _, group := range bindings.Groups {
					groupBindings = append(groupBindings, group.Name)
				}
				if !utils.StringContains(groupBindings, group.Name) {
					toRemove = append(toRemove, oc.GetGroupTuple(group.Name))
				}
			}
		}
	}
	return toAdd, toRemove, nil
}

// function that expands the login binding relation for an oauth2client
func (oc *OAuth2Client) ExpandLoginBindingRelation(ctx context.Context) (users []*User, groups []*Group, err error) {
	clients := common.GetContext(ctx)
	log := clients.Log.WithName("ExpandLoginBindingRelation").WithValues("ClientID", *oc.ClientID)

	ctx, span := clients.Tracer.Start(ctx, "ExpandLoginBindingRelation")
	defer span.End()

	if span.IsRecording() {
		span.SetAttributes(
			attribute.String("client_id", *oc.ClientID),
		)
	}

	respTuples, err := clients.KetoClient.Expand(ctx, oc.GetLoginBindingsSubjectSet(), 100)
	if err != nil {
		log.Error(err, "Failed to query tuples")
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, nil, err
	}

	log.Info("Success expanding oauth2 client relation", "relation", consts.OAuth2ClientRelationLoginBindings, "tuples", respTuples)

	if respTuples != nil && respTuples.Children != nil {
		for _, child := range respTuples.Children {

			user, group, _ := processRelation(child)
			switch {
			case user != nil:
				users = append(users, user)
			case group != nil:
				groups = append(groups, group)
			}
		}
	}
	return users, groups, nil
}

// function that checks if an oauth2 client exists in keto
func (oc *OAuth2Client) ExistsInKeto(ctx context.Context) (bool, error) {
	clients := common.GetContext(ctx)
	log := clients.Log.WithName("OAuth2ClientExistsInKeto").WithValues("ID", *oc.ClientID)

	ctx, span := clients.Tracer.Start(ctx, "OAuth2ClientExistsInKeto")
	defer span.End()

	if span.IsRecording() {
		span.SetAttributes(
			attribute.String("client_id", *oc.ClientID),
		)
	}

	query := rts.RelationQuery{
		Namespace: px.Ptr(consts.OAuth2ClientNamespace.String()),
		Object:    oc.ClientID,
		Relation:  px.Ptr(consts.ObjectRelationOrganizations.String()),
		Subject: rts.NewSubjectSet(
			consts.OrganizationNamespace.String(),
			consts.MainOrganizationName, //TODO: decide whether to hardcode this or not
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

// function that creates an oauth2client in keto
func (oc *OAuth2Client) CreateInKeto(ctx context.Context) error {
	clients := common.GetContext(ctx)
	log := clients.Log.WithName("CreateOAuth2ClientInKeto").WithValues("ID", *oc.ClientID)

	ctx, span := clients.Tracer.Start(ctx, "CreateOAuth2ClientInKeto")
	defer span.End()

	if span.IsRecording() {
		span.SetAttributes(
			attribute.String("client_id", *oc.ClientID),
		)
	}

	err := clients.KetoClient.CreateTuple(ctx, oc.GetOrganizationTuple())
	if err != nil {
		log.Error(err, "Failed to create tuple")
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return fmt.Errorf("failed to create tuple: %w", err)
	}

	log.Info("Success creating oauth2client in keto")
	return nil
}

// function that deletes an oauth2client in keto
func (oc *OAuth2Client) DeleteInKeto(ctx context.Context) error {
	clients := common.GetContext(ctx)
	log := clients.Log.WithName("DeleteOAuth2ClientInKeto").WithValues("ID", *oc.ClientID)

	ctx, span := clients.Tracer.Start(ctx, "DeleteOAuth2ClientInKeto")
	defer span.End()

	if span.IsRecording() {
		span.SetAttributes(
			attribute.String("client_id", *oc.ClientID),
		)
	}

	err := clients.KetoClient.DeleteTuple(ctx, oc.GetOrganizationTuple())
	if err != nil {
		log.Error(err, "Failed to delete tuple")
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return fmt.Errorf("failed to delete tuple: %w", err)
	}

	log.Info("Success deleting oauth2client in keto")
	return nil
}

// fuction that creates a new *OAuth2Client given a clientID
func NewOAuth2Client(clientID string) *OAuth2Client {
	return &OAuth2Client{
		ClientID: &clientID,
	}
}

// function that checks if a client id is in a []*model.OAuth2Client
func ClientIDInListOfOAuth2Clients(clients []*OAuth2Client, clientID string) bool {
	for _, client := range clients {
		if *client.ClientID == clientID {
			return true
		}
	}
	return false
}
