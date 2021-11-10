# bidi-grpc

bidi-grpc provides a mechanism for creating a persistent, reliable, and
bi-directional gRPC connection between two processes.

The connection itself is physically established in a single direction as usual
(i.e., client -> server), however both net.Listener and net.Conn (actually
grpc.ClientConn) are returned on either side so that both sides can act as both
client and server simultaneously.

For an example, see the example greeter application
([client](./example/bin/greeter_client/main.go),
[server](./example/bin/greeter_server/main.go)).
