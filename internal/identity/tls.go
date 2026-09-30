// Package identity authenticates both tunnel peers with explicitly provisioned SPKI pins.
package identity

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"math/big"
	"networkroute/internal/protocol"
	"os"
	"path/filepath"
	"time"
)

func Pin(cert *x509.Certificate) string {
	sum := sha256.Sum256(cert.RawSubjectPublicKeyInfo)
	return hex.EncodeToString(sum[:])
}

// WritePrivateFile exclusively creates a file using the same ACL as identity keys.
func WritePrivateFile(path string, data []byte) error {
	f, err := createPrivateFile(path)
	if err != nil {
		return err
	}
	_, err = f.Write(data)
	if err == nil {
		err = f.Sync()
	}
	closed := f.Close()
	if err != nil {
		return err
	}
	return closed
}

// Existing reuses an identity only if both files match and the certificate is valid.
func Existing(dir string) (string, error) {
	certFile, keyFile := filepath.Join(dir, "identity.pem"), filepath.Join(dir, "identity.key")
	_, ce := os.Stat(certFile)
	_, ke := os.Stat(keyFile)
	if os.IsNotExist(ce) && os.IsNotExist(ke) {
		return Generate(dir)
	}
	if ce != nil || ke != nil {
		return "", fmt.Errorf("identity is incomplete or inaccessible; choose another directory")
	}
	pair, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return "", err
	}
	cert, err := x509.ParseCertificate(pair.Certificate[0])
	if err != nil {
		return "", err
	}
	now := time.Now()
	if now.Before(cert.NotBefore) || now.After(cert.NotAfter) {
		return "", fmt.Errorf("local identity expired or not yet valid")
	}
	return Pin(cert), nil
}
func Generate(dir string) (string, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	pub, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return "", err
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return "", err
	}
	now := time.Now()
	tpl := &x509.Certificate{SerialNumber: serial, Subject: pkix.Name{CommonName: "NetworkRoute peer"}, NotBefore: now.Add(-5 * time.Minute), NotAfter: now.AddDate(1, 0, 0), KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth}}
	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, pub, key)
	if err != nil {
		return "", err
	}
	raw, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return "", err
	}
	for _, item := range []struct {
		name, kind string
		data       []byte
	}{{"identity.key", "PRIVATE KEY", raw}, {"identity.pem", "CERTIFICATE", der}} {
		f, err := createPrivateFile(filepath.Join(dir, item.name))
		if err != nil {
			return "", fmt.Errorf("create identity (never overwrites): %w", err)
		}
		err = pem.Encode(f, &pem.Block{Type: item.kind, Bytes: item.data})
		ce := f.Close()
		if err != nil {
			return "", err
		}
		if ce != nil {
			return "", ce
		}
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		return "", err
	}
	return Pin(cert), nil
}
func Config(certFile, keyFile string, pins []string, server bool) (*tls.Config, error) {
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, fmt.Errorf("load tunnel identity: %w", err)
	}
	if len(pins) == 0 {
		return nil, fmt.Errorf("at least one trusted peer pin is required")
	}
	decoded := make([][]byte, 0, len(pins))
	for _, pin := range pins {
		p, err := hex.DecodeString(pin)
		if err != nil || len(p) != 32 {
			return nil, fmt.Errorf("peer pin must be 64 hex characters")
		}
		decoded = append(decoded, p)
	}
	cfg := &tls.Config{MinVersion: tls.VersionTLS13, Certificates: []tls.Certificate{cert}, NextProtos: []string{protocol.ALPN}}
	// Certificate pin verification below is mandatory when PKI hostname verification is disabled.
	if server {
		cfg.ClientAuth = tls.RequireAnyClientCert
	} else {
		cfg.InsecureSkipVerify = true
	}
	cfg.VerifyConnection = func(cs tls.ConnectionState) error {
		if len(cs.PeerCertificates) == 0 {
			return fmt.Errorf("missing peer identity")
		}
		c := cs.PeerCertificates[0]
		now := time.Now()
		if now.Before(c.NotBefore) || now.After(c.NotAfter) {
			return fmt.Errorf("peer identity expired or not yet valid")
		}
		sum := sha256.Sum256(c.RawSubjectPublicKeyInfo)
		match := 0
		for _, p := range decoded {
			match |= subtle.ConstantTimeCompare(sum[:], p)
		}
		if match != 1 {
			return fmt.Errorf("peer public key is not trusted")
		}
		return nil
	}
	return cfg, nil
}
