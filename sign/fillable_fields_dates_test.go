package sign

import (
	"strings"
	"testing"
	"time"
)

func TestFormatDateString(t *testing.T) {
	layout := "01/02/2006 15:04"

	t.Run("UTC", func(t *testing.T) {
		date := time.Date(2024, 1, 15, 14, 30, 0, 0, time.UTC)
		got := formatDateString(date, layout)
		want := "01/15/2024 14:30 UTC"
		if got != want {
			t.Errorf("formatDateString(UTC) = %q, want %q", got, want)
		}
		if !strings.Contains(got, "UTC") {
			t.Errorf("formatDateString(UTC) must contain UTC, got %q", got)
		}
	})

	t.Run("positive offset", func(t *testing.T) {
		date := time.Date(2024, 1, 15, 14, 30, 0, 0, time.FixedZone("CET", 1*3600))
		got := formatDateString(date, layout)
		want := "01/15/2024 14:30 +01:00"
		if got != want {
			t.Errorf("formatDateString(+01:00) = %q, want %q", got, want)
		}
	})

	t.Run("negative offset", func(t *testing.T) {
		date := time.Date(2024, 1, 15, 14, 30, 0, 0, time.FixedZone("EST", -5*3600))
		got := formatDateString(date, layout)
		want := "01/15/2024 14:30 -05:00"
		if got != want {
			t.Errorf("formatDateString(-05:00) = %q, want %q", got, want)
		}
	})

	t.Run("offset with minutes", func(t *testing.T) {
		date := time.Date(2024, 1, 15, 14, 30, 0, 0, time.FixedZone("IST", 5*3600+30*60))
		got := formatDateString(date, layout)
		want := "01/15/2024 14:30 +05:30"
		if got != want {
			t.Errorf("formatDateString(+05:30) = %q, want %q", got, want)
		}
	})
}

func TestResolveDateLayout(t *testing.T) {
	t.Run("DateFormat takes precedence", func(t *testing.T) {
		got := resolveDateLayout("02.01.2006 15:04", "fr-FR")
		want := "02.01.2006 15:04"
		if got != want {
			t.Errorf("resolveDateLayout(custom, fr-FR) = %q, want %q", got, want)
		}
	})

	t.Run("empty DateFormat and Locale uses default US", func(t *testing.T) {
		got := resolveDateLayout("", "")
		want := "01/02/2006 15:04"
		if got != want {
			t.Errorf("resolveDateLayout(empty, empty) = %q, want %q", got, want)
		}
		got = resolveDateLayout("   ", "   ")
		if got != want {
			t.Errorf("resolveDateLayout(space, space) = %q, want %q", got, want)
		}
	})

	// Locales with hyphen (BCP 47)
	localeTests := []struct {
		locale string
		layout string
	}{
		{"en-US", "01/02/2006 15:04"},
		{"en-GB", "02/01/2006 15:04"},
		{"fr-FR", "02/01/2006 15:04"},
		{"de-DE", "02.01.2006 15:04"},
		{"es-ES", "02/01/2006 15:04"},
		{"it-IT", "02/01/2006 15:04"},
	}
	for _, tt := range localeTests {
		t.Run("locale_"+tt.locale, func(t *testing.T) {
			got := resolveDateLayout("", tt.locale)
			if got != tt.layout {
				t.Errorf("resolveDateLayout(empty, %q) = %q, want %q", tt.locale, got, tt.layout)
			}
		})
	}

	// Same locales with underscore
	for _, tt := range localeTests {
		underscore := strings.ReplaceAll(tt.locale, "-", "_")
		t.Run("locale_"+underscore, func(t *testing.T) {
			got := resolveDateLayout("", underscore)
			if got != tt.layout {
				t.Errorf("resolveDateLayout(empty, %q) = %q, want %q", underscore, got, tt.layout)
			}
		})
	}

	t.Run("unknown locale falls back to default", func(t *testing.T) {
		got := resolveDateLayout("", "nl-NL")
		want := "01/02/2006 15:04"
		if got != want {
			t.Errorf("resolveDateLayout(empty, nl-NL) = %q, want %q", got, want)
		}
	})
}
