package node

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"os"
	"time"
)

// CA holds a generated certificate authority's PEM-encoded cert/key and
// the parsed objects needed to sign node certificates.
type CA struct {
	CertPEM []byte
	KeyPEM  []byte
	cert    *x509.Certificate
	key     *ecdsa.PrivateKey
}

// NodeTLSCreds holds a node's PEM-encoded certificate and private key,
// both signed by a CA.
type NodeTLSCreds struct {
	CertPEM []byte
	KeyPEM  []byte
}

// GenerateCA creates a self-signed certificate authority.
func GenerateCA() (*CA, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	template := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "ZephyrCache CA"},
		NotBefore:             time.Now().Add(-time.Minute),
		NotAfter:              time.Now().Add(24 * time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
	}
	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		return nil, err
	}
	cert, err := x509.ParseCertificate(certDER)
	if err != nil {
		return nil, err
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return nil, err
	}
	return &CA{
		CertPEM: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER}),
		KeyPEM:  pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}),
		cert:    cert,
		key:     key,
	}, nil
}

// ClusterServerName is the synthetic SAN every node cert carries. ClientTLSConfig
// pins this name so peers can dial each other by container ID or IP without
// hostname mismatches.
const ClusterServerName = "zephyr-cluster"

// GenerateNodeCert creates a certificate signed by ca valid for the given hosts
// (IP addresses or DNS names). ClusterServerName is always added to the SAN list.
func GenerateNodeCert(ca *CA, hosts []string) (*NodeTLSCreds, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: "ZephyrCache Node"},
		NotBefore:    time.Now().Add(-time.Minute),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
	}
	template.DNSNames = append(template.DNSNames, ClusterServerName)
	for _, h := range hosts {
		if ip := net.ParseIP(h); ip != nil {
			template.IPAddresses = append(template.IPAddresses, ip)
		} else {
			template.DNSNames = append(template.DNSNames, h)
		}
	}
	certDER, err := x509.CreateCertificate(rand.Reader, template, ca.cert, &key.PublicKey, ca.key)
	if err != nil {
		return nil, err
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return nil, err
	}
	return &NodeTLSCreds{
		CertPEM: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER}),
		KeyPEM:  pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}),
	}, nil
}

// ServerTLSConfig returns a *tls.Config for the replication HTTPS server.
// It requires and verifies client certificates signed by caPEM (mTLS).
func ServerTLSConfig(caPEM []byte, creds *NodeTLSCreds) (*tls.Config, error) {
	cert, err := tls.X509KeyPair(creds.CertPEM, creds.KeyPEM)
	if err != nil {
		return nil, err
	}
	pool := x509.NewCertPool()
	pool.AppendCertsFromPEM(caPEM)
	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		ClientAuth:   tls.RequireAndVerifyClientCert,
		ClientCAs:    pool,
		MinVersion:   tls.VersionTLS13,
	}, nil
}

// ClientTLSConfig returns a *tls.Config for outgoing replication HTTPS calls.
// It presents the node's own certificate and trusts only caPEM.
func ClientTLSConfig(caPEM []byte, creds *NodeTLSCreds) (*tls.Config, error) {
	cert, err := tls.X509KeyPair(creds.CertPEM, creds.KeyPEM)
	if err != nil {
		return nil, err
	}
	pool := x509.NewCertPool()
	pool.AppendCertsFromPEM(caPEM)
	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		RootCAs:      pool,
		// All cluster nodes share one cert containing this SAN; pinning to it
		// lets us dial peers by container ID / IP without hostname mismatch.
		ServerName: ClusterServerName,
		MinVersion: tls.VersionTLS13,
	}, nil
}

// ClientFacingServerTLSConfig returns a *tls.Config for the client-facing HTTPS
// server. Unlike the replication endpoint, clients are not required to present
// certificates (one-way TLS).
func ClientFacingServerTLSConfig(creds *NodeTLSCreds) (*tls.Config, error) {
	cert, err := tls.X509KeyPair(creds.CertPEM, creds.KeyPEM)
	if err != nil {
		return nil, err
	}
	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS13,
	}, nil
}

// ConfigureClientFacingTLS reads CLIENT_CERT_FILE and CLIENT_KEY_FILE from the
// environment. When unset it falls back to REPLICA_CERT_FILE / REPLICA_KEY_FILE.
// If no cert is available the client endpoint stays on plain HTTP.
func ConfigureClientFacingTLS(n *Node) error {
	certFile := os.Getenv("CLIENT_CERT_FILE")
	keyFile := os.Getenv("CLIENT_KEY_FILE")
	if certFile == "" || keyFile == "" {
		certFile = os.Getenv("REPLICA_CERT_FILE")
		keyFile = os.Getenv("REPLICA_KEY_FILE")
	}
	if certFile == "" || keyFile == "" {
		return nil
	}
	certPEM, err := os.ReadFile(certFile)
	if err != nil {
		return err
	}
	keyPEM, err := os.ReadFile(keyFile)
	if err != nil {
		return err
	}
	cfg, err := ClientFacingServerTLSConfig(&NodeTLSCreds{CertPEM: certPEM, KeyPEM: keyPEM})
	if err != nil {
		return err
	}
	n.SetClientFacingTLS(cfg)
	return nil
}

// configureTLS reads REPLICA_CERT_FILE, REPLICA_KEY_FILE, and REPLICA_CA_FILE
// from the environment and, when all three are set, enables mTLS for the
// replication endpoint. If none are set the node runs without TLS.
func ConfigureTLS(n *Node) error {
	certFile := os.Getenv("REPLICA_CERT_FILE")
	keyFile := os.Getenv("REPLICA_KEY_FILE")
	caFile := os.Getenv("REPLICA_CA_FILE")
	if certFile == "" && keyFile == "" && caFile == "" {
		return nil
	}
	certPEM, err := os.ReadFile(certFile)
	if err != nil {
		return err
	}
	keyPEM, err := os.ReadFile(keyFile)
	if err != nil {
		return err
	}
	caPEM, err := os.ReadFile(caFile)
	if err != nil {
		return err
	}
	creds := &NodeTLSCreds{CertPEM: certPEM, KeyPEM: keyPEM}
	serverTLS, err := ServerTLSConfig(caPEM, creds)
	if err != nil {
		return err
	}
	clientTLS, err := ClientTLSConfig(caPEM, creds)
	if err != nil {
		return err
	}
	n.SetReplicaTLS(serverTLS, clientTLS)
	return nil
}
