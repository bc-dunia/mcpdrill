package mcp

import (
	"fmt"
	"slices"
)

const (
	DefaultProtocolVersion = "2025-11-25"
	ClientName             = "mcpdrill"
	ClientVersion          = "1.0.0"
)

var SupportedProtocolVersions = []string{
	"2025-11-25",
	"2025-03-26",
	"2024-11-05",
}

type VersionPolicy string

const (
	VersionPolicyStrict    VersionPolicy = "strict"
	VersionPolicySupported VersionPolicy = "supported"
	VersionPolicyNone      VersionPolicy = "none"
)

func IsSupported(version string) bool {
	return slices.Contains(SupportedProtocolVersions, version)
}

func ValidateNegotiation(requested, returned string, policy VersionPolicy) error {
	switch policy {
	case VersionPolicyStrict:
		if returned != requested {
			return &VersionMismatchError{
				Requested: requested,
				Returned:  returned,
				Reason:    "strict policy requires exact version match",
			}
		}
	case VersionPolicySupported:
		if !IsSupported(returned) {
			return &VersionMismatchError{
				Requested: requested,
				Returned:  returned,
				Reason:    fmt.Sprintf("server returned unsupported version (supported: %v)", SupportedProtocolVersions),
			}
		}
	case VersionPolicyNone:
		return nil
	default:
		return ValidateNegotiation(requested, returned, VersionPolicyStrict)
	}
	return nil
}

func ParseVersionPolicy(s string) VersionPolicy {
	switch s {
	case "strict":
		return VersionPolicyStrict
	case "supported":
		return VersionPolicySupported
	case "none":
		return VersionPolicyNone
	default:
		return VersionPolicyStrict
	}
}

type VersionMismatchError struct {
	Requested string
	Returned  string
	Reason    string
}

func (e *VersionMismatchError) Error() string {
	return fmt.Sprintf("protocol version mismatch: requested %q, server returned %q: %s",
		e.Requested, e.Returned, e.Reason)
}
