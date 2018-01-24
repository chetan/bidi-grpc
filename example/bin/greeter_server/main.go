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
	"fmt"
	"time"

	"google.golang.org/grpc/connectivity"

	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"

	bidigrpc "github.com/chetan/bidi-grpc"
	"github.com/chetan/bidi-grpc/example/helloworld"
)

const (
	port = ":50051"
)

func main() {

	// create server
	grpcServer := grpc.NewServer()
	helloworld.RegisterGreeterServer(grpcServer, helloworld.NewServerImpl())
	reflection.Register(grpcServer)

	clientChan, shutdownChan, err := bidigrpc.Listen(port, grpcServer)
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
				bobNum := clientNum
				for {
					if gconn.GetState() == connectivity.Shutdown {
						fmt.Println("client shutdown, exiting loop")
						break
					}
					helloworld.Greet(grpcClient, "client", fmt.Sprintf("bob%d", bobNum))
					time.Sleep(helloworld.Timeout)
				}
			}()
		}
	}()

	<-shutdownChan
}
