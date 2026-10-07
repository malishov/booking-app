package config

import "testing"

func TestLoad_RequiresDatabaseURL(t *testing.T) {
	t.Setenv("DATABASE_URL", "")

	if _, err := Load(); err == nil {
		t.Fatal("expected error when DATABASE_URL is empty, got nil")
	}
}

func TestLoad_AppliesDefaults(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://localhost/booking")
	t.Setenv("HTTP_ADDR", ":9000")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.HTTPAddr != ":9000" {
		t.Fatalf("HTTPAddr = %q, want :9000", cfg.HTTPAddr)
	}
	if cfg.DatabaseURL != "postgres://localhost/booking" {
		t.Fatalf("DatabaseURL = %q, want postgres://localhost/booking", cfg.DatabaseURL)
	}
}

func TestLoad_SessionCookieSecure(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://localhost/booking")
	for _, tc := range []struct {
		value         string
		want, wantErr bool
	}{
		{"true", true, false}, {"false", false, false}, {"invalid", false, true},
	} {
		t.Run(tc.value, func(t *testing.T) {
			t.Setenv("SESSION_COOKIE_SECURE", tc.value)
			cfg, err := Load()
			if (err != nil) != tc.wantErr {
				t.Fatalf("error = %v", err)
			}
			if err == nil && cfg.SessionCookieSecure != tc.want {
				t.Fatal("unexpected secure cookie setting")
			}
		})
	}
}
