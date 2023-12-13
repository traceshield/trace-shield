package handlers

import (
	"errors"
	"net/http"

	"github.com/go-chi/render"
	"github.com/traceshield/trace-shield/graph/common"
	"github.com/traceshield/trace-shield/graph/model"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

type BootstrapRequest struct {
	UserID string `json:"userId,omitempty"`
}

func (b *BootstrapRequest) Bind(r *http.Request) error {
	// a.Article is nil if no Article fields are sent in the request. Return an
	// error to avoid a nil pointer dereference.
	if b.UserID == "" {
		return errors.New("missing required Article fields.")
	}

	// a.User is nil if no Userpayload fields are sent in the request. In this app
	// this won't cause a panic, but checks in this Bind method may be required if
	// a.User or further nested fields like a.User.Name are accessed elsewhere.

	// just a post-process after a decode..
	// a.ProtectedID = ""                                 // unset the protected ID
	// a.Article.Title = strings.ToLower(a.Article.Title) // as an example, we down-case
	return nil
}

func (b *BootstrapRequest) Render(w http.ResponseWriter, r *http.Request) error {
	// Pre-processing before a response is marshalled and sent across the wire
	// a.Elapsed = 10
	return nil
}

func BootstrapAdmin(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	clients := common.GetContext(ctx)

	log := clients.Log.WithName("BootstrapAdmin")
	log.Info("Bootstrapping first user as admin")

	ctx, span := clients.Tracer.Start(ctx, "getGroupPolicyTenants")
	defer span.End()

	data := &BootstrapRequest{}
	if err := render.Bind(r, data); err != nil {
		log.Error(err, "failed to bind request")
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		render.Render(w, r, ErrInvalidRequest(err))
		return
	}

	if data.UserID == "" {
		err := errors.New("missing required userId field")
		log.Error(err, "failed to bind request")
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return
	}

	if span.IsRecording() {
		span.SetAttributes(
			attribute.String("user_id", data.UserID),
		)
	}

	org := model.NewOrganization()

	currentAdmins, err := org.ExpandOrgAdminRelation(ctx)
	if err != nil {
		log.Error(err, "failed to get current admins")
		render.Render(w, r, ErrFailedToGetAdmins(err))
		return
	}

	if len(currentAdmins) == 0 {
		log.Info("No current admins, adding initial user as admin", "user", data.UserID)
		err = clients.KetoClient.CreateTuple(ctx, org.GetAdminTuple(data.UserID))
		if err != nil {
			log.Error(err, "failed to add bootstrap user")
			render.Render(w, r, ErrFailedToSetInitialAdmin(err))
			return
		}
	}

	render.Status(r, http.StatusOK) // TODO: we should probably do error checking above so we can return a 400 if something goes wrong
	render.Render(w, r, bootstrapUser(data))
}

func bootstrapUser(b *BootstrapRequest) *BootstrapRequest {
	return b
}
