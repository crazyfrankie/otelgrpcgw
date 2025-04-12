package example

import (
	"context"
	"errors"
	"fmt"
	"google.golang.org/grpc/credentials/insecure"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/crazyfrankie/otelgrpcgw"
	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"go.opentelemetry.io/otel/exporters/zipkin"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"google.golang.org/grpc"

	"github.com/crazyfrankie/otelgrpcgw/example/hello"
)

func TestTrace(t *testing.T) {
	mux := runtime.NewServeMux(runtime.WithMiddlewares(otelgrpcgw.NewMiddleware("/",
		otelgrpcgw.WithTracerProvider(initTracerProvider("hello")))))

	cc, err := grpc.NewClient(":8082", grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		panic(err)
	}
	err = hello.RegisterHelloServiceHandler(context.Background(), mux, cc)
	if err != nil {
		panic(err)
	}

	s := &http.Server{
		Addr:    "localhost:8083",
		Handler: mux,
	}
	if err := s.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		t.Logf("Failed to listen and serve: %v", err)
	}
}

func TestServer(t *testing.T) {
	srv := grpc.NewServer()
	hello.RegisterHelloServiceServer(srv, &HelloService{})

	conn, err := net.Listen("tcp", ":8082")
	if err != nil {
		panic(err)
	}

	srv.Serve(conn)
}

type HelloService struct {
	hello.UnimplementedHelloServiceServer
}

func (s *HelloService) Hello(ctx context.Context, req *hello.HelloRequest) (*hello.HelloResponse, error) {
	fmt.Println(req.GetMsg())
	return nil, nil
}

func initTracerProvider(servicename string) *trace.TracerProvider {
	res, err := newResource(servicename, "v0.0.1")
	if err != nil {
		fmt.Printf("failed create resource, %s", err)
	}

	tp, err := newTraceProvider(res)
	if err != nil {
		panic(err)
	}

	return tp
}

func newResource(servicename, serviceVersion string) (*resource.Resource, error) {
	return resource.Merge(resource.Default(),
		resource.NewWithAttributes(semconv.SchemaURL,
			semconv.ServiceNameKey.String(servicename),
			semconv.ServiceVersionKey.String(serviceVersion)))
}

func newTraceProvider(res *resource.Resource) (*trace.TracerProvider, error) {
	exporter, err := zipkin.New("http://localhost:9411/api/v2/spans")
	if err != nil {
		return nil, err
	}

	traceProvider := trace.NewTracerProvider(
		trace.WithBatcher(exporter, trace.WithBatchTimeout(time.Second)), trace.WithResource(res))

	return traceProvider, nil
}
