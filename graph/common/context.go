package common

import (
	"context"
	"net/http"

	"github.com/go-logr/logr"
	hydra "github.com/ory/hydra-client-go/v2"
	kratos "github.com/ory/kratos-client-go"
	controller "github.com/traceshield/trace-shield-controller/generated/client/clientset/versioned"
	"go.opentelemetry.io/otel/trace"
)

type ClientContext struct {
	ControllerClient   *controller.Clientset
	KratosAdminClient  *kratos.APIClient
	KratosPublicClient *kratos.APIClient
	KetoClient         *KetoGrpcClient
	HydraClient        *hydra.APIClient
	Tracer             trace.Tracer
	Log                logr.Logger
}

var clientContextKey string = "CLIENT_CONTEXT"

func CreateContext(args *ClientContext) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ClientContext := &ClientContext{
				ControllerClient:   args.ControllerClient,
				KratosAdminClient:  args.KratosAdminClient,
				KratosPublicClient: args.KratosPublicClient,
				KetoClient:         args.KetoClient,
				HydraClient:        args.HydraClient,
			}
			requestWithCtx := r.WithContext(context.WithValue(r.Context(), clientContextKey, ClientContext))
			next.ServeHTTP(w, requestWithCtx)
		})
	}
}

func GetContext(ctx context.Context) *ClientContext {
	clientContext, ok := ctx.Value(clientContextKey).(*ClientContext)
	if !ok {
		return nil
	}
	return clientContext
}
