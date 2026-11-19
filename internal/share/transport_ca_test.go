package share_test

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ArcKelMiranda/yhat-agent/internal/share"
)

// newTestCA builds a private CA plus a server certificate valid for
// 127.0.0.1. It returns the CA certificate, the server key pair and the PEM
// file that holds the CA, so the transport can be told to trust it.
func newTestCA(t *testing.T) (caCert *x509.Certificate, caKey *ecdsa.PrivateKey, serverCert tls.Certificate, caPEM []byte) {
	t.Helper()

	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate CA key: %v", err)
	}
	caTmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "YHat Test CA"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTmpl, caTmpl, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatalf("create CA certificate: %v", err)
	}
	caCert, err = x509.ParseCertificate(caDER)
	if err != nil {
		t.Fatalf("parse CA certificate: %v", err)
	}

	serverKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate server key: %v", err)
	}
	serverTmpl := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: "127.0.0.1"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1")},
		DNSNames:     []string{"localhost"},
	}
	serverDER, err := x509.CreateCertificate(rand.Reader, serverTmpl, caCert, &serverKey.PublicKey, caKey)
	if err != nil {
		t.Fatalf("create server certificate: %v", err)
	}
	serverCert = tls.Certificate{
		Certificate: [][]byte{serverDER, caDER},
		PrivateKey:  serverKey,
	}
	caPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER})
	return caCert, caKey, serverCert, caPEM
}

// TestNewTransportWithCA_TrustsSelfSignedServer verifies that a central
// server with a private (self-signed) authority is reachable when its CA is
// passed through central_ca_file.
func TestNewTransportWithCA_TrustsSelfSignedServer(t *testing.T) {
	_, _, serverCert, caPEM := newTestCA(t)

	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/memories" {
			t.Errorf("URL path: want /api/memories, got %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusCreated)
	}))
	srv.TLS = &tls.Config{Certificates: []tls.Certificate{serverCert}}
	srv.StartTLS()
	defer srv.Close()

	caPath := filepath.Join(t.TempDir(), "ca.crt")
	if err := os.WriteFile(caPath, caPEM, 0600); err != nil {
		t.Fatalf("write CA file: %v", err)
	}

	lt, err := share.NewTransportWithCA(srv.URL, "test-token", caPath)
	if err != nil {
		t.Fatalf("NewTransportWithCA: %v", err)
	}

	payload := share.LocalPayload{
		ID:      "ca-test-001",
		Origin:  "local",
		Type:    "decision",
		Title:   "CA trust test",
		Content: "The transport trusts the CA file.",
		Author:  "ca-tester",
	}
	if err := lt.Send(context.Background(), payload); err != nil {
		t.Fatalf("Send with CA file: %v", err)
	}
}

// TestNewTransportWithCA_FailsWithoutCAFile verifies that TLS verification is
// not silently disabled: without the CA file the same server is rejected.
func TestNewTransportWithCA_FailsWithoutCAFile(t *testing.T) {
	_, _, serverCert, _ := newTestCA(t)

	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
	}))
	srv.TLS = &tls.Config{Certificates: []tls.Certificate{serverCert}}
	srv.StartTLS()
	defer srv.Close()

	lt, err := share.NewTransportWithCA(srv.URL, "test-token", "")
	if err != nil {
		t.Fatalf("NewTransportWithCA: %v", err)
	}
	if err := lt.Send(context.Background(), share.LocalPayload{ID: "ca-test-002"}); err == nil {
		t.Fatal("Send without CA file: want TLS verification error, got nil")
	}
}

// TestNewTransportWithCA_UnreadableCAFile verifies that a missing CA file is
// reported instead of falling back to an unverified client.
func TestNewTransportWithCA_UnreadableCAFile(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "does-not-exist.crt")
	if _, err := share.NewTransportWithCA("https://localhost:8100", "test-token", missing); err == nil {
		t.Fatal("NewTransportWithCA with missing CA file: want error, got nil")
	}
}

// TestNewTransportWithCA_InvalidCAFile verifies that a file without any
// certificate is rejected.
func TestNewTransportWithCA_InvalidCAFile(t *testing.T) {
	caPath := filepath.Join(t.TempDir(), "broken.crt")
	if err := os.WriteFile(caPath, []byte("not a certificate\n"), 0600); err != nil {
		t.Fatalf("write broken CA file: %v", err)
	}
	if _, err := share.NewTransportWithCA("https://localhost:8100", "test-token", caPath); err == nil {
		t.Fatal("NewTransportWithCA with invalid CA file: want error, got nil")
	}
}
