package outputs

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"io/ioutil"

	"github.com/elastic/libbeat/logp"
)

var (
	// ErrNotACertificate indicates a PEM file to be loaded not being a valid
	// PEM file or certificate.
	ErrNotACertificate = errors.New("file is not a certificate")

	// ErrCertificateNoKey indicate a configuration error with missing key file
	ErrCertificateNoKey = errors.New("key file not configured")

	// ErrKeyNoCertificate indicate a configuration error with missing certificate file
	ErrKeyNoCertificate = errors.New("certificate file not configured")

	// ErrInvalidTLSVersion indicates an unknown tls version string given.
	ErrInvalidTLSVersion = errors.New("invalid TLS version string")

	// ErrUnknownCipherSuite indicates an unknown tls cipher suite being used
	ErrUnknownCipherSuite = errors.New("unknown cypher suite")

	// ErrUnknownCurveID indicates an unknown curve id has been configured
	ErrUnknownCurveID = errors.New("unknown curve id")
)

// TLSConfig defines config file options for TLS clients.
type TLSConfig struct {
	Disabled       bool
	Certificate    string
	CertificateKey string
	CAs            []string
	TLSInsecure    *bool
	CipherSuites   []string
	MinVersion     *string
	MaxVersion     *string
	CurveTypes     []string
}

// LoadTLSConfig will load a certificate from config with all TLS based keys
// defined. If Certificate and CertificateKey are configured, client authentication
// will be configured. If no CAs are configured, the host CA will be used by go
// built-in TLS support.
func LoadTLSConfig(config *TLSConfig) (*tls.Config, error) {
	if config == nil || config.Disabled {
		return nil, nil
	}

	certificate := config.Certificate
	key := config.CertificateKey
	rootCAs := config.CAs
	hasCertificate := certificate != ""
	hasKey := key != ""

	var certs []tls.Certificate
	switch {
	case hasCertificate && !hasKey:
		return nil, ErrCertificateNoKey
	case !hasCertificate && hasKey:
		return nil, ErrKeyNoCertificate
	case hasCertificate && hasKey:
		cert, err := tls.LoadX509KeyPair(certificate, key)
		if err != nil {
			logp.Critical("Failed loading client certificate", err)
			return nil, err
		}
		certs = []tls.Certificate{cert}
	}

	var roots *x509.CertPool
	if len(rootCAs) > 0 {
		roots = x509.NewCertPool()
		for _, caFile := range rootCAs {
			pemData, err := ioutil.ReadFile(caFile)
			if err != nil {
				logp.Critical("Failed reading CA certificate: %s", err)
				return nil, err
			}

			if ok := roots.AppendCertsFromPEM(pemData); !ok {
				return nil, ErrNotACertificate
			}
		}
	}

	insecureSkipVerify := false
	if config.TLSInsecure != nil {
		insecureSkipVerify = *config.TLSInsecure
	}

	minVersion, err := parseTLSVersion(config.MinVersion)
	if err != nil {
		return nil, err
	}

	maxVersion, err := parseTLSVersion(config.MaxVersion)
	if err != nil {
		return nil, err
	}

	cipherSuites, err := parseTLSCipherSuites(config.CipherSuites)
	if err != nil {
		return nil, err
	}

	curveIDs, err := parseCurveTypes(config.CurveTypes)
	if err != nil {
		return nil, err
	}

	tlsConfig := tls.Config{
		MinVersion:         minVersion,
		MaxVersion:         maxVersion,
		Certificates:       certs,
		RootCAs:            roots,
		InsecureSkipVerify: insecureSkipVerify,
		CipherSuites:       cipherSuites,
		CurvePreferences:   curveIDs,
	}
	return &tlsConfig, nil
}

func parseTLSVersion(s *string) (uint16, error) {
	if s == nil {
		return 0, nil
	}

	switch *s {
	case "SSL-3.0":
		return tls.VersionSSL30, nil
	case "1.0":
		return tls.VersionTLS10, nil
	case "1.1":
		return tls.VersionTLS11, nil
	case "1.2":
		return tls.VersionTLS12, nil
	default:
		return 0, ErrInvalidTLSVersion
	}
}

func parseTLSCipherSuites(names []string) ([]uint16, error) {
	suites := map[string]uint16{
		"RSA-RC4-128-SHA":                tls.TLS_RSA_WITH_RC4_128_SHA,
		"RSA-3DES-EDE-CBC-SHA":           tls.TLS_RSA_WITH_3DES_EDE_CBC_SHA,
		"RSA-AES-128-CBC-SHA":            tls.TLS_RSA_WITH_AES_128_CBC_SHA,
		"RSA-AES-256-CBC-SHA":            tls.TLS_RSA_WITH_AES_256_CBC_SHA,
		"ECDHE-ECDSA-RC4-128-SHA":        tls.TLS_ECDHE_ECDSA_WITH_RC4_128_SHA,
		"ECDHE-ECDSA-AES-128-CBC-SHA":    tls.TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA,
		"ECDHE-ECDSA-AES-256-CBC-SHA":    tls.TLS_ECDHE_ECDSA_WITH_AES_256_CBC_SHA,
		"ECDHE-RSA-RC4-128-SHA":          tls.TLS_ECDHE_RSA_WITH_RC4_128_SHA,
		"ECDHE-RSA-3DES-EDE-CBC-SHA":     tls.TLS_ECDHE_RSA_WITH_3DES_EDE_CBC_SHA,
		"ECDHE-RSA-AES-128-CBC-SHA":      tls.TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA,
		"ECDHE-RSA-AES-256-CBC-SHA":      tls.TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA,
		"ECDHE-RSA-AES-128-GCM-SHA256":   tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
		"ECDHE-ECDSA-AES-128-GCM-SHA256": tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
		"ECDHE-RSA-AES-256-GCM-SHA384":   tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
		"ECDHE-ECDSA-AES-256-GCM-SHA384": tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
	}

	var list []uint16
	for _, name := range names {
		id, ok := suites[name]
		if !ok {
			return nil, ErrUnknownCipherSuite
		}

		list = append(list, id)
	}
	return list, nil
}

func parseCurveTypes(names []string) ([]tls.CurveID, error) {
	curveIDs := map[string]tls.CurveID{
		"P256": tls.CurveP256,
		"P384": tls.CurveP384,
		"P521": tls.CurveP521,
	}

	var list []tls.CurveID
	for _, name := range names {
		id, ok := curveIDs[name]
		if !ok {
			return nil, ErrUnknownCurveID
		}
		list = append(list, id)
	}
	return list, nil
}
