// Package core owns sessions, the provider abstraction, and credential storage.
// The daemon, CLI, and desktop app all depend on this package.
package core

import "time"

// Credentials are short-lived credentials for one cloud session.
type Credentials struct {
	AccessKeyID     string     `json:"accessKeyId"`
	SecretAccessKey string     `json:"secretAccessKey"`
	SessionToken    string     `json:"sessionToken,omitempty"`
	Expiration      *time.Time `json:"expiration,omitempty"`
}

// Session is one configured way to obtain credentials.
type Session struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Region string `json:"region"`
}

// Provider is a credential source. Each cloud or auth flow implements it.
type Provider interface {
	// Kind returns the provider kind, for example "aws-sso".
	Kind() string
	// Credentials produces fresh credentials for session.
	Credentials(session Session) (Credentials, error)
}
