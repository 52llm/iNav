package config

import "testing"

func TestLoadDefaults(t *testing.T) {
	t.Setenv("INAV_TOKEN", "secret")
	t.Setenv("INAV_DB_PATH", "")
	t.Setenv("INAV_LISTEN_ADDR", "")
	t.Setenv("INAV_PUBLIC_READ", "")

	c, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.Token != "secret" {
		t.Errorf("Token = %q, want %q", c.Token, "secret")
	}
	if c.DBPath != "inav.db" {
		t.Errorf("DBPath = %q, want %q", c.DBPath, "inav.db")
	}
	if c.ListenAddr != ":8080" {
		t.Errorf("ListenAddr = %q, want %q", c.ListenAddr, ":8080")
	}
	if c.PublicRead {
		t.Errorf("PublicRead = true, want false")
	}
}

func TestLoadRequiresToken(t *testing.T) {
	t.Setenv("INAV_TOKEN", "")
	if _, err := Load(); err == nil {
		t.Fatal("expected error when INAV_TOKEN is empty")
	}
}

func TestPublicReadTrue(t *testing.T) {
	t.Setenv("INAV_TOKEN", "x")
	t.Setenv("INAV_PUBLIC_READ", "true")
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if !c.PublicRead {
		t.Error("PublicRead = false, want true")
	}
}
