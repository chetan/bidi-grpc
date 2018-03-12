package bidigrpc

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"time"

	"github.com/cenkalti/backoff"
	"github.com/hashicorp/yamux"
	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
)

// DialOpts configures the Dialer for the underlying TCP connection
type DialOpts struct {
	Addr      string
	TLS       bool // whether or not to use TLS for the underlying channel
	TLSConfig *tls.Config
}

// Connect to the server and establish a yamux channel for bidi grpc.
func Connect(ctx context.Context, dialOpts *DialOpts, grpcServer *grpc.Server) *grpc.ClientConn {
	yDialer := NewYamuxDialer()

	go func() {
		// start connect loop to maintain server connection over the yamux channel
		connectOp := func() error {
			return connect(dialOpts, grpcServer, yDialer)
		}
		for {
			// retry will keep trying to connect to the server forever (or until context is done/canceled)
			// after a successful connection, however, retry will return nil when the connection is closed
			// so wrap in a loop here to reconnect and try again so we can maintain the connection
			retry(ctx, connectOp, 5*time.Second, 0)
			// check if we should give up due to canceled ctx
			select {
			case <-ctx.Done():
			default:
				fmt.Println("ctx not done yet, trying again")
			}
		}
	}()

	gconn, _ := grpc.Dial(dialOpts.Addr, grpc.WithInsecure(), grpc.WithDialer(yDialer.Dial))
	return gconn
}

// retry the given operation
func retry(ctx context.Context, op backoff.Operation, maxInterval time.Duration, maxElapsedTime time.Duration) error {
	eb := backoff.NewExponentialBackOff()
	eb.MaxInterval = maxInterval
	eb.MaxElapsedTime = maxElapsedTime
	if ctx == nil {
		return backoff.Retry(op, eb)
	}
	return backoff.Retry(op, backoff.WithContext(eb, ctx))
}

// connect to server
//
// dials out to the target server and then sets up a yamux channel for
// multiplexing both grpc client and server on the same underlying tcp socket
//
// this is separate from the run-loop for easy resource cleanup via defer
func connect(dialOpts *DialOpts, grpcServer *grpc.Server, yDialer *YamuxDialer) error {
	// dial underlying tcp connection
	var conn net.Conn
	var err error

	if dialOpts.TLS {
		// use tls
		cfg := dialOpts.TLSConfig
		if cfg == nil {
			cfg = &tls.Config{}
		}
		conn, err = tls.Dial("tcp", dialOpts.Addr, cfg)

	} else {
		conn, err = (&net.Dialer{}).DialContext(context.Background(), "tcp", dialOpts.Addr)
	}
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
func Listen(lis net.Listener, grpcServer *grpc.Server) (clientChan chan *grpc.ClientConn, shutdownChan chan int, err error) {
	// create channels
	clientChan = make(chan *grpc.ClientConn)
	shutdownChan = make(chan int)

	go func() {
		// start listenLoop - blocks until some error occurs
		listenLoop(lis, grpcServer, clientChan)
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
			conn.Close() // close conn and retry
			continue
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
