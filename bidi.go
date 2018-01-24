package bidigrpc

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/hashicorp/yamux"
	"github.com/soheilhy/cmux"
	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
)

// Connect to the server and establish a yamux channel for bidi grpc
func Connect(addr string, grpcServer *grpc.Server) *grpc.ClientConn {
	yDialer := NewYamuxDialer()
	gconn, _ := grpc.Dial(addr, grpc.WithInsecure(), grpc.WithDialer(yDialer.Dial))

	go func() {
		// start connect loop to maintain server connection over the yamux channel
		for {
			connect(addr, grpcServer, yDialer)
			time.Sleep(1 * time.Second)
		}
	}()

	return gconn
}

// connect to server
//
// dials out to the target server and then sets up a yamux channel for
// multiplexing both grpc client and server on the same underlying tcp socket
//
// this is separate from the run-loop for easy resource cleanup via defer
func connect(addr string, grpcServer *grpc.Server, yDialer *YamuxDialer) error {
	// dial underlying tcp connection
	conn, err := (&net.Dialer{}).DialContext(context.Background(), "tcp", addr)
	if err != nil {
		return err
	}
	defer conn.Close()

	session, err := yamux.Client(conn, nil)
	if err != nil {
		return err
	}
	defer session.Close()

	// now that we have a connection, create both clients & servers

	// setup client
	yDialer.SetSession(session)

	// start grpc server in a separate goroutine. this will exit when the
	// underlying session (conn) closes and clean itself up.
	go grpcServer.Serve(session)

	// return when the conn closes so we can try reconnecting
	<-session.CloseChan()
	return nil
}

// Listen starts the server side of the yamux channel for bidi grpc.
//
// Standard unidirectional grpc without yamux is still accepted as normal for
// clients which don't support or need bidi communication.
func Listen(addr string, grpcServer *grpc.Server) (chan *grpc.ClientConn, chan int, error) {
	// create underlying tcp listener
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to open listener: %s", err)
	}

	// use cmux to look for plain http2 clients
	// this allows us to support non-yamux enabled clients (such as non-golang impls)
	mux := cmux.New(lis)
	tlsL := mux.Match(cmux.TLS())
	grpcL := mux.Match(cmux.HTTP2HeaderField("content-type", "application/grpc"))
	yamuxL := mux.Match(cmux.Any())

	clientChan := make(chan *grpc.ClientConn)

	// start servers for both plain-grpc and yamux bidi grpc
	go grpcServer.Serve(grpcL)
	go grpcServer.Serve(tlsL)
	go listenLoop(yamuxL, grpcServer, clientChan)

	shutdownChan := make(chan int)
	go func() {
		mux.Serve() // serve forever
		close(shutdownChan)
	}()

	return clientChan, shutdownChan, nil
}

// listenLoop accepts new connections and sets up a yamux channel and grpc client on it.
func listenLoop(lis net.Listener, grpcServer *grpc.Server, clientChan chan *grpc.ClientConn) {
	for {
		// accept a new connection and set up a yamux session on it
		conn, err := lis.Accept()
		if err != nil {
			return // chan is prob shut down
		}

		// server session will be used to multiplex both clients & servers
		session, err := yamux.Server(conn, nil)
		if err != nil {
			return // chan is prob shut down
		}

		// start grpc server using yamux session (which implements net.Listener)
		go grpcServer.Serve(session)

		// create new client connection and dialer for this session
		dialer := NewYamuxDialer()
		dialer.SetSession(session)
		gconn, _ := grpc.Dial("localhost:50000", grpc.WithInsecure(), grpc.WithDialer(dialer.Dial))

		go func() {
			// wait for session close and close related gconn
			<-session.CloseChan()
			gconn.Close()
		}()

		go func() {
			// close session if gconn is closed for any reason
			state := gconn.GetState()
			for {
				switch state {
				case connectivity.Shutdown:
					gconn.Close()
					session.Close()
					return
				}
				gconn.WaitForStateChange(context.Background(), gconn.GetState())
				state = gconn.GetState()
			}
		}()

		clientChan <- gconn // publish gconn
	}
}
