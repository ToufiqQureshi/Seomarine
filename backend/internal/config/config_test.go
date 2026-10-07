package config

import "testing"

func TestLoad(t *testing.T) {
	const dbURL = "postgres://localhost/seomarine"

	tests := []struct {
		name     string
		env      map[string]string
		wantAddr string
		wantErr  bool
	}{
		{name: "defaults the port", env: map[string]string{"DATABASE_URL": dbURL}, wantAddr: ":8080"},
		{name: "uses PORT", env: map[string]string{"DATABASE_URL": dbURL, "PORT": "3000"}, wantAddr: ":3000"},
		{name: "accepts the highest port", env: map[string]string{"DATABASE_URL": dbURL, "PORT": "65535"}, wantAddr: ":65535"},
		{name: "requires DATABASE_URL", env: map[string]string{"PORT": "3000"}, wantErr: true},
		{name: "rejects a non-numeric port", env: map[string]string{"DATABASE_URL": dbURL, "PORT": "http"}, wantErr: true},
		{name: "rejects port zero", env: map[string]string{"DATABASE_URL": dbURL, "PORT": "0"}, wantErr: true},
		{name: "rejects a port above 65535", env: map[string]string{"DATABASE_URL": dbURL, "PORT": "65536"}, wantErr: true},
		{name: "rejects a negative port", env: map[string]string{"DATABASE_URL": dbURL, "PORT": "-1"}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := Load(func(key string) string { return tt.env[key] })
			if tt.wantErr {
				if err == nil {
					t.Fatalf("Load() = %+v, want an error", cfg)
				}
				return
			}
			if err != nil {
				t.Fatalf("Load() error = %v", err)
			}
			if cfg.Addr != tt.wantAddr {
				t.Errorf("Addr = %q, want %q", cfg.Addr, tt.wantAddr)
			}
			if cfg.DatabaseURL != dbURL {
				t.Errorf("DatabaseURL = %q, want %q", cfg.DatabaseURL, dbURL)
			}
		})
	}
}
