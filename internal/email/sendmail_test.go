package email

import (
	"strings"
	"testing"
)

func TestWithDefaults(t *testing.T) {
	tests := []struct {
		name string
		in   Config
		want Config
	}{
		{
			name: "empty config gets every default",
			in:   Config{},
			want: Config{Host: DefaultHost, Port: DefaultPort, From: DefaultFrom},
		},
		{
			name: "explicit values are preserved",
			in:   Config{Host: "smtp.example.com", Port: 587, From: "certs@example.com"},
			want: Config{Host: "smtp.example.com", Port: 587, From: "certs@example.com"},
		},
		{
			name: "partial config only fills the gaps",
			in:   Config{Host: "relay.internal"},
			want: Config{Host: "relay.internal", Port: DefaultPort, From: DefaultFrom},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.in.WithDefaults(); got != tt.want {
				t.Fatalf("WithDefaults() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		config  Config
		wantErr bool
	}{
		{
			name:   "valid configuration without auth",
			config: Config{Host: "mailhog-service", Port: 1025, From: "no-reply@example.com"},
		},
		{
			name:   "valid configuration with auth",
			config: Config{Host: "smtp.example.com", Port: 587, From: "certs@example.com", Username: "user", Password: "secret"},
		},
		{
			name:    "missing host",
			config:  Config{Port: 1025, From: "no-reply@example.com"},
			wantErr: true,
		},
		{
			name:    "port out of range",
			config:  Config{Host: "mailhog-service", Port: 70000, From: "no-reply@example.com"},
			wantErr: true,
		},
		{
			name:    "missing sender",
			config:  Config{Host: "mailhog-service", Port: 1025},
			wantErr: true,
		},
		{
			name:    "password without username",
			config:  Config{Host: "smtp.example.com", Port: 587, From: "certs@example.com", Password: "secret"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.wantErr && err == nil {
				t.Fatal("Validate() error = nil, want error")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("Validate() error = %v, want nil", err)
			}
		})
	}
}

func TestSendReportsUnreachableServer(t *testing.T) {
	cfg := Config{Host: "127.0.0.1", Port: 1, From: "no-reply@example.com"}

	err := cfg.Send("subject", "body", "ops@example.com")
	if err == nil {
		t.Fatal("Send() error = nil, want a delivery error for an unreachable server")
	}
	if !strings.Contains(err.Error(), "failed to send email") {
		t.Fatalf("Send() error = %q, want it to mention the failed send", err)
	}
}
