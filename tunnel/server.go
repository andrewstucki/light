package tunnel

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/andrewstucki/light/tunnel/proto"
	"github.com/gorilla/mux"
	"golang.org/x/crypto/acme/autocert"
	"golang.org/x/net/idna"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type ServerConfig struct {
	Host             string
	Address          string
	HTTPPort         int
	GRPCPort         int
	ACMEEmailAddress string
	Token            string
}

type tunnelServer struct {
	host     string
	token    string
	port     int
	registry *tunnelRegistry
	router   *mux.Router
}

func newTunnelServer(port int, host, token string, registry *tunnelRegistry) *tunnelServer {
	server := &tunnelServer{
		port:     port,
		token:    token,
		host:     host,
		registry: registry,
	}
	router := mux.NewRouter()
	hostRouter := router.Host(host).Subrouter()
	hostRouter.Methods("POST").Path("/connect").HandlerFunc(server.Connect)
	hostRouter.PathPrefix("/").HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		response.WriteHeader(http.StatusNotFound)
	})
	router.PathPrefix("/").HandlerFunc(server.Handler)
	server.router = router
	return server
}

func (t *tunnelServer) Handler(response http.ResponseWriter, request *http.Request) {
	id := strings.TrimSuffix(request.Host, "."+t.host)
	session, ok := t.registry.sessionByID(id)
	if !ok {
		response.WriteHeader(http.StatusNotFound)
		return
	}
	req, err := httpRequestToProto(request)
	if err != nil {
		response.WriteHeader(http.StatusInternalServerError)
		return
	}
	resp, err := session.send(request.Context(), req)
	if err != nil {
		if err == io.EOF {
			response.WriteHeader(http.StatusNotFound)
		} else {
			response.WriteHeader(http.StatusInternalServerError)
		}
		return
	}
	convert(resp, response)
}

type connectRequest struct {
	ID string `json:"id"`
}

type connectResponse struct {
	Port        int    `json:"port"`
	CA          []byte `json:"ca"`
	PrivateKey  []byte `json:"privateKey"`
	Certificate []byte `json:"certificate"`
}

func (t *tunnelServer) Connect(response http.ResponseWriter, request *http.Request) {
	defer request.Body.Close()

	if t.token != "" {
		// check static token
		token := request.Header.Get("X-Tunnel-Token")
		if token != t.token {
			response.WriteHeader(http.StatusUnauthorized)
			return
		}
	}

	req := &connectRequest{}
	decoder := json.NewDecoder(request.Body)
	if err := decoder.Decode(req); err != nil {
		response.WriteHeader(http.StatusBadRequest)
		return
	}
	nonce, created, err := t.registry.createSession(req.ID)
	if err != nil {
		response.WriteHeader(http.StatusInternalServerError)
		return
	}
	if !created {
		response.WriteHeader(http.StatusBadRequest)
		return
	}
	certificate, privateKey, err := rootCA.generate(req.ID, nonce)
	if err != nil {
		response.WriteHeader(http.StatusInternalServerError)
		return
	}

	encoder := json.NewEncoder(response)
	if err := encoder.Encode(&connectResponse{
		Port:        t.port,
		CA:          rootCA.PEM,
		PrivateKey:  privateKey,
		Certificate: certificate,
	}); err != nil {
		response.WriteHeader(http.StatusInternalServerError)
		return
	}
}

func (t *tunnelServer) ReverseServe(stream proto.Tunnel_ReverseServeServer) error {
	ctx := stream.Context()
	session, found := t.registry.get(id(ctx))
	if !found {
		return status.Errorf(codes.NotFound, "client not found")
	}
	defer t.registry.clear(id(ctx))

	if err := session.handle(func(request *proto.APIRequest) (*proto.APIResponse, error) {
		if err := stream.Send(request); err != nil {
			return nil, err
		}
		return stream.Recv()
	}); err != nil {
		if err != io.EOF {
			return status.Errorf(codes.Internal, err.Error())
		}
	}
	return nil
}

func RunServer(ctx context.Context, config ServerConfig) error {
	listener, err := net.Listen("tcp", config.Address+":"+strconv.Itoa(config.GRPCPort))
	if err != nil {
		return err
	}
	defer listener.Close()

	registry := newTunnelRegistry()
	server := newTunnelServer(config.GRPCPort, config.Host, config.Token, registry)
	grpcServer := grpc.NewServer(
		grpc.Creds(serverCredentials),
		grpc.StreamInterceptor(spiffeStreamMiddleware),
	)
	proto.RegisterTunnelServer(grpcServer, server)

	group, ctx := errgroup.WithContext(ctx)
	group.Go(func() error {
		return grpcServer.Serve(listener)
	})
	group.Go(func() error {
		manager := &autocert.Manager{
			Cache:  newCertCache(),
			Prompt: autocert.AcceptTOS,
			Email:  config.ACMEEmailAddress,
			HostPolicy: func(ctx context.Context, host string) error {
				if host == config.Host {
					return nil
				}
				h, err := idna.Lookup.ToASCII(host)
				if err != nil {
					return err
				}
				parsed, err := url.Parse(h)
				if err != nil {
					return err
				}
				isSubdomain := strings.HasSuffix(parsed.Host, "."+config.Host)
				if !isSubdomain {
					return fmt.Errorf("host %q is not an allowed host", host)
				}
				// check that we have only a single level of subdomain
				trimmed := strings.TrimSuffix(parsed.Host, "."+config.Host)
				if strings.Contains(trimmed, ".") {
					return fmt.Errorf("host %q is not an allowed host", host)
				}
				return nil
			},
		}
		httpServer := http.Server{
			Addr:    config.Address + ":" + strconv.Itoa(config.HTTPPort),
			Handler: server.router,
		}
		if config.ACMEEmailAddress != "" {
			httpServer.TLSConfig = manager.TLSConfig()
		}

		errs := make(chan error, 1)
		go func() {
			if config.ACMEEmailAddress != "" {
				errs <- httpServer.ListenAndServeTLS("", "")
			} else {
				errs <- httpServer.ListenAndServe()
			}
		}()
		select {
		case <-ctx.Done():
			grpcServer.Stop()
			httpServer.Shutdown(ctx)
			<-errs
			return nil
		case err := <-errs:
			grpcServer.Stop()
			return err
		}
	})
	go registry.reap(ctx)

	return group.Wait()
}
