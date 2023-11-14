package main

import (
	"context"
	"fmt"
	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	trkpb "github.com/irisco88/protos/gen/tracking/v1"
	userpb "github.com/irisco88/protos/gen/user/v1"
	"github.com/tmc/grpc-websocket-proxy/wsproxy"
	"github.com/urfave/cli/v2"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/signal"
	"syscall"
)

var (
	ServiceHost      string
	ServicePort      uint
	TrackingEndPoint string
	UserEndPoint     string
	UserHttpEndPoint string
)

func main() {
	logger, err := zap.NewProduction()
	if err != nil {
		log.Fatal(err)
	}
	app := &cli.App{
		Name:  "gateway",
		Usage: "grpc gateway server",
		Commands: []*cli.Command{
			{
				Name:  "start",
				Usage: "starts server",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:        "host",
						Usage:       "host address",
						Value:       "0.0.0.0",
						DefaultText: "0.0.0.0",
						Destination: &ServiceHost,
						EnvVars:     []string{"HOST"},
					},
					&cli.UintFlag{
						Name:        "port",
						Usage:       "server port number",
						Value:       5000,
						DefaultText: "5000",
						Aliases:     []string{"p"},
						Destination: &ServicePort,
						EnvVars:     []string{"PORT"},
					},
					&cli.StringFlag{
						Name:        "tracking",
						Usage:       "tracking endpoint address",
						Destination: &TrackingEndPoint,
						EnvVars:     []string{"TRACKING_ENDPOINT"},
						Required:    true,
					},
					&cli.StringFlag{
						Name:        "user",
						Usage:       "user endpoint address",
						Destination: &UserEndPoint,
						EnvVars:     []string{"USER_ENDPOINT"},
						Required:    true,
					},
					&cli.StringFlag{
						Name:        "user-http",
						Usage:       "user http endpoint address",
						Destination: &UserHttpEndPoint,
						EnvVars:     []string{"USER_HTTP_ENDPOINT"},
						Required:    true,
					},
				},
				Action: func(ctx *cli.Context) error {
					gatewayAddr := net.JoinHostPort(ServiceHost, fmt.Sprintf("%d", ServicePort))
					gwMux := runtime.NewServeMux(runtime.WithMetadata(func(ctx context.Context, request *http.Request) metadata.MD {
						md := metadata.Pairs("token", request.Header.Get("token"))
						query := request.URL.Query()
						if token := query.Get("token"); token != "" {
							md.Set("token", token)
						}
						return md
					}))
					opts := []grpc.DialOption{
						grpc.WithTransportCredentials(insecure.NewCredentials()),
					}

					// Register TrackingService gRPC service endpoint
					if e := trkpb.RegisterTrackingServiceHandlerFromEndpoint(ctx.Context, gwMux, TrackingEndPoint, opts); e != nil {
						return fmt.Errorf("failed to register tracking: %v", e.Error())
					}

					if e := userpb.RegisterUserServiceHandlerFromEndpoint(ctx.Context, gwMux, UserEndPoint, opts); e != nil {
						return fmt.Errorf("failed to register user: %v", e.Error())
					}

					httpServer := &http.Server{
						Addr: gatewayAddr,
						Handler: wsproxy.WebsocketProxy(gwMux, wsproxy.WithForwardedHeaders(func(header string) bool {
							return true
						})),
					}
					if e := AddUserHTTPMethods(UserHttpEndPoint, gwMux); e != nil {
						return e
					}
					go func() {
						logger.Info("starting gateway HTTP server",
							zap.String("address", gatewayAddr),
						)
						if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
							logger.Error("failed to serve gateway http server", zap.Error(err))
						}
					}()

					stop := make(chan os.Signal, 1)
					signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
					<-stop

					if err := httpServer.Shutdown(ctx.Context); err != nil {
						return fmt.Errorf("HTTP server shutdown error: %v", err)
					}
					return nil
				},
			},
		},
	}

	if e := app.Run(os.Args); e != nil {
		logger.Error("failed to run app", zap.Error(e))
	}
}

func AddUserHTTPMethods(userHttpEndpoint string, mux *runtime.ServeMux) error {
	targetURL, err := url.Parse(userHttpEndpoint)
	if err != nil {
		log.Fatalf("Failed to parse target URL: %v", err)
	}
	proxy := httputil.NewSingleHostReverseProxy(targetURL)
	forwardMethod := func(w http.ResponseWriter, r *http.Request, pathParams map[string]string) {
		proxy.ServeHTTP(w, r)
	}

	if e := mux.HandlePath("GET", "/api/v1/user/avatar/download/{code}", forwardMethod); e != nil {
		return e
	}
	if e := mux.HandlePath("POST", "/api/v1/user/avatar/upload", forwardMethod); e != nil {
		return e
	}
	return nil
}
