package main

import (
	"os"
	"strings"
	"testing"
)

func TestGetSanitizedBaseName(t *testing.T) {
	fh := NewFileHandler()
	tests := []struct {
		input    string
		expected string
	}{
		{"Hello World!", "hello_world"},
		{"manga Page 1: Action!", "manga_page_1_action"},
		{"   Multiple    Spaces   ", "multiple_spaces"},
		{"!@#$%^&*", "generated_image"},
		{strings.Repeat("a", 100), strings.Repeat("a", 64)},
	}

	for _, tc := range tests {
		result := fh.GetSanitizedBaseName(tc.input)
		if result != tc.expected {
			t.Errorf("GetSanitizedBaseName(%q) = %q; expected %q", tc.input, result, tc.expected)
		}
	}
}

func TestValidateAuthentication(t *testing.T) {
	// Clean environment
	originalEnv := make(map[string]string)
	envVars := []string{
		"NANOBANANA_GEMINI_API_KEY",
		"NANOBANANA_GOOGLE_API_KEY",
		"GEMINI_API_KEY",
		"GOOGLE_API_KEY",
		"GEMINI_CLI_APP",
	}
	for _, env := range envVars {
		originalEnv[env] = os.Getenv(env)
		os.Unsetenv(env)
	}
	defer func() {
		for k, v := range originalEnv {
			if v != "" {
				os.Setenv(k, v)
			} else {
				os.Unsetenv(k)
			}
		}
	}()

	// 1. Test no key
	_, err := ValidateAuthentication()
	if err == nil {
		t.Error("expected error when no API key environment variables are set")
	}

	// 2. Test fallback order
	os.Setenv("GEMINI_CLI_APP", "key5")
	auth, err := ValidateAuthentication()
	if err != nil || auth.APIKey != "key5" || auth.KeyType != "GEMINI_API_KEY" {
		t.Errorf("expected GEMINI_CLI_APP to be used, got %v, err=%v", auth, err)
	}

	os.Setenv("GOOGLE_API_KEY", "key4")
	auth, err = ValidateAuthentication()
	if err != nil || auth.APIKey != "key4" || auth.KeyType != "GOOGLE_API_KEY" {
		t.Errorf("expected GOOGLE_API_KEY to override, got %v, err=%v", auth, err)
	}

	os.Setenv("GEMINI_API_KEY", "key3")
	auth, err = ValidateAuthentication()
	if err != nil || auth.APIKey != "key3" || auth.KeyType != "GEMINI_API_KEY" {
		t.Errorf("expected GEMINI_API_KEY to override, got %v, err=%v", auth, err)
	}

	os.Setenv("NANOBANANA_GOOGLE_API_KEY", "key2")
	auth, err = ValidateAuthentication()
	if err != nil || auth.APIKey != "key2" || auth.KeyType != "GOOGLE_API_KEY" {
		t.Errorf("expected NANOBANANA_GOOGLE_API_KEY to override, got %v, err=%v", auth, err)
	}

	os.Setenv("NANOBANANA_GEMINI_API_KEY", "key1")
	auth, err = ValidateAuthentication()
	if err != nil || auth.APIKey != "key1" || auth.KeyType != "GEMINI_API_KEY" {
		t.Errorf("expected NANOBANANA_GEMINI_API_KEY to override, got %v, err=%v", auth, err)
	}
}

func TestParseCharacters(t *testing.T) {
	globalContext := `
# Story Title

## Characters
* **Kenji (Tag: kenji)**: A high school student with messy hair.
  - Vibe: anxious but determined.
  - Outfit: school uniform.
* **Elara**: A mysterious wizard from another dimension.
  - Role: mentor.

### Packet-kun (Tag: pkt)
A small helper robot shaped like a network packet.
Vibe: enthusiastic.

## Rules
### Obsolete Section
This is just some text, not a character section.
`
	chars := parseCharacters(globalContext, "/tmp", false)
	if len(chars) != 3 {
		t.Errorf("expected 3 characters, got %d: %+v", len(chars), chars)
	}

	// Verify Kenji
	foundKenji := false
	for _, c := range chars {
		if c.Name == "Kenji" {
			foundKenji = true
			if c.Desc != "A high school student with messy hair. anxious but determined. school uniform." {
				t.Errorf("Kenji desc mismatch: got %q", c.Desc)
			}
			if !strings.HasSuffix(c.AbsPath, "characters/kenji_portrait.png") {
				t.Errorf("Kenji path mismatch: got %q", c.AbsPath)
			}
		}
	}
	if !foundKenji {
		t.Error("Kenji not found")
	}

	// Verify Packet-kun
	foundPacket := false
	for _, c := range chars {
		if c.Name == "Packet-kun" {
			foundPacket = true
			if c.Desc != "A small helper robot shaped like a network packet. Vibe: enthusiastic." {
				t.Errorf("Packet-kun desc mismatch: got %q", c.Desc)
			}
			if !strings.HasSuffix(c.AbsPath, "characters/pkt_portrait.png") {
				t.Errorf("Packet-kun path mismatch: got %q", c.AbsPath)
			}
		}
	}
	if !foundPacket {
		t.Error("Packet-kun not found")
	}
}

func TestParseEnvironments(t *testing.T) {
	globalContext := `
# Story Title

## Environment Setup
* **Unity HQ**: A sleek office building in Singapore with glass walls and metal beams.
  - Vibe: futuristic, corporate.
* **Singapore Office (Tag: singapore office env)**: The branch office with cubicles.

- **ENVIRONMENT ANCHORS**:
* **Kitchen (Tag: kit)**: Cozy kitchen with wooden cabinets.
`
	envs := parseEnvironments(globalContext, "/tmp")
	if len(envs) != 3 {
		t.Errorf("expected 3 environments, got %d: %+v", len(envs), envs)
	}

	// Verify Unity HQ
	foundHQ := false
	for _, e := range envs {
		if e.Name == "Unity HQ" {
			foundHQ = true
			if e.Desc != "A sleek office building in Singapore with glass walls and metal beams. Vibe: futuristic, corporate." {
				t.Errorf("Unity HQ desc mismatch: got %q", e.Desc)
			}
			if !strings.HasSuffix(e.AbsPath, "environments/unity_hq_env_far.png") {
				t.Errorf("Unity HQ path mismatch: got %q", e.AbsPath)
			}
		}
	}
	if !foundHQ {
		t.Error("Unity HQ not found")
	}

	// Verify Singapore Office
	foundOffice := false
	for _, e := range envs {
		if e.Name == "Singapore Office" {
			foundOffice = true
			if e.Desc != "The branch office with cubicles." {
				t.Errorf("Singapore Office desc mismatch: got %q", e.Desc)
			}
			if !strings.HasSuffix(e.AbsPath, "environments/singapore_office_env_env_far.png") {
				t.Errorf("Singapore Office path mismatch: got %q", e.AbsPath)
			}
		}
	}
	if !foundOffice {
		t.Error("Singapore Office not found")
	}

	// Verify Kitchen
	foundKitchen := false
	for _, e := range envs {
		if e.Name == "Kitchen" {
			foundKitchen = true
			if e.Desc != "Cozy kitchen with wooden cabinets." {
				t.Errorf("Kitchen desc mismatch: got %q", e.Desc)
			}
			if !strings.HasSuffix(e.AbsPath, "environments/kit_env_far.png") {
				t.Errorf("Kitchen path mismatch: got %q", e.AbsPath)
			}
		}
	}
	if !foundKitchen {
		t.Error("Kitchen not found")
	}
}
