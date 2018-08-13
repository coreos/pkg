package k8stlsutil

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"math"
	"math/big"
	"net"
	"time"
)

const (
	RSAKeySize   = 2048
	Duration365d = time.Hour * 24 * 365
)

type CertConfig struct {
	CommonName   string
	Organization []string
	AltNames     AltNames
}

// AltNames contains the domain names and IP addresses that will be added
// to the API Server's x509 certificate SubAltNames field. The values will
// be passed directly to the x509.Certificate object.
type AltNames struct {
	DNSNames []string
	IPs      []net.IP
}

func NewPrivateKey() (*rsa.PrivateKey, error) {
	return rsa.GenerateKey(rand.Reader, RSAKeySize)
}

func EncodePublicKeyPEM(key *rsa.PublicKey) ([]byte, error) {
	der, err := x509.MarshalPKIXPublicKey(key)
	if err != nil {
		return []byte{}, err
	}
	block := pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: der,
	}
	return pem.EncodeToMemory(&block), nil
}

func EncodePrivateKeyPEM(key *rsa.PrivateKey) []byte {
	block := pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	}
	return pem.EncodeToMemory(&block)
}

func EncodeCertificatePEM(cert *x509.Certificate) []byte {
	block := pem.Block{
		Type:  "CERTIFICATE",
		Bytes: cert.Raw,
	}
	return pem.EncodeToMemory(&block)
}

func NewSelfSignedCACertificate(cfg CertConfig, key *rsa.PrivateKey, validDuration time.Duration) (*x509.Certificate, error) {
	now := time.Now()

	tmpl := x509.Certificate{
		SerialNumber: new(big.Int).SetInt64(0),
		Subject: pkix.Name{
			CommonName:   cfg.CommonName,
			Organization: cfg.Organization,
		},
		NotBefore:             now,
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
		IsCA: true,
	}

	switch {
	case validDuration == 0:
		// 0 means default expiry of roughly 10 years from now
		tmpl.NotAfter = now.Add(Duration365d * 10)
	case validDuration < 0:
		// < 0 indicates that the certificate has "no well-defined expiration
		// date" as per rfc5280(4.1.2.5), and "SHOULD be assigned the
		// GeneralizedTime value of 99991231235959Z"
		tmpl.NotAfter = time.Date(9999, 12, 31, 23, 59, 59, 0, time.UTC)
	default:
		tmpl.NotAfter = now.Add(validDuration)
	}

	certDERBytes, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, key.Public(), key)
	if err != nil {
		return nil, err
	}
	return x509.ParseCertificate(certDERBytes)
}

func ParsePEMEncodedCACert(pemdata []byte) (*x509.Certificate, error) {
	decoded, _ := pem.Decode(pemdata)
	if decoded == nil {
		return nil, errors.New("no PEM data found")
	}
	return x509.ParseCertificate(decoded.Bytes)
}

func ParsePEMEncodedPrivateKey(pemdata []byte) (*rsa.PrivateKey, error) {
	decoded, _ := pem.Decode(pemdata)
	if decoded == nil {
		return nil, errors.New("no PEM data found")
	}
	return x509.ParsePKCS1PrivateKey(decoded.Bytes)
}

func NewSignedCertificate(cfg CertConfig, key *rsa.PrivateKey, caCert *x509.Certificate, caKey *rsa.PrivateKey, validDuration time.Duration) (*x509.Certificate, error) {
	serial, err := rand.Int(rand.Reader, new(big.Int).SetInt64(math.MaxInt64))
	if err != nil {
		return nil, err
	}

	certTmpl := x509.Certificate{
		Subject: pkix.Name{
			CommonName:   cfg.CommonName,
			Organization: caCert.Subject.Organization,
		},
		DNSNames:     cfg.AltNames.DNSNames,
		IPAddresses:  cfg.AltNames.IPs,
		SerialNumber: serial,
		NotBefore:    caCert.NotBefore,
		KeyUsage:     x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
	}

	switch {
	case validDuration == 0:
		// 0 means default expiry of roughly 1 year from now
		certTmpl.NotAfter = time.Now().Add(Duration365d)
	case validDuration < 0:
		// < 0 indicates that the certificate has "no well-defined expiration
		// date" as per rfc5280(4.1.2.5), and "SHOULD be assigned the
		// GeneralizedTime value of 99991231235959Z"
		certTmpl.NotAfter = time.Date(9999, 12, 31, 23, 59, 59, 0, time.UTC)
	default:
		certTmpl.NotAfter = time.Now().Add(validDuration)
	}

	certDERBytes, err := x509.CreateCertificate(rand.Reader, &certTmpl, caCert, key.Public(), caKey)
	if err != nil {
		return nil, err
	}
	return x509.ParseCertificate(certDERBytes)
}
