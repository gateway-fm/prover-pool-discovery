package discovery

import (
	"strings"
)

// TransportProtocol represent available Transport
// Protocols for discovery
type TransportProtocol int

const (
	TransportHttp TransportProtocol = iota
	TransportHttps
	TransportWs
	TransportWss
	TransportGrpc
	TransportUnsupported
)

// Transports are slice of Service Statuses string representations
var Transports = [...]string{
	TransportHttp:  "http",
	TransportHttps: "https",
	TransportWs:    "ws",
	TransportWss:   "wss",
	TransportGrpc:  "grpc",
}

// String return Transport enum as a string
func (s TransportProtocol) String() string {
	return Transports[s] + "://"
}

// FormatAddress add protocol prefix to
// given address
func (s TransportProtocol) FormatAddress(addr string) string {
	if s == TransportGrpc {
		return addr
	}
	return s.String() + addr
}

// TransportFromString return new Transport enum from given
// string
func TransportFromString(s string) TransportProtocol {
	for i, r := range Transports {
		if strings.ToLower(s) == r {
			return TransportProtocol(i)
		}
	}
	return TransportUnsupported
}

// TransportFromStringE return new Transport enum from given
// string or return an error
func TransportFromStringE(s string) (TransportProtocol, error) {
	for i, r := range Transports {
		if strings.ToLower(s) == r {
			return TransportProtocol(i), nil
		}
	}
	return TransportUnsupported, ErrUnsupportedTransportProtocol{s}
}