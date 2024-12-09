package main

import (
	"context"
	"crypto/tls"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"

	"github.com/amaury95/graphify"
	"github.com/arangodb/go-driver"
	config "github.com/arangodb/go-driver/http"
	"github.com/gorilla/mux"
	"go.uber.org/fx"
	libraryv1 "graphify.template/domain/library/v1"
	relationv1 "graphify.template/domain/relation/v1"
)

func main() {
	dbUrl := flag.String("db", os.Getenv("DB_URL"), "Database URL")
	dbUser := flag.String("user", os.Getenv("DB_USER"), "Database user")
	dbPass := flag.String("pass", os.Getenv("DB_PASS"), "Database password")
	flag.Parse()

	fx.New(
		// Context
		fx.Provide(func() context.Context {
			return graphify.DevelopmentContext(context.Background())
		}),

		// Storage
		fx.Supply(graphify.FilesystemStorageConfig{
			BasePath:  "./uploads",
			MaxMemory: 10 << 20, // 10 MB limit
		}),
		fx.Provide(
			fx.Annotate(
				graphify.NewFilesystemStorage,
				fx.As(new(graphify.IFileStorage)),
			),
		),

		// Observer
		fx.Provide(
			fx.Annotate(
				graphify.NewObserver[graphify.Topic],
				fx.As(new(graphify.IObserver[graphify.Topic])),
			),
		),

		// Connection
		fx.Supply(
			graphify.ConnectionConfig{
				DBName:     "library",
				UserName:   *dbUser,
				Password:   *dbPass,
				Connection: config.ConnectionConfig{Endpoints: []string{*dbUrl}},
			}),
		fx.Provide(
			fx.Annotate(
				graphify.NewConnection,
				fx.As(new(graphify.IConnection)),
			),
		),

		// Access
		fx.Provide(
			fx.Annotate(
				graphify.NewArangoAccess,
				fx.As(new(graphify.IAccess)),
			),
		),

		// Graph
		fx.Provide(
			func(ctx context.Context, access graphify.IAccess) graphify.IGraph {
				graph := graphify.NewGraph()

				graph.Node(libraryv1.Book{})
				graph.Node(libraryv1.Client{})
				graph.Node(libraryv1.Library{})
				graph.Edge(libraryv1.Client{}, libraryv1.Book{}, relationv1.Borrow{})

				access.Collection(ctx, libraryv1.Library{}, func(ctx context.Context, c driver.Collection) {
					c.EnsureGeoIndex(ctx, []string{"location"}, &driver.EnsureGeoIndexOptions{})
				})
				access.AutoMigrate(ctx, graph)
				return graph
			},
		),

		// Admin
		fx.Supply(graphify.AdminHandlerConfig{
			Secret: []byte("secret"),
		}),
		fx.Provide(graphify.NewAdminHandler),

		// Graphql
		fx.Provide(graphify.NewGraphqlHandler),

		/* setup router */
		fx.Provide(func(ctx context.Context, admin *graphify.AdminHandler, graphql *graphify.GraphqlHandler) *mux.Router {
			router := mux.NewRouter()

			router.PathPrefix("/admin").
				Handler(admin.Handler(ctx))

			router.PathPrefix("/graphql").Handler(
				graphql.Handler(ctx,
					graphify.ExposeNodes(),
				))

			return router
		}),

		/* run http server */
		fx.Invoke(func(ctx context.Context, lc fx.Lifecycle, router *mux.Router) *http.Server {
			srv := &http.Server{
				Addr:        ":6431",
				Handler:     router,
				BaseContext: func(net.Listener) context.Context { return ctx }, // Inject app context to requests
			}

			lc.Append(fx.Hook{
				OnStart: func(ctx context.Context) error {
					ln, err := net.Listen("tcp", srv.Addr)
					if err != nil {
						return err
					}

					// Load TLS certificate and key
					cert, err := tls.LoadX509KeyPair("server.crt", "server.key")
					if err != nil {
						return fmt.Errorf("error loading TLS cert: %v", err)
					}

					srv.TLSConfig = &tls.Config{
						Certificates: []tls.Certificate{cert},
						MinVersion:   tls.VersionTLS12,
					}

					fmt.Println("Starting HTTPS server at", srv.Addr)
					go srv.ServeTLS(ln, "", "") // Empty strings since we configured TLS above
					return nil
				},
				OnStop: func(ctx context.Context) error {
					return srv.Shutdown(ctx)
				},
			})

			return srv
		}),
	).Run()
}
