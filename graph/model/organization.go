package model

import (
	"context"
	"fmt"

	rts "github.com/ory/keto/proto/ory/keto/relation_tuples/v1alpha2"
	px "github.com/ory/x/pointerx"
	"github.com/traceshield/trace-shield/consts"
	"github.com/traceshield/trace-shield/graph/common"
	"github.com/traceshield/trace-shield/utils"
	"go.opentelemetry.io/otel/codes"
)

// function that will return the relation query for the admins of the organization
func (o *Organization) GetAdminsQuery() *rts.RelationQuery {
	return &rts.RelationQuery{
		Namespace: px.Ptr(consts.OrganizationNamespace.String()),
		Object:    px.Ptr(consts.MainOrganizationName),
		Relation:  px.Ptr(consts.OrganizationRelationAdmins.String()),
	}
}

// function that will return the subject set for the admins of the organization
func (o *Organization) GetAdminsSubjectSet() *rts.Subject {
	return rts.NewSubjectSet(
		consts.OrganizationNamespace.String(),
		consts.MainOrganizationName,
		consts.OrganizationRelationAdmins.String(),
	)
}

// function that will return the relation tuple for a user on the organization
func (o *Organization) GetAdminTuple(userId string) *rts.RelationTuple {
	return &rts.RelationTuple{
		Namespace: consts.OrganizationNamespace.String(),
		Object:    consts.MainOrganizationName,
		Relation:  consts.OrganizationRelationAdmins.String(),
		Subject: rts.NewSubjectSet(
			consts.UserNamespace.String(),
			userId,
			"",
		),
	}
}

// function that initializes a new organization
func NewOrganization() *Organization {
	return &Organization{}
}

func (o *Organization) UpdateAdmins(ctx context.Context, admins []*UserInput) error {
	clients := common.GetContext(ctx)
	log := clients.Log

	ctx, span := clients.Tracer.Start(ctx, "UpdateOrganizationAdmins")
	defer span.End()

	toAdd, toRemove, err := o.OrgAdminChangeset(ctx, admins)
	if err != nil {
		log.Error(err, "Failed to get organization admins changeset")
		return err
	}

	if len(toAdd) == 0 && len(toRemove) == 0 {
		log.Info("No changes to organization admins")
		return nil
	}
	return clients.KetoClient.TransactTuples(ctx, toAdd, toRemove)
}

// function that determines which admins to add or remove from an organization
func (o *Organization) OrgAdminChangeset(ctx context.Context, admins []*UserInput) (toAdd []*rts.RelationTuple, toRemove []*rts.RelationTuple, err error) {
	clients := common.GetContext(ctx)
	log := clients.Log.WithName("OrgAdminChangeset")

	ctx, span := clients.Tracer.Start(ctx, "OrgAdminChangeset")
	defer span.End()

	currentAdmins, err := o.ExpandOrgAdminRelation(ctx)
	if err != nil {
		log.Error(err, "Failed to get current admins")
		return nil, nil, fmt.Errorf("failed to get current admins: %w", err)
	}
	if admins != nil {
		adminBindings, err := getUserIdsFromUserInputs(ctx, admins)
		if err != nil {
			log.Error(err, "Failed to get user ids from user inputs")
		}

		for _, userId := range adminBindings {
			if !UserIdInListOfUsers(currentAdmins, userId) {
				toAdd = append(toAdd, o.GetAdminTuple(userId))
			}
		}

		for _, user := range currentAdmins {
			if !utils.StringContains(adminBindings, user.ID) {
				toRemove = append(toRemove, o.GetAdminTuple(user.ID))
			}
		}
	}
	return toAdd, toRemove, nil
}

// function that expands the admin relation for an organization
func (o *Organization) ExpandOrgAdminRelation(ctx context.Context) (admins []*User, err error) {
	clients := common.GetContext(ctx)
	log := clients.Log.WithName("ExpandOrgAdminRelation")

	ctx, span := clients.Tracer.Start(ctx, "ExpandOrgAdminRelation")
	defer span.End()

	respTuples, err := clients.KetoClient.Expand(ctx, o.GetAdminsSubjectSet(), 100)
	if err != nil {
		log.Error(err, "Failed to query tuples")
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}

	log.Info("Success expanding organization admin relation", "relation", consts.OrganizationRelationAdmins, "tuples", respTuples)

	if respTuples != nil && respTuples.Children != nil {
		for _, child := range respTuples.Children {

			user, _, _ := processRelation(child)
			switch {
			case user != nil:
				admins = append(admins, user)
			}
		}
	}
	return admins, nil
}
