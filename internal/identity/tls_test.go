package identity

import (
	"crypto/tls"
	"crypto/x509"
	"path/filepath"
	"testing"
	"time"
)

func TestPinAndExpiry(t *testing.T) {
	dir := t.TempDir()
	pin, err := Generate(dir)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := Config(filepath.Join(dir, "identity.pem"), filepath.Join(dir, "identity.key"), []string{pin}, false)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(cfg.Certificates[0].Certificate[0])
	if err != nil {
		t.Fatal(err)
	}
	state := tls.ConnectionState{PeerCertificates: []*x509.Certificate{cert}}
	if err = cfg.VerifyConnection(state); err != nil {
		t.Fatal(err)
	}
	cert.NotAfter = time.Now().Add(-time.Hour)
	if cfg.VerifyConnection(state) == nil {
		t.Fatal("expired certificate accepted")
	}
	if _, err = Generate(dir); err == nil {
		t.Fatal("identity silently overwritten")
	}
}
