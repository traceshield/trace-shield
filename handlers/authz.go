package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	rts "github.com/ory/keto/proto/ory/keto/relation_tuples/v1alpha2"
	px "github.com/ory/x/pointerx"
	"github.com/traceshield/trace-shield/consts"
	"github.com/traceshield/trace-shield/graph/common"
	"github.com/traceshield/trace-shield/utils"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type PolicyRequest struct {
	Subject             string `json:"subject"`
	IsOAuth2Client      bool   `json:"isOAuth2Client"`
	RequestedPermission string `json:"requestedPermission"`
	permission          consts.ObservabilityTenantPermission
	relation            consts.ObservabilityTenantRelation
}

func ObservabilityTenantPolicyCheck(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	clients := common.GetContext(ctx)

	log := clients.Log.WithName("ObservabilityTenantPolicyCheck")

	ctx, span := clients.Tracer.Start(ctx, "ObservabilityTenantPolicyCheck")
	defer span.End()

	p := &PolicyRequest{}

	// Try to decode the request body into the struct. If there is an error,
	// respond to the client with the error message and a 400 status code.
	err := json.NewDecoder(r.Body).Decode(p)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// json, _ := json.Marshal(p)
	log.Info("Checking permissions", "Subject", p.Subject, "IsOauth2Client", p.IsOAuth2Client, "RequestedPermission", p.RequestedPermission) // TODO: remove debug log query since it leaks tokens

	permission, err := consts.ParseObservabilityTenantPermission(p.RequestedPermission)
	if err != nil {
		log.Error(err, "Failed to parse permission")
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}
	p.permission = permission

	relation, err := p.permission.GetRelation()
	if err != nil {
		log.Error(err, "Failed to get relation")
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}
	p.relation = relation

	if span.IsRecording() {
		span.SetAttributes(
			attribute.String("subject", p.Subject),
			attribute.Bool("isOAuth2Client", p.IsOAuth2Client),
			attribute.String("requestedPermission", p.RequestedPermission),
			attribute.String("permission", p.permission.String()),
			attribute.String("relation", p.relation.String()),
		)
	}

	tenantHeader := r.Header.Get(consts.TenantHeader)

	if tenantHeader != "" {
		// If the tenant header is already set, we will check the permission directly against the tenant
		// This is to allow the user to access the tenant they are already in

		log.Info("Checking if subject has access to tenant", "subject", p.Subject, "tenant", tenantHeader, "permission", p.permission)

		hasAccess, err := clients.KetoClient.Check(ctx, p.GetRelationTuple(tenantHeader))
		if err != nil {
			log.Error(err, "Failed to check if subject has access to the tenant", "subject", p.Subject, "tenant", tenantHeader, "permission", p.permission)
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			http.Error(w, err.Error(), http.StatusForbidden)
			return
		}

		if hasAccess {
			log.Info("Subject has access to tenant", "subject", p.Subject, "tenant", tenantHeader, "permission", p.permission)
			return
		} else {
			err := fmt.Errorf("Subject does not have access to tenant")
			log.Error(err, "Permission check failed", "subject", p.Subject, "tenant", tenantHeader, "permission", p.permission)
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			http.Error(w, err.Error(), http.StatusForbidden)
			return
		}
	}

	tenants := []string{}

	if !p.IsOAuth2Client {
		orgAdminTuple := &rts.RelationTuple{
			Namespace: consts.OrganizationNamespace.String(),
			Object:    consts.MainOrganizationName,
			Relation:  consts.OrganizationPermissionAdmin.String(),
			Subject: rts.NewSubjectSet(
				consts.UserNamespace.String(),
				p.Subject,
				"",
			),
		}

		isOrgAdmin, err := clients.KetoClient.Check(ctx, orgAdminTuple)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			log.Error(err, "Failed to check if user is org admin")
		}

		if isOrgAdmin {
			gottenTenants, err := clients.ControllerClient.ObservabilityV1alpha1().Tenants().List(ctx, metav1.ListOptions{})
			if err != nil {
				span.RecordError(err)
				span.SetStatus(codes.Error, err.Error())
				log.Error(err, "Failed to list observability tenants")
			}

			for _, tenant := range gottenTenants.Items {
				tenants = append(tenants, tenant.Name)
			}
			log.Info("Subject is an admin and has access to all tenants", "subject", p.Subject, "tenants", tenants, "permission", p.permission)
		} else {
			// get all the groups a user is a member of
			groups, err := getUserGroups(ctx, p.Subject)
			if err != nil {
				log.Error(err, "Failed to get user groups")
			}

			// get all the tenants a user has permissions for via group membership
			for _, group := range groups {
				groupTenants, err := getGroupPolicyTenants(ctx, p, group)
				if err != nil {
					log.Error(err, "Failed to get group tenants", "group", group)
				}
				tenants = append(tenants, groupTenants...)
			}
		}

	}
	// get all the tenants a client has permissions for
	clientTenants, err := getDirectTenants(ctx, p)
	if err != nil {
		log.Error(err, "Failed to get client tenants")
	}
	tenants = append(tenants, clientTenants...)

	if len(tenants) == 0 {
		err := fmt.Errorf("No tenants that can be accessed")
		log.Error(err, "Permission check failed", "subject", p.Subject, "permission", p.permission)
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	} else {
		if p.IsOAuth2Client && len(tenants) > 1 {
			err := fmt.Errorf("OAuth2 clients can only have access to one tenant. Please contact your administrator to resolve the issue.")
			log.Error(err, "Permission check failed", "subject", p.Subject, "permission", p.permission)
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			http.Error(w, err.Error(), http.StatusForbidden)
			return
		}
		log.Info("Subject granted access to the following tenants", "subject", p.Subject, "tenants", tenants, "permission", p.permission)
		w.Header().Set(consts.TenantHeader, strings.Join(utils.DedupeList(tenants), "|"))
		return
	}
}

func (p *PolicyRequest) GetRelationQuery() *rts.RelationQuery {
	ss := &rts.Subject{}
	if p.IsOAuth2Client {
		ss = rts.NewSubjectSet(
			consts.OAuth2ClientNamespace.String(),
			p.Subject,
			"",
		)
	} else {
		ss = rts.NewSubjectSet(
			consts.UserNamespace.String(),
			p.Subject,
			"",
		)
	}

	return &rts.RelationQuery{
		Namespace: px.Ptr(consts.ObservabilityTenantNamespace.String()),
		Relation:  px.Ptr(p.relation.String()),
		Subject:   ss,
	}
}

func (p *PolicyRequest) GetRelationTuple(tenantId string) *rts.RelationTuple {
	ss := &rts.Subject{}
	if p.IsOAuth2Client {
		ss = rts.NewSubjectSet(
			consts.OAuth2ClientNamespace.String(),
			p.Subject,
			"",
		)
	} else {
		ss = rts.NewSubjectSet(
			consts.UserNamespace.String(),
			p.Subject,
			"",
		)
	}

	return &rts.RelationTuple{
		Namespace: consts.ObservabilityTenantNamespace.String(),
		Object:    tenantId,
		Relation:  p.permission.String(),
		Subject:   ss,
	}
}

// Get the ObservabilityTenants a user has permissions for
func getDirectTenants(ctx context.Context, p *PolicyRequest) ([]string, error) {
	clients := common.GetContext(ctx)
	log := clients.Log.WithName("getDirectTenants")

	ctx, span := clients.Tracer.Start(ctx, "getDirectTenants")
	defer span.End()

	if span.IsRecording() {
		span.SetAttributes(
			attribute.String("subject", p.Subject),
			attribute.Bool("isOAuth2Client", p.IsOAuth2Client),
			attribute.String("requestedPermission", p.RequestedPermission),
			attribute.String("permission", p.permission.String()),
			attribute.String("relation", p.relation.String()),
		)
	}

	query := p.GetRelationQuery()
	respTuples, err := clients.KetoClient.QueryAllTuples(ctx, query, 100)
	if err != nil {
		log.Error(err, "Failed to query tuples")
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return []string{}, err
	}

	output := []string{}
	for _, tuple := range respTuples {
		// likely unnecessary but just in case
		if tuple.Namespace == consts.ObservabilityTenantNamespace.String() && tuple.Object != "" {
			output = append(output, tuple.Object)
		}
	}

	return output, nil
}

// Get the ObservabilityTenants a group has permissions for
func getGroupPolicyTenants(ctx context.Context, p *PolicyRequest, group string) ([]string, error) {
	clients := common.GetContext(ctx)
	log := clients.Log.WithName("getGroupPolicyTenants")

	ctx, span := clients.Tracer.Start(ctx, "getGroupPolicyTenants")
	defer span.End()

	if span.IsRecording() {
		span.SetAttributes(
			attribute.String("subject", p.Subject),
			attribute.Bool("isOAuth2Client", p.IsOAuth2Client),
			attribute.String("requestedPermission", p.RequestedPermission),
			attribute.String("permission", p.permission.String()),
			attribute.String("relation", p.relation.String()),
			attribute.String("group", group),
		)
	}

	query := rts.RelationQuery{
		Namespace: px.Ptr(consts.ObservabilityTenantNamespace.String()),
		Relation:  px.Ptr(p.relation.String()),
		Subject:   rts.NewSubjectSet(consts.GroupNamespace.String(), group, consts.GroupRelationMembers.String()),
	}
	respTuples, err := clients.KetoClient.QueryAllTuples(ctx, &query, 100)
	if err != nil {
		log.Error(err, "Failed to query tuples")
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return []string{}, err
	}

	output := []string{}
	for _, tuple := range respTuples {
		// likely unnecessary but just in case
		if tuple.Namespace == consts.ObservabilityTenantNamespace.String() && tuple.Object != "" {
			output = append(output, tuple.Object)
		}
	}

	return output, nil
}
