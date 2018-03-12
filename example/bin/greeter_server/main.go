/*
 *
 * Copyright 2015 gRPC authors.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 *
 */

//go:generate protoc -I ../helloworld --go_out=plugins=grpc:../helloworld ../helloworld/helloworld.proto

package main

import (
	"crypto/tls"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"time"

	"google.golang.org/grpc/connectivity"

	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"

	bidigrpc "github.com/chetan/bidi-grpc"
	"github.com/chetan/bidi-grpc/example/helloworld"
	"github.com/soheilhy/cmux"
)

const (
	port        = ":50051"
	defaultName = "bob"
)

var (
	help   bool
	useTLS bool
	name   string
)

type okHandler struct {
}

func (h *okHandler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	io.WriteString(w, "ok\n")
}

func flags() {
	flag.BoolVar(&help, "help", false, "Get help")
	flag.BoolVar(&help, "h", false, "")

	flag.StringVar(&name, "name", defaultName, "Name to greet with")
	flag.BoolVar(&useTLS, "tls", false, "Use TLS listener")

	flag.Parse()

	if help {
		fmt.Println("greeter_server")
		fmt.Println()
		flag.PrintDefaults()
		os.Exit(1)
	}
}

func main() {
	flags()

	// create server
	grpcServer := grpc.NewServer()
	helloworld.RegisterGreeterServer(grpcServer, helloworld.NewServerImpl())
	reflection.Register(grpcServer)

	var lis net.Listener
	var err error

	// create base listener
	if useTLS {
		fmt.Println("starting greeter_server on port", port, "with TLS")
		cer, err := tls.LoadX509KeyPair("server.crt", "server.key")
		if err != nil {
			panic(err)
		}
		lis, err = tls.Listen("tcp", port, &tls.Config{Certificates: []tls.Certificate{cer}})

	} else {
		fmt.Println("starting greeter_server on port", port)
		lis, err = net.Listen("tcp", port)
		if err != nil {
			panic(err)
		}
	}

	// use cmux to look for plain http2 grpc clients
	// this allows us to support non-yamux enabled clients (such as non-golang impls)
	// we can also optionally handle plain-http2 (non-grpc) and http1 clients
	mux := cmux.New(lis)
	yamuxL := mux.Match(bidigrpc.YamuxMatcher)                                    // yamux listener
	grpcL := mux.Match(cmux.HTTP2HeaderField("content-type", "application/grpc")) // gRPC listener
	http2L := mux.Match(cmux.HTTP2())                                             // HTTP 2.x listener (all non-grpc)
	http1L := mux.Match(cmux.HTTP1Fast())                                         // HTTP 1.x listener
	anyL := mux.Match(cmux.Any())

	// handle non-grpc http1/2 clients
	httpHandler := &okHandler{}
	go http.Serve(http1L, httpHandler)
	go http.Serve(http2L, httpHandler)

	// serve plain grpc clients
	go grpcServer.Serve(grpcL)

	// close others
	go closeLoop(anyL)

	// start bidi-grpc client/server
	clientChan, shutdownChan, err := bidigrpc.Listen(yamuxL, grpcServer)
	if err != nil {
		panic(err)
	}

	go func() {
		err := mux.Serve()
		if err != nil {
			fmt.Println("bye", err)
		}
	}()

	// start client loop
	clientNum := 0
	go func() {
		for {
			// accept new client conn and start working with it
			gconn := <-clientChan
			grpcClient := helloworld.NewGreeterClient(gconn)

			go func() {
				clientNum++
				num := clientNum
				for {
					if gconn.GetState() == connectivity.Shutdown {
						fmt.Println("client shutdown, exiting loop")
						break
					}
					helloworld.Greet(grpcClient, "client", fmt.Sprintf("%s%d", name, num))
					time.Sleep(helloworld.Timeout)
				}
			}()
		}
	}()

	<-shutdownChan
}

// closeLoop accepts connections from the given listener and immediately closes them.
func closeLoop(lis net.Listener) {
	for {
		conn, err := lis.Accept()
		if err != nil {
			return // chan is prob shut down
		}
		fmt.Println("closing unknown conn")
		conn.Close()
	}
}
