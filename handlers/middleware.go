package handlers

import (
	"context"
	"fmt"
	"net/http"

	rts "github.com/ory/keto/proto/ory/keto/relation_tuples/v1alpha2"
	kratosClient "github.com/ory/kratos-client-go"

	"github.com/traceshield/trace-shield/consts"
	"github.com/traceshield/trace-shield/graph/common"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
)

// A private key for context that only this package can access. This is important
// to prevent collisions between different context uses
var userCtxKey = &contextKey{"user"}

type contextKey struct {
	name string
}

type AccountType string

const (
	TypeServiceAccount AccountType = "ServiceAccount"
	TypeUser           AccountType = "User"
)

// A stand-in for our backed user object
type User struct {
	// Type          AccountType
	// Username      string
	Id    string
	Name  string
	Email string
	// Groups        []string
	// ProfileImage  *string
	// JWT           *string
	KratosSession *kratosClient.Session
	IsAdmin       bool
}

// Middleware decodes the share session cookie and packs the session into context
func Middleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			clients := common.GetContext(ctx)

			log := clients.Log.WithName("Middleware")

			ctx, span := clients.Tracer.Start(ctx, "AuthenticationMiddleware")
			defer span.End()

			cookie, err := r.Cookie("ory_kratos_session")
			// Allow unauthenticated users in
			if err != nil && err != http.ErrNoCookie {
				log.Error(err, "Error when getting cookie")
				span.RecordError(err)
				span.SetStatus(codes.Error, err.Error())
				next.ServeHTTP(w, r)
				return
			}

			sessionHeader := r.Header.Get("X-Session-Token")

			// log.Info(fmt.Sprintf("Cookie: %s", cookie.String()))
			var resp *kratosClient.Session
			if cookie != nil {
				clientResp, req, err := clients.KratosPublicClient.FrontendApi.ToSession(ctx).Cookie(cookie.String()).Execute()
				if err != nil {
					// TODO: should we return here?
					log.Error(err, fmt.Sprintf("Error when calling `V0alpha2Api.ToSession``: %v\n", err))
					log.Error(err, fmt.Sprintf("Full HTTP response: %v\n", req))
					span.RecordError(err)
					span.SetStatus(codes.Error, err.Error())
					next.ServeHTTP(w, r) // TODO: find proper way to handle unauthenticated response?
					return
				}
				resp = clientResp
			} else if sessionHeader != "" {
				clientResp, req, err := clients.KratosPublicClient.FrontendApi.ToSession(ctx).XSessionToken(sessionHeader).Execute()
				if err != nil {
					// TODO: should we return here?
					log.Error(err, fmt.Sprintf("Error when calling `V0alpha2Api.ToSession``: %v\n", err))
					log.Error(err, fmt.Sprintf("Full HTTP response: %v\n", req))
					span.RecordError(err)
					span.SetStatus(codes.Error, err.Error())
					next.ServeHTTP(w, r) // TODO: find proper way to handle unauthenticated response?
					return
				}
				resp = clientResp
			} else {
				// TODO: re-enable below once we have a proper health and/or metrics endpoint
				// err := fmt.Errorf("No session cookie or header found")
				// log.Error(err, "Error while authenticating user", "path", r.URL.Path, "method", r.Method, "request", r)
				// span.RecordError(err)
				// span.SetStatus(codes.Error, err.Error())
				next.ServeHTTP(w, r)
				return
			}
			// response from `ToSession`: Session
			log.Info(fmt.Sprintf("%s", resp.Identity.Traits))

			var email string
			var name string

			if val, ok := resp.Identity.Traits.(map[string]interface{})["email"]; ok {
				if foundEmail, ok := val.(string); ok {
					email = foundEmail
				} else {
					span.RecordError(err)
					span.SetStatus(codes.Error, err.Error())
					log.Error(err, "Error when parsing email")
				}
			}

			if val, ok := resp.Identity.Traits.(map[string]interface{})["name"]; ok {
				first, firstFound := val.(map[string]interface{})["first"]
				last, lastFound := val.(map[string]interface{})["last"]

				if firstName, ok := first.(string); ok {
					if lastName, ok := last.(string); ok {
						if firstFound && lastFound {
							name = firstName + " " + lastName
						}
					}
				}
			}

			user := &User{
				KratosSession: resp,
				Email:         email,
				Name:          name,
				Id:            resp.Identity.Id,
			}

			// user.Type = TypeUser
			// user.Username = strings.Replace(strings.Split(user.Email, "@")[0], ".", "-", -1)

			user.IsAdmin = false

			adminQuery := rts.RelationTuple{
				Namespace: consts.OrganizationNamespace.String(),
				Object:    consts.MainOrganizationName,
				Relation:  consts.OrganizationPermissionAdmin.String(),
				Subject: rts.NewSubjectSet(
					consts.UserNamespace.String(),
					resp.Identity.Id,
					"",
				),
			}

			isAdmin, err := clients.KetoClient.Check(ctx, &adminQuery)
			if err != nil {
				span.RecordError(err)
				span.SetStatus(codes.Error, err.Error())
				log.Error(err, "Error when checking if user is admin")
			}

			log.Info("Admin check", "user", user.Email, "admin", isAdmin)
			user.IsAdmin = isAdmin

			if span.IsRecording() {
				span.SetAttributes(
					attribute.String("user_id", user.Id),
					attribute.String("email", user.Email),
				)
			}

			// for _, adminEmail := range kubricksConfig.Spec.Admins {
			// 	if adminEmail == email {
			// 		user.IsAdmin = true
			// 	}
			// }

			ctx = context.WithValue(ctx, userCtxKey, user)

			// and call the next with our new context
			log.Info("Success auth", "user", user.Email)
			r = r.WithContext(ctx)
			propagators := otel.GetTextMapPropagator()
			propagators.Inject(ctx, propagation.HeaderCarrier(r.Header))
			next.ServeHTTP(w, r)
		})
	}
}

// ForContext finds the user from the context. REQUIRES Middleware to have run.
func ForContext(ctx context.Context) *User {
	raw, _ := ctx.Value(userCtxKey).(*User)
	return raw
}
