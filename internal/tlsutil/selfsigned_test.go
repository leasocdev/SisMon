package tlsutil

import (
	"crypto/tls"
	"crypto/x509"
	"testing"
	"time"
)

func TestSelfSignedReused(t *testing.T) {
	dir := t.TempDir()
	c1, k1, err := EnsureSelfSigned(dir, []string{"nw.local"})
	if err != nil {
		t.Fatal(err)
	}
	c2, k2, err := EnsureSelfSigned(dir, []string{"nw.local"})
	if err != nil || c1 != c2 || k1 != k2 {
		t.Fatal("not reused", err)
	}
	pair, err := tls.LoadX509KeyPair(c1, k1)
	if err != nil {
		t.Fatal(err)
	}
	cert, _ := x509.ParseCertificate(pair.Certificate[0])
	if cert.NotAfter.Before(time.Now().AddDate(4, 0, 0)) {
		t.Fatal("validity < 4 years")
	}
	found := false
	for _, n := range cert.DNSNames {
		found = found || n == "localhost"
	}
	if !found || cert.VerifyHostname("nw.local") != nil {
		t.Fatal(cert.DNSNames)
	}
}
