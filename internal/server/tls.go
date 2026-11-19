package server

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/true18/shortener/internal/config"
)

func newHTTPServer(cfg config.Config, handler http.Handler) (*http.Server, error) {
	server := &http.Server{
		Addr:              cfg.ServerAddress,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
	}
	if !cfg.EnableHTTPS {
		return server, nil
	}

	tlsConfig, err := newTLSConfig(cfg.ServerAddress, rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("create TLS certificate: %w", err)
	}
	server.TLSConfig = tlsConfig

	return server, nil
}

func newTLSConfig(address string, random io.Reader) (*tls.Config, error) {
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), random)
	if err != nil {
		return nil, err
	}

	serialLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serial, err := rand.Int(random, serialLimit)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	template := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: "localhost"},
		NotBefore:    now.Add(-time.Minute),
		NotAfter:     now.Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:     []string{"localhost"},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1")},
	}
	addCertificateHost(template, address)

	der, err := x509.CreateCertificate(random, template, template, &privateKey.PublicKey, privateKey)
	if err != nil {
		return nil, err
	}
	template.Raw = der

	return &tls.Config{
		MinVersion: tls.VersionTLS12,
		Certificates: []tls.Certificate{{
			Certificate: [][]byte{der},
			PrivateKey:  privateKey,
			Leaf:        template,
		}},
	}, nil
}

func addCertificateHost(certificate *x509.Certificate, address string) {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		host = address
	}
	host = strings.Trim(host, "[]")
	if host == "" || host == "localhost" {
		return
	}

	if ip := net.ParseIP(host); ip != nil {
		certificate.IPAddresses = append(certificate.IPAddresses, ip)
		return
	}
	certificate.DNSNames = append(certificate.DNSNames, host)
}
