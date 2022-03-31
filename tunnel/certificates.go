package tunnel

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net/url"
	"os"
	"time"

	"google.golang.org/grpc/credentials"
)

var (
	rootCA            *ca
	serverCredentials credentials.TransportCredentials
)

func init() {
	var err error
	rootCA, err = generateCA()
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}

	certBytes, privateKeyBytes, err := rootCA.generate("server", "")
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
	serverCert, err := tls.X509KeyPair(certBytes, privateKeyBytes)
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}

	certPool := x509.NewCertPool()
	if !certPool.AppendCertsFromPEM(rootCA.PEM) {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}

	serverCredentials = credentials.NewTLS(&tls.Config{
		Certificates: []tls.Certificate{serverCert},
		ClientAuth:   tls.RequireAndVerifyClientCert,
		ClientCAs:    certPool,
	})
}

func getSVID(host, nonce string) *url.URL {
	var svid url.URL
	svid.Scheme = "spiffe"
	svid.Host = host
	svid.Path = nonce
	return &svid
}

type ca struct {
	Certificate *x509.Certificate
	PEM         []byte
	PrivateKey  *ecdsa.PrivateKey
	X509        tls.Certificate
}

func (c *ca) generate(host, nonce string) ([]byte, []byte, error) {
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, err
	}
	spiffe := getSVID(host, nonce)
	serial, err := serialNumber()
	if err != nil {
		return nil, nil, err
	}
	cert := &x509.Certificate{
		SerialNumber: serial,
		DNSNames:     []string{host},
		URIs:         []*url.URL{spiffe},
		Subject: pkix.Name{
			Organization: []string{"Tunnel"},
		},
		NotBefore:             time.Now().Add(-2 * time.Minute),
		NotAfter:              time.Now().AddDate(1, 0, 0),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}

	data, err := x509.CreateCertificate(rand.Reader, cert, c.Certificate, &privateKey.PublicKey, c.PrivateKey)
	if err != nil {
		return nil, nil, err
	}

	encoded, err := x509.MarshalECPrivateKey(privateKey)
	if err != nil {
		return nil, nil, err
	}
	certificatePEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: data,
	})
	privateKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "EC PRIVATE KEY",
		Bytes: encoded,
	})

	return certificatePEM, privateKeyPEM, nil
}

func generateCA() (*ca, error) {
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	serial, err := serialNumber()
	if err != nil {
		return nil, err
	}
	cert := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			Organization: []string{"Tunnel CA"},
		},
		IsCA:                  true,
		NotBefore:             time.Now().Add(-10 * time.Minute),
		NotAfter:              time.Now().AddDate(10, 0, 0),
		KeyUsage:              x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
	}

	data, err := x509.CreateCertificate(rand.Reader, cert, cert, &privateKey.PublicKey, privateKey)
	if err != nil {
		return nil, err
	}

	encoded, err := x509.MarshalECPrivateKey(privateKey)
	if err != nil {
		return nil, err
	}
	certificatePEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: data,
	})
	privateKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "EC PRIVATE KEY",
		Bytes: encoded,
	})
	x509Cert, err := tls.X509KeyPair(certificatePEM, privateKeyPEM)
	if err != nil {
		return nil, err
	}

	return &ca{
		Certificate: cert,
		PEM:         certificatePEM,
		PrivateKey:  privateKey,
		X509:        x509Cert,
	}, nil
}

func serialNumber() (*big.Int, error) {
	max := new(big.Int)
	max.Exp(big.NewInt(2), big.NewInt(80), nil).Sub(max, big.NewInt(1))

	return rand.Int(rand.Reader, max)
}
