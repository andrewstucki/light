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
	"strings"
	"time"

	"github.com/andrewstucki/light/tunnel/proto"
	"golang.org/x/net/idna"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

//go:generate protoc -Iproto tunnel.proto --go_out=proto/ --go-grpc_out=require_unimplemented_servers=false:proto/

type Config struct {
	Server  string
	Token   string
	ID      string
	Handler http.Handler
}

// Connect is used to serve a new client handler
func Connect(ctx context.Context, config Config) error {
	id, err := idna.Lookup.ToASCII(config.ID)
	if err != nil {
		return err
	}

	if strings.Contains(id, ".") {
		return errors.New("no . characters allowed in an id")
	}

	if id == "" {
		return errors.New("must specify an id")
	}

	serverURL, err := url.Parse(config.Server)
	if err != nil {
		return err
	}

	serverURL.Path = "/connect"
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	if err := encoder.Encode(&connectRequest{
		ID: id,
	}); err != nil {
		return err
	}

	request, err := http.NewRequest("POST", serverURL.String(), &buffer)
	if err != nil {
		return err
	}
	request.Header.Add("Content-Type", "application/json")
	if config.Token != "" {
		request.Header.Add("X-Tunnel-Token", config.Token)
	}
	response, err := http.DefaultClient.Do(request)
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

	grpcAddress := serverURL.Hostname() + ":" + strconv.Itoa(resp.Port)
	connection, err := grpc.DialContext(ctx, grpcAddress, grpc.WithTransportCredentials(tlsCredentials))
	if err != nil {
		return err
	}
	defer connection.Close()

	client := proto.NewTunnelClient(connection)
	heartbeat, err := client.Heartbeat(ctx)
	if err != nil {
		return err
	}
	go func() {
		for {
			select {
			case <-time.After(heartbeatTimeout):
				err := heartbeat.Send(&proto.Empty{})
				if err != nil {
					return
				}
			case <-ctx.Done():
				return
			}
		}
	}()

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
