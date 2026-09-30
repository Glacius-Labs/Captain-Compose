package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	paho "github.com/eclipse/paho.mqtt.golang"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

type testCA struct {
	cert *x509.Certificate
	key  *ecdsa.PrivateKey
}

func newTestCA(t *testing.T, name string) testCA {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	require.NoError(t, err)
	now := time.Now()
	template := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: name},
		NotBefore:             now.Add(-time.Minute),
		NotAfter:              now.Add(time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	require.NoError(t, err)
	cert, err := x509.ParseCertificate(der)
	require.NoError(t, err)
	return testCA{cert: cert, key: key}
}

func (ca testCA) issue(t *testing.T, name string, dns []string, ips []net.IP, usage x509.ExtKeyUsage) tls.Certificate {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	require.NoError(t, err)
	now := time.Now()
	template := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: name},
		NotBefore:    now.Add(-time.Minute),
		NotAfter:     now.Add(time.Hour),
		DNSNames:     dns,
		IPAddresses:  ips,
		ExtKeyUsage:  []x509.ExtKeyUsage{usage},
		KeyUsage:     x509.KeyUsageDigitalSignature,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, ca.cert, &key.PublicKey, ca.key)
	require.NoError(t, err)
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	require.NoError(t, err)
	certificate, err := tls.X509KeyPair(
		pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}),
		pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER}),
	)
	require.NoError(t, err)
	return certificate
}

func writeTestCA(t *testing.T, ca testCA) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "ca.pem")
	require.NoError(t, os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: ca.cert.Raw}), 0600))
	return path
}

// startTestMQTTBroker speaks only enough MQTT 3.1.1 to complete a Paho CONNECT.
// The TLS handshake is real, including certificate-chain, hostname, and mTLS checks.
func startTestMQTTBroker(t *testing.T, certificate tls.Certificate, clientAuth tls.ClientAuthType, clientCAs *x509.CertPool) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	tlsListener := tls.NewListener(listener, &tls.Config{
		MinVersion:   tls.VersionTLS12,
		Certificates: []tls.Certificate{certificate},
		ClientAuth:   clientAuth,
		ClientCAs:    clientCAs,
	})
	t.Cleanup(func() { _ = tlsListener.Close() })
	go func() {
		for {
			conn, err := tlsListener.Accept()
			if err != nil {
				return
			}
			go serveTestMQTTClient(conn)
		}
	}()
	return listener.Addr().String()
}

func serveTestMQTTClient(conn net.Conn) {
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
	var fixed [1]byte
	if _, err := conn.Read(fixed[:]); err != nil || fixed[0] != 0x10 {
		return
	}
	remaining := 0
	shift := uint(0)
	for i := 0; i < 4; i++ {
		var digit [1]byte
		if _, err := conn.Read(digit[:]); err != nil {
			return
		}
		remaining |= int(digit[0]&0x7f) << shift
		if digit[0]&0x80 == 0 {
			_, _ = io.CopyN(io.Discard, conn, int64(remaining))
			_, _ = conn.Write([]byte{0x20, 0x02, 0x00, 0x00}) // successful CONNACK
			return
		}
		shift += 7
	}
}

func connectWithTLS(t *testing.T, address, caPath, clientCertPath, clientKeyPath string) error {
	t.Helper()
	config := MQTTConfig{
		BrokerURL: "ssl://" + address,
		ClientID:  "tls-test-" + uuid.NewString(),
		TLS: TLSConfig{
			CACertPath:     caPath,
			ClientCertPath: clientCertPath,
			ClientKeyPath:  clientKeyPath,
		},
	}
	options, err := mqttOptions(config)
	require.NoError(t, err)
	options.SetConnectTimeout(2 * time.Second)
	options.SetMaxReconnectInterval(100 * time.Millisecond)
	client := paho.NewClient(options)
	defer client.Disconnect(100)
	token := client.Connect()
	if !token.WaitTimeout(4 * time.Second) {
		return fmt.Errorf("MQTT connect timed out")
	}
	return token.Error()
}

func TestMQTTTLSHandshakeAndCertificateValidation(t *testing.T) {
	ca := newTestCA(t, "Captain Compose test CA")
	caPath := writeTestCA(t, ca)
	serverCert := ca.issue(t, "localhost", nil, []net.IP{net.ParseIP("127.0.0.1")}, x509.ExtKeyUsageServerAuth)
	t.Run("trusted certificate succeeds", func(t *testing.T) {
		address := startTestMQTTBroker(t, serverCert, tls.NoClientCert, nil)
		require.NoError(t, connectWithTLS(t, address, caPath, "", ""))
	})
	t.Run("wrong hostname is rejected", func(t *testing.T) {
		address := startTestMQTTBroker(t, serverCert, tls.NoClientCert, nil)
		_, port, err := net.SplitHostPort(address)
		require.NoError(t, err)
		require.Error(t, connectWithTLS(t, net.JoinHostPort("localhost", port), caPath, "", ""))
	})
	t.Run("untrusted issuer is rejected", func(t *testing.T) {
		untrustedCA := newTestCA(t, "untrusted test CA")
		untrustedCert := untrustedCA.issue(t, "localhost", nil, []net.IP{net.ParseIP("127.0.0.1")}, x509.ExtKeyUsageServerAuth)
		address := startTestMQTTBroker(t, untrustedCert, tls.NoClientCert, nil)
		require.Error(t, connectWithTLS(t, address, caPath, "", ""))
	})
	t.Run("required client certificate is enforced", func(t *testing.T) {
		clientPool := x509.NewCertPool()
		clientPool.AddCert(ca.cert)
		address := startTestMQTTBroker(t, serverCert, tls.RequireAndVerifyClientCert, clientPool)
		require.Error(t, connectWithTLS(t, address, caPath, "", ""))
	})
	t.Run("client certificate succeeds", func(t *testing.T) {
		clientPool := x509.NewCertPool()
		clientPool.AddCert(ca.cert)
		address := startTestMQTTBroker(t, serverCert, tls.RequireAndVerifyClientCert, clientPool)
		clientCert := ca.issue(t, "agent", nil, nil, x509.ExtKeyUsageClientAuth)
		certPath := filepath.Join(t.TempDir(), "client.pem")
		keyPath := filepath.Join(t.TempDir(), "client.key")
		require.NoError(t, os.WriteFile(certPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: clientCert.Certificate[0]}), 0600))
		keyDER, err := x509.MarshalPKCS8PrivateKey(clientCert.PrivateKey)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(keyPath, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER}), 0600))
		require.NoError(t, connectWithTLS(t, address, caPath, certPath, keyPath))
	})
}
