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

// function that will return the relation tuple for the group on a ObservabilityTenant
func (g *Group) GetTenantTuple(tenant string, relation consts.ObservabilityTenantRelation) *rts.RelationTuple {
	return &rts.RelationTuple{
		Namespace: consts.ObservabilityTenantNamespace.String(),
		Object:    tenant,
		Relation:  relation.String(),
		Subject: rts.NewSubjectSet(
			consts.GroupNamespace.String(),
			g.Name,
			consts.GroupRelationMembers.String(),
		),
	}
}

// function that will return the relation tuple for a user on a group
func (g *Group) GetUserTuple(userId string) *rts.RelationTuple {
	return &rts.RelationTuple{
		Namespace: consts.GroupNamespace.String(),
		Object:    g.Name,
		Relation:  consts.GroupRelationMembers.String(),
		Subject: rts.NewSubjectSet(
			consts.UserNamespace.String(),
			userId,
			"",
		),
	}
}

// function that will return the relation tuple for a group on an organization
func (g *Group) GetOrganizationTuple() *rts.RelationTuple {
	return &rts.RelationTuple{
		Namespace: consts.GroupNamespace.String(),
		Object:    g.Name,
		Relation:  consts.ObjectRelationOrganizations.String(),
		Subject: rts.NewSubjectSet(
			consts.OrganizationNamespace.String(),
			consts.MainOrganizationName,
			"",
		),
	}
}

// function that will return the relation query for the group on an organization
func (g *Group) GetOrganizationQuery() *rts.RelationQuery {
	return &rts.RelationQuery{
		Namespace: px.Ptr(consts.GroupNamespace.String()),
		Object:    px.Ptr(g.Name),
		Relation:  px.Ptr(consts.ObjectRelationOrganizations.String()),
		Subject: rts.NewSubjectSet(
			consts.OrganizationNamespace.String(),
			consts.MainOrganizationName,
			"",
		),
	}
}

// function that will return the subject set for the members of the group
func (g *Group) GetMembersSubjectSet() *rts.Subject {
	return rts.NewSubjectSet(
		consts.GroupNamespace.String(),
		g.Name,
		consts.GroupRelationMembers.String(),
	)
}

// function that checks if a group exists in keto
func (g *Group) ExistsInKeto(ctx context.Context) (bool, error) {
	clients := common.GetContext(ctx)
	log := clients.Log.WithName("GroupExistsInKeto").WithValues("Name", g.Name)

	ctx, span := clients.Tracer.Start(ctx, "GroupExistsInKeto")
	defer span.End()

	if span.IsRecording() {
		span.SetAttributes(
			attribute.String("group_id", g.Name),
		)
	}

	respTuples, err := clients.KetoClient.QueryAllTuples(ctx, g.GetOrganizationQuery(), 100)
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

// function that creates a group in keto
func (g *Group) CreateInKeto(ctx context.Context) error {
	clients := common.GetContext(ctx)
	log := clients.Log.WithName("CreateGroupInKeto").WithValues("Name", g.Name)

	ctx, span := clients.Tracer.Start(ctx, "CreateGroupInKeto")
	defer span.End()

	if span.IsRecording() {
		span.SetAttributes(
			attribute.String("group_id", g.Name),
		)
	}

	err := clients.KetoClient.CreateTuple(ctx, g.GetOrganizationTuple())
	if err != nil {
		log.Error(err, "Failed to create tuple")
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return fmt.Errorf("failed to create tuple: %w", err)
	}

	log.Info("Success creating group in keto")
	return nil
}

// function that deletes a group in keto
func (g *Group) DeleteInKeto(ctx context.Context) error {
	clients := common.GetContext(ctx)
	log := clients.Log.WithName("DeleteGroupInKeto").WithValues("Name", g.Name)

	ctx, span := clients.Tracer.Start(ctx, "DeleteGroupInKeto")
	defer span.End()

	if span.IsRecording() {
		span.SetAttributes(
			attribute.String("group_id", g.Name),
		)
	}

	err := clients.KetoClient.DeleteTuple(ctx, g.GetOrganizationTuple())
	if err != nil {
		log.Error(err, "Failed to delete tuple")
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}

	log.Info("Success deleting group in keto")
	return nil
}

// function that updates the members of a group in keto
func (g *Group) UpdateMembers(ctx context.Context, members []*UserInput) error {
	clients := common.GetContext(ctx)
	log := clients.Log

	ctx, span := clients.Tracer.Start(ctx, "UpdateGroupMembers")
	defer span.End()

	if span.IsRecording() {
		span.SetAttributes(
			attribute.String("group_id", g.Name),
		)
	}

	toAdd, toRemove, err := g.GroupChangeset(ctx, members)
	if err != nil {
		log.Error(err, "Failed to get groupd members changeset")
		return err
	}

	if len(toAdd) == 0 && len(toRemove) == 0 {
		log.Info("No changes to group members")
		return nil
	}
	return clients.KetoClient.TransactTuples(ctx, toAdd, toRemove)
}

// function that determines which users to add or remove as a member from a group
func (g *Group) GroupChangeset(ctx context.Context, members []*UserInput) (toAdd []*rts.RelationTuple, toRemove []*rts.RelationTuple, err error) {
	clients := common.GetContext(ctx)
	log := clients.Log.WithName("GroupChangeset")

	ctx, span := clients.Tracer.Start(ctx, "GroupChangeset")
	defer span.End()

	if span.IsRecording() {
		span.SetAttributes(
			attribute.String("group_id", g.Name),
		)
	}

	currentMembers, err := g.ExpandGroupMembersRelation(ctx)
	if err != nil {
		log.Error(err, "Failed to get current members")
		return nil, nil, fmt.Errorf("failed to get current members: %w", err)
	}
	if members != nil {
		memberBindings, err := getUserIdsFromUserInputs(ctx, members)
		if err != nil {
			log.Error(err, "Failed to get user ids from user inputs")
		}

		for _, userId := range memberBindings {
			if !UserIdInListOfUsers(currentMembers, userId) {
				toAdd = append(toAdd, g.GetUserTuple(userId))
			}
		}

		for _, user := range currentMembers {
			if !utils.StringContains(memberBindings, user.ID) {
				toRemove = append(toRemove, g.GetUserTuple(user.ID))
			}
		}
	}
	return toAdd, toRemove, nil
}

// function that expands the members relation for a group
func (g *Group) ExpandGroupMembersRelation(ctx context.Context) (members []*User, err error) {
	clients := common.GetContext(ctx)
	log := clients.Log.WithName("ExpandGroupMembersRelation")

	ctx, span := clients.Tracer.Start(ctx, "ExpandGroupMembersRelation")
	defer span.End()

	if span.IsRecording() {
		span.SetAttributes(
			attribute.String("group_id", g.Name),
		)
	}

	respTuples, err := clients.KetoClient.Expand(ctx, g.GetMembersSubjectSet(), 100)
	if err != nil {
		log.Error(err, "Failed to query tuples")
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}

	log.Info("Success expanding group members relation", "relation", consts.GroupRelationMembers, "tuples", respTuples)

	if respTuples != nil && respTuples.Children != nil {
		for _, child := range respTuples.Children {

			user, _, _ := processRelation(child)
			switch {
			case user != nil:
				members = append(members, user)
			}
		}
	}
	return members, nil
}

// function that checks if a group is in a []*Group
func GroupNameInListOfGroups(groups []*Group, groupId string) bool {
	for _, group := range groups {
		if group.Name == groupId {
			return true
		}
	}
	return false
}

// function that return a *model.Group for a given group name
func NewGroup(groupName string) *Group {
	return &Group{
		Name: groupName,
	}
}
