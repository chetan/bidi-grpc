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

package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"

	bidigrpc "github.com/chetan/bidi-grpc"
	"github.com/chetan/bidi-grpc/example/helloworld"
)

const (
	address     = "localhost:50051"
	defaultName = "world"
)

var (
	help      bool
	plainGRPC bool
	useTLS    bool
	name      string
)

func flags() {
	flag.BoolVar(&help, "help", false, "Get help")
	flag.BoolVar(&help, "h", false, "")

	flag.StringVar(&name, "name", defaultName, "Name to greet with")
	flag.BoolVar(&plainGRPC, "plain", false, "Plain GRPC client (no bidi comms)")
	flag.BoolVar(&useTLS, "tls", false, "Use TLS (works with plain and bidi)")

	flag.Parse()

	if help {
		fmt.Println("greeter_client")
		fmt.Println()
		flag.PrintDefaults()
		os.Exit(1)
	}

}

func main() {
	flags()
	if plainGRPC {
		doPlainClient(address, name)
	} else {
		doBidiClient(address, name)
	}
}

func doBidiClient(addr string, name string) {
	// create reusable grpc server
	grpcServer := grpc.NewServer()
	helloworld.RegisterGreeterServer(grpcServer, helloworld.NewServerImpl())
	reflection.Register(grpcServer)

	// open channel and create client
	dialOpts := &bidigrpc.DialOpts{Addr: addr}
	gconn := bidigrpc.Connect(context.Background(), dialOpts, grpcServer)
	defer gconn.Close()
	grpcClient := helloworld.NewGreeterClient(gconn)

	// run client loop
	for {
		helloworld.Greet(grpcClient, "server", name)
		time.Sleep(helloworld.Timeout)
	}
}

// doPlainClient creates a client-only connection to the server w/o the use of
// yamux. This is to show that a non-wrapped client can still talk to the server.
func doPlainClient(addr string, name string) {
	gconn, err := grpc.Dial(address, grpc.WithInsecure())
	if err != nil {
		log.Fatalf("did not connect: %v", err)
	}
	defer gconn.Close()

	grpcClient := helloworld.NewGreeterClient(gconn)
	for {
		err := helloworld.Greet(grpcClient, "server", name)
		if err != nil {
			log.Printf("greet err: %s", err)
			break
		}
		time.Sleep(helloworld.Timeout)
	}
}
