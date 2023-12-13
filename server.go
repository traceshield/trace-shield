package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"time"

	gqlHandler "github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/playground"
	"github.com/go-chi/chi/v5"
	"github.com/rs/cors"

	"github.com/ravilushqa/otelgqlgen"
	"github.com/traceshield/trace-shield/graph/common"
	"github.com/traceshield/trace-shield/graph/directives"
	"github.com/traceshield/trace-shield/graph/generated"
	"github.com/traceshield/trace-shield/graph/resolvers"
	"github.com/traceshield/trace-shield/handlers"
	"go.opentelemetry.io/otel"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/contrib/propagators/b3"
	jaegerProp "go.opentelemetry.io/contrib/propagators/jaeger"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

const defaultPort = "8082"

var tracer = otel.Tracer("trace-shield-server")

var (
	setupLog = ctrl.Log.WithName("setup")
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = defaultPort
	}

	opts := zap.Options{}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()
	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	initTracer(ctx)

	httpClient := &http.Client{Transport: otelhttp.NewTransport(http.DefaultTransport)}

	kratosAdminClient, err := common.NewKratosAdminClient(httpClient)
	if err != nil {
		setupLog.Error(err, "An admin address for kratos must be configured")
		panic(err)
	}

	kratosPublicClient, err := common.NewKratosPublicClient(httpClient)
	if err != nil {
		setupLog.Error(err, "An admin address for kratos must be configured")
		panic(err)
	}

	conndetails := common.NewKetoConnectionDetailsFromEnv()
	ketoClient, err := common.NewKetoGrpcClient(ctx, conndetails)
	if err != nil {
		setupLog.Error(err, "Failed to setup Keto gRPC client")
		panic(err)
	}

	hydraAdminClient, err := common.NewHydraAdminClient(httpClient)
	if err != nil {
		setupLog.Error(err, "An admin address for hydra must be configured")
		panic(err)
	}

	controllerClient, err := common.NewControllerClient(httpClient)
	if err != nil {
		setupLog.Error(err, "Failed to setup controller client")
		panic(err)
	}

	clientCtx := &common.ClientContext{
		ControllerClient:   controllerClient,
		KratosAdminClient:  kratosAdminClient,
		KratosPublicClient: kratosPublicClient,
		KetoClient:         ketoClient,
		HydraClient:        hydraAdminClient,
		Tracer:             tracer,
		Log:                ctrl.Log,
		// Log:                ctrl.Log.WithName("clients"),
	}

	if err := serve(ctx, clientCtx); err != nil {
		setupLog.Error(err, "failed to serve")
	}
}

func serve(ctx context.Context, clientCtx *common.ClientContext) (err error) {
	port := os.Getenv("PORT")
	if port == "" {
		port = defaultPort
	}

	router := chi.NewRouter()

	// Add CORS middleware around every request
	// See https://github.com/rs/cors for full option listing
	router.Use(cors.New(cors.Options{
		AllowedOrigins:   []string{"http://localhost:3000", "http://localhost:8082"}, // TODO: add config for actual hostname
		AllowCredentials: true,
		Debug:            false,
	}).Handler)

	// adds our middleware with clients to the router
	router.Use(common.CreateContext(clientCtx))

	gqlConfig := generated.Config{
		Resolvers: &resolvers.Resolver{},
		Directives: generated.DirectiveRoot{
			IsAuthenticated: directives.IsAuthenticated,
			//TODO: change all create and delete mutations so that name and namespace are used directly rather than the wrapped in the input
			CheckPermissions: directives.CheckPermissions,
		},
	}

	gqlSrv := gqlHandler.NewDefaultServer(generated.NewExecutableSchema(gqlConfig))
	gqlSrv.Use(otelgqlgen.Middleware())
	// gqlSrv.AddTransport(&transport.Websocket{
	//     Upgrader: websocket.Upgrader{
	//         CheckOrigin: func(r *http.Request) bool {
	//             // Check against your desired domains here
	// TODO: add domain to Kubricks Config CRD
	//              return r.Host == foundKubricksConfig.Spec.Domain
	//         },
	//         ReadBufferSize:  1024,
	//         WriteBufferSize: 1024,
	//     },
	// })

	authGroup := router.Group(nil)
	authGroup.Use(handlers.Middleware())

	authGroup.Handle("/graphiql", playground.Handler("GraphQL playground", "/graphql"))
	authGroup.Handle("/graphql", gqlSrv)
	authGroup.Post("/user-webhook", handlers.BootstrapAdmin)
	authGroup.Post("/oauth2/consent", handlers.Consent)
	router.Post("/check", handlers.ObservabilityTenantPolicyCheck)

	otelHandler := otelhttp.NewHandler(router, "Router")

	srv := &http.Server{
		Addr:    ":" + port,
		Handler: otelHandler,
	}

	go func() {
		if err = srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			setupLog.Error(err, "failed to serve GraphQL API")
		}
	}()

	setupLog.Info("server started")
	setupLog.Info("connect to http://localhost:" + port + "/graphiql for GraphQL playground")

	<-ctx.Done()

	setupLog.Info("server stopped")

	ctxShutDown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer func() {
		cancel()
	}()

	if err = srv.Shutdown(ctxShutDown); err != nil {
		setupLog.Error(err, "server shutdown failed")
	}

	setupLog.Info("server exited properly")

	if err == http.ErrServerClosed {
		err = nil
	}

	return

}

func initTracer(ctx context.Context) {

	opts := []sdktrace.TracerProviderOption{}

	if os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT") != "" {
		client := otlptracehttp.NewClient()
		exp, err := otlptrace.New(ctx, client)
		if err != nil {
			log.Fatalf("failed to initialize otlp export pipeline: %v", err)
		}
		opts = append(opts, sdktrace.WithSyncer(exp))
	}

	tp := sdktrace.NewTracerProvider(
		opts...,
	)

	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, b3.New(b3.WithInjectEncoding(b3.B3MultipleHeader|b3.B3SingleHeader)), jaegerProp.Jaeger{}, propagation.Baggage{}))
}
