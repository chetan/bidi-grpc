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
	"net"
	"os"
	"time"

	"google.golang.org/grpc/connectivity"

	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"

	bidigrpc "github.com/chetan/bidi-grpc"
	"github.com/chetan/bidi-grpc/example/helloworld"
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

	clientChan, shutdownChan, err := bidigrpc.Listen(lis, grpcServer, nil)
	if err != nil {
		panic(err)
	}

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
