package model

import (
	"context"
	"fmt"

	rts "github.com/ory/keto/proto/ory/keto/relation_tuples/v1alpha2"
	observabilityv1alpha1 "github.com/traceshield/trace-shield-controller/api/observability/v1alpha1"
	"github.com/traceshield/trace-shield/consts"
	"github.com/traceshield/trace-shield/graph/common"
	"github.com/traceshield/trace-shield/utils"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

type ObservabilityTenantRelation struct {
	Type     consts.ObservabilityTenantRelation
	Bindings *ObservabilityTenantPermissionBindingsInput
}

// function that returns a *ObservabilityTenantRelation for a given id
func NewObservabilityTenant(id string) *ObservabilityTenant {
	return &ObservabilityTenant{
		ID: id,
	}
}

// function that returns a *ObservabilityTenantRelation for a given id
func NewObservabilityTenantFromControllerClient(input *observabilityv1alpha1.Tenant) *ObservabilityTenant {
	ot := &ObservabilityTenant{}
	ot.ID = input.Name
	if input.Spec.Limits != nil {
		ot.Limits.Mimir = input.Spec.Limits.Mimir
		ot.Limits.Tempo = input.Spec.Limits.Tempo
		ot.Limits.Loki = input.Spec.Limits.Loki
	}
	return ot
}

// function that will parses the tenant from the cluster and returns a *ObservabilityTenant
func (ot *ObservabilityTenant) ParseControllerClient(input *observabilityv1alpha1.Tenant) {
	ot.ID = input.Name
	if input.Spec.Limits != nil {
		ot.Limits.Mimir = input.Spec.Limits.Mimir
		ot.Limits.Tempo = input.Spec.Limits.Tempo
		ot.Limits.Loki = input.Spec.Limits.Loki
	}
}

func (ot *ObservabilityTenant) MutateObservabilityTenantInKeto(ctx context.Context, tenantRelations []ObservabilityTenantRelation) error {
	clients := common.GetContext(ctx)
	log := clients.Log.WithName("ObservabilityTenant").WithValues("ID", ot.ID)

	ctx, span := clients.Tracer.Start(ctx, "MutateObservabilityTenantInKeto")
	defer span.End()

	if span.IsRecording() {
		span.SetAttributes(
			attribute.String("tenant_id", ot.ID),
		)
	}

	toAdd := make([]*rts.RelationTuple, 0)
	toRemove := make([]*rts.RelationTuple, 0)

	for _, relation := range tenantRelations {
		add, remove, err := ot.OsTenantChangeset(ctx, relation.Bindings, relation.Type)
		if err != nil {
			log.Error(err, "Failed to get observability tenant changeset")
			return err
		}

		toAdd = append(toAdd, add...)
		toRemove = append(toRemove, remove...)
	}

	if len(toAdd) == 0 && len(toRemove) == 0 {
		log.Info("No changes to observability tenant permissions")
		return nil
	}
	return clients.KetoClient.TransactTuples(ctx, toAdd, toRemove)
}

// function that determines which users or groups to add or remove from the observability tenant of an oauth2 client
func (ot *ObservabilityTenant) OsTenantChangeset(ctx context.Context, bindings *ObservabilityTenantPermissionBindingsInput, relation consts.ObservabilityTenantRelation) (toAdd []*rts.RelationTuple, toRemove []*rts.RelationTuple, err error) {
	clients := common.GetContext(ctx)
	log := clients.Log.WithName("ObservabilityTenantChangeset").WithValues("ID", ot.ID)

	ctx, span := clients.Tracer.Start(ctx, "ObservabilityTenantChangeset")
	defer span.End()

	if span.IsRecording() {
		span.SetAttributes(
			attribute.String("tenant_id", ot.ID),
		)
	}

	currentUsers, currentGroups, currentClients, err := ot.ExpandTenantRelation(ctx, relation)
	if err != nil {
		log.Error(err, "Failed to expand tenant relation")
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
					user := NewUser(userId)
					toAdd = append(toAdd, user.GetTenantTuple(ot.ID, relation))
				}
			}

			for _, user := range currentUsers {
				if !utils.StringContains(userBindings, user.ID) {
					toRemove = append(toRemove, user.GetTenantTuple(ot.ID, relation))
				}
			}
		}
		if bindings.Groups != nil {
			for _, group := range bindings.Groups {
				if !GroupNameInListOfGroups(currentGroups, group.Name) {
					group := NewGroup(group.Name)
					toAdd = append(toAdd, group.GetTenantTuple(ot.ID, relation))
				}
			}

			for _, group := range currentGroups {
				var groupBindings []string
				for _, group := range bindings.Groups {
					groupBindings = append(groupBindings, group.Name)
				}
				if !utils.StringContains(groupBindings, group.Name) {
					toRemove = append(toRemove, group.GetTenantTuple(ot.ID, relation))
				}
			}
		}
		if bindings.Oauth2Clients != nil {
			for _, client := range bindings.Oauth2Clients {
				if !ClientIDInListOfOAuth2Clients(currentClients, client.ClientID) {
					client := NewOAuth2Client(client.ClientID)
					toAdd = append(toAdd, client.GetTenantTuple(ot.ID, relation))
				}
			}

			for _, client := range currentClients {
				var clientBindings []string
				for _, client := range bindings.Oauth2Clients {
					clientBindings = append(clientBindings, client.ClientID)
				}
				if !utils.StringContains(clientBindings, *client.ClientID) {
					toRemove = append(toRemove, client.GetTenantTuple(ot.ID, relation))
				}
			}
		}
	}
	return toAdd, toRemove, nil
}

// function that expands everybody with permissions on a tenant
func (ot *ObservabilityTenant) ExpandTenantRelation(ctx context.Context, relation consts.ObservabilityTenantRelation) (users []*User, groups []*Group, oauth2clients []*OAuth2Client, err error) {
	clients := common.GetContext(ctx)
	log := clients.Log.WithName("ExpandTenantRelation").WithValues("ID", ot.ID)

	ctx, span := clients.Tracer.Start(ctx, "ExpandTenantRelation")
	defer span.End()

	if span.IsRecording() {
		span.SetAttributes(
			attribute.String("tenant_id", ot.ID),
		)
	}

	ss := rts.NewSubjectSet(consts.ObservabilityTenantNamespace.String(), ot.ID, relation.String())

	respTuples, err := clients.KetoClient.Expand(ctx, ss, 100)
	if err != nil {
		log.Error(err, "Failed to query tuples")
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, nil, nil, err
	}

	log.Info("Success expanding tenant relation", "relation", relation, "tuples", respTuples)

	if respTuples != nil && respTuples.Children != nil {
		for _, child := range respTuples.Children {

			user, group, client := processRelation(child)
			switch {
			case user != nil:
				users = append(users, user)
			case group != nil:
				groups = append(groups, group)
			case client != nil:
				oauth2clients = append(oauth2clients, client)
			}
		}
	}

	return users, groups, oauth2clients, nil
}

// function that resolves an ObservabilityTenantPermissionBindings
func (ot *ObservabilityTenant) ResolveTenantBindings(ctx context.Context, relation consts.ObservabilityTenantRelation) (bindings *ObservabilityTenantPermissionBindings, err error) {
	clients := common.GetContext(ctx)
	log := clients.Log.WithName("ResolveTenantBindings").WithValues("ID", ot.ID)

	ctx, span := clients.Tracer.Start(ctx, "ResolveTenantBindings")
	defer span.End()

	if span.IsRecording() {
		span.SetAttributes(
			attribute.String("tenant_id", ot.ID),
		)
	}

	if ot.DisplayName != nil {
		span.SetAttributes(
			attribute.String("tenant_name", *ot.DisplayName),
		)
	}

	users, groups, oauth2clients, err := ot.ExpandTenantRelation(ctx, relation)
	if err != nil {
		log.Error(err, "Failed to expand tenant relation")
		return nil, fmt.Errorf("failed to expand tenant relation %s: %w", relation, err)
	}

	if len(users) > 0 || len(groups) > 0 || len(oauth2clients) > 0 {
		bindings = &ObservabilityTenantPermissionBindings{}
	}

	for _, user := range users {
		bindings.Users = append(bindings.Users, user)
	}

	for _, group := range groups {
		bindings.Groups = append(bindings.Groups, group)
	}

	for _, oath2client := range oauth2clients {
		bindings.Oauth2Clients = append(bindings.Oauth2Clients, oath2client)
	}
	log.Info("Success resolving tenant bindings")
	return bindings, nil
}
