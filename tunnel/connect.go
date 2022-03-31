package tunnel

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	"github.com/andrewstucki/light/tunnel/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

//go:generate protoc -Iproto tunnel.proto --go_out=proto/ --go-grpc_out=require_unimplemented_servers=false:proto/

type Config struct {
	Scheme  string
	Port    int
	Address string
	ID      string
	Handler http.Handler
}

// Connect is used to serve a new client handler
func Connect(ctx context.Context, config Config) error {
	scheme := "http"
	if config.Scheme != "" {
		scheme = config.Scheme
	}
	port := 80
	if config.Port != 0 {
		port = config.Port
	}

	url := url.URL{
		Scheme: scheme,
		Host:   config.Address + ":" + strconv.Itoa(port),
		Path:   "/connect",
	}
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	if err := encoder.Encode(&connectRequest{
		ID: config.ID,
	}); err != nil {
		return err
	}

	response, err := http.Post(url.String(), "application/json", &buffer)
	if err != nil {
		return err
	}

	if response.StatusCode != http.StatusOK {
		response.Body.Close()
		return fmt.Errorf("remote error: %d", response.StatusCode)
	}

	resp := &connectResponse{}
	decoder := json.NewDecoder(response.Body)
	if err := decoder.Decode(resp); err != nil {
		response.Body.Close()
		return err
	}
	response.Body.Close()

	certPool := x509.NewCertPool()
	if !certPool.AppendCertsFromPEM(resp.CA) {
		return errors.New("invalid server CA")
	}

	clientCert, err := tls.X509KeyPair(resp.Certificate, resp.PrivateKey)
	if err != nil {
		return err
	}

	tlsConfig := &tls.Config{
		ServerName:   "server",
		Certificates: []tls.Certificate{clientCert},
		RootCAs:      certPool,
	}
	tlsCredentials := credentials.NewTLS(tlsConfig)

	grpcAddress := config.Address + ":" + strconv.Itoa(resp.Port)
	connection, err := grpc.DialContext(ctx, grpcAddress, grpc.WithTransportCredentials(tlsCredentials))
	if err != nil {
		return err
	}
	defer connection.Close()

	client := proto.NewTunnelClient(connection)
	stream, err := client.ReverseServe(ctx)
	if err != nil {
		return err
	}

	for {
		request, err := stream.Recv()
		if err != nil {
			return err
		}

		req, err := apiRequestFromProto(ctx, request)
		if err != nil {
			return err
		}

		resp := newAPIResponse()
		config.Handler.ServeHTTP(resp, req)

		if err := stream.Send(resp.toProto()); err != nil {
			return err
		}
	}
}
