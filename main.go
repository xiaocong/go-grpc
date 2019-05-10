package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"sync"
	"time"

	"demogrpc/hello"

	"github.com/grpc-ecosystem/grpc-gateway/runtime"

	"github.com/jbenet/goprocess"
	goprocessctx "github.com/jbenet/goprocess/context"
	"google.golang.org/grpc"
)

// implement service
type helloService struct{}

// Hello method
func (s *helloService) Hello(ctx context.Context, req *hello.HelloRequest) (*hello.HelloResponse, error) {
	fmt.Printf("Hello message: %s", req.Message)
	return &hello.HelloResponse{Code: 1, Message: "Hi! I am working!"}, nil
}

func main() {
	parent := goprocess.WithSignals(os.Interrupt)
	parent.Go(serveRPC)

	select {
	case <-parent.Closing():
	}

	select {
	case <-parent.Closed():
	}
}

// 开始 gRPC 服务
func serveRPC(proc goprocess.Process) {
	wg := sync.WaitGroup{}

	lis, _ := net.Listen("tcp", "127.0.0.1:10050")
	s := grpc.NewServer()
	hello.RegisterSayServer(s, &helloService{})

	go func() {
		wg.Add(1)
		defer wg.Done()

		fmt.Println("Starting gprc...")
		_ = s.Serve(lis)
		go proc.Close()
	}()

	// start gRPC gateway
	proc.Go(serveHTTP)

	select {
	case <-proc.Closing():
		s.GracefulStop()
		lis.Close()
	}
	wg.Wait()
}

func serveHTTP(proc goprocess.Process) {
	// register http gateway handlers
	// mux := runtime.NewServeMux()
	// see https://github.com/grpc-ecosystem/grpc-gateway/issues/233
	mux := runtime.NewServeMux(runtime.WithMarshalerOption(
		runtime.MIMEWildcard,
		&runtime.JSONPb{
			OrigName:     true,
			EmitDefaults: true,
		},
	))
	wg := sync.WaitGroup{}

	opts := []grpc.DialOption{grpc.WithInsecure()}
	if err := hello.RegisterSayHandlerFromEndpoint(goprocessctx.OnClosingContext(proc), mux, "127.0.0.1:10050", opts); err != nil {
		fmt.Printf("error to register endpoint %s", err)
	}

	var httpendpoint = "127.0.0.1:8080"
	httpserver := &http.Server{Addr: httpendpoint, Handler: mux}
	go func() {
		wg.Add(1)
		defer wg.Done()

		fmt.Println("Starting http...")
		if err := httpserver.ListenAndServe(); err != http.ErrServerClosed {
			// close proc only if the err is not ErrServerClosed
		}
		go proc.Close()
	}()

	select {
	case <-proc.Closing():
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()

		httpserver.Shutdown(ctx)
	}

	wg.Wait()
}
