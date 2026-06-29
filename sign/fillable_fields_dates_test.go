package sign

import (
	"strings"
	"testing"
	"time"
)

func TestApplyTimezone(t *testing.T) {
	utc := time.Date(2024, 1, 15, 14, 30, 0, 0, time.UTC)

	t.Run("empty timezone passthrough", func(t *testing.T) {
		got, err := applyTimezone(utc, "")
		if err != nil {
			t.Fatalf("applyTimezone(empty) error: %v", err)
		}
		if !got.Equal(utc) {
			t.Errorf("applyTimezone(empty) = %v, want %v", got, utc)
		}
	})

	t.Run("UTC to Europe/Paris winter", func(t *testing.T) {
		got, err := applyTimezone(utc, "Europe/Paris")
		if err != nil {
			t.Fatalf("applyTimezone(Europe/Paris) error: %v", err)
		}
		want := time.Date(2024, 1, 15, 15, 30, 0, 0, mustLoadLocation(t, "Europe/Paris"))
		if !got.Equal(want) {
			t.Errorf("applyTimezone(Europe/Paris) = %v, want %v", got, want)
		}
	})

	t.Run("UTC to Europe/Paris summer", func(t *testing.T) {
		summer := time.Date(2024, 7, 15, 14, 30, 0, 0, time.UTC)
		got, err := applyTimezone(summer, "Europe/Paris")
		if err != nil {
			t.Fatalf("applyTimezone(Europe/Paris) error: %v", err)
		}
		want := time.Date(2024, 7, 15, 16, 30, 0, 0, mustLoadLocation(t, "Europe/Paris"))
		if !got.Equal(want) {
			t.Errorf("applyTimezone(Europe/Paris summer) = %v, want %v", got, want)
		}
	})

	t.Run("invalid timezone", func(t *testing.T) {
		_, err := applyTimezone(utc, "Not/A/Timezone")
		if err == nil {
			t.Fatal("applyTimezone(invalid) expected error")
		}
		if !strings.Contains(err.Error(), "invalid timezone") {
			t.Errorf("applyTimezone(invalid) error = %v, want invalid timezone message", err)
		}
	})
}

func mustLoadLocation(t *testing.T, name string) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation(name)
	if err != nil {
		t.Fatalf("LoadLocation(%q): %v", name, err)
	}
	return loc
}

func TestFormatTimezoneSuffix(t *testing.T) {
	t.Run("UTC", func(t *testing.T) {
		date := time.Date(2024, 1, 15, 14, 30, 0, 0, time.UTC)
		got := formatTimezoneSuffix(date)
		if got != "UTC" {
			t.Errorf("formatTimezoneSuffix(UTC) = %q, want UTC", got)
		}
	})

	t.Run("IANA abbreviation CET", func(t *testing.T) {
		loc := mustLoadLocation(t, "Europe/Paris")
		date := time.Date(2024, 1, 15, 14, 30, 0, 0, loc)
		got := formatTimezoneSuffix(date)
		if got != "CET" {
			t.Errorf("formatTimezoneSuffix(CET) = %q, want CET", got)
		}
	})

	t.Run("IANA abbreviation CEST", func(t *testing.T) {
		loc := mustLoadLocation(t, "Europe/Paris")
		date := time.Date(2024, 7, 15, 14, 30, 0, 0, loc)
		got := formatTimezoneSuffix(date)
		if got != "CEST" {
			t.Errorf("formatTimezoneSuffix(CEST) = %q, want CEST", got)
		}
	})

	t.Run("FixedZone abbreviation EST", func(t *testing.T) {
		date := time.Date(2024, 1, 15, 14, 30, 0, 0, time.FixedZone("EST", -5*3600))
		got := formatTimezoneSuffix(date)
		if got != "EST" {
			t.Errorf("formatTimezoneSuffix(EST) = %q, want EST", got)
		}
	})

	t.Run("offset fallback when name unusable", func(t *testing.T) {
		date := time.Date(2024, 1, 15, 14, 30, 0, 0, time.FixedZone("", 5*3600+30*60))
		got := formatTimezoneSuffix(date)
		want := "+05:30"
		if got != want {
			t.Errorf("formatTimezoneSuffix(empty name) = %q, want %q", got, want)
		}
	})
}

func TestFormatDateString(t *testing.T) {
	layout := "01/02/2006 15:04"

	t.Run("UTC", func(t *testing.T) {
		date := time.Date(2024, 1, 15, 14, 30, 0, 0, time.UTC)
		got := formatDateString(date, layout)
		want := "01/15/2024 14:30 UTC"
		if got != want {
			t.Errorf("formatDateString(UTC) = %q, want %q", got, want)
		}
	})

	t.Run("positive offset with abbreviation", func(t *testing.T) {
		date := time.Date(2024, 1, 15, 14, 30, 0, 0, time.FixedZone("CET", 1*3600))
		got := formatDateString(date, layout)
		want := "01/15/2024 14:30 CET"
		if got != want {
			t.Errorf("formatDateString(CET) = %q, want %q", got, want)
		}
	})

	t.Run("negative offset with abbreviation", func(t *testing.T) {
		date := time.Date(2024, 1, 15, 14, 30, 0, 0, time.FixedZone("EST", -5*3600))
		got := formatDateString(date, layout)
		want := "01/15/2024 14:30 EST"
		if got != want {
			t.Errorf("formatDateString(EST) = %q, want %q", got, want)
		}
	})

	t.Run("offset fallback with minutes", func(t *testing.T) {
		date := time.Date(2024, 1, 15, 14, 30, 0, 0, time.FixedZone("", 5*3600+30*60))
		got := formatDateString(date, layout)
		want := "01/15/2024 14:30 +05:30"
		if got != want {
			t.Errorf("formatDateString(+05:30) = %q, want %q", got, want)
		}
	})
}

func TestFormatFillableDate(t *testing.T) {
	utc := time.Date(2024, 1, 15, 14, 30, 0, 0, time.UTC)
	paris, err := applyTimezone(utc, "Europe/Paris")
	if err != nil {
		t.Fatalf("applyTimezone: %v", err)
	}
	ny, err := applyTimezone(utc, "America/New_York")
	if err != nil {
		t.Fatalf("applyTimezone: %v", err)
	}

	tests := []struct {
		name string
		date time.Time
		app  Appearance
		want string
	}{
		{
			name: "numeric en-US with EST",
			date: time.Date(2024, 1, 15, 14, 30, 0, 0, time.FixedZone("EST", -5*3600)),
			app:  Appearance{Locale: "en-US", DateStyle: DateStyleNumeric},
			want: "01/15/2024 14:30 EST",
		},
		{
			name: "numeric fr-FR Paris",
			date: paris,
			app:  Appearance{Locale: "fr-FR", DateStyle: DateStyleNumeric},
			want: "15/01/2024 15:30 CET",
		},
		{
			name: "date-only fr-FR Paris",
			date: paris,
			app:  Appearance{Locale: "fr-FR", DateStyle: DateStyleDateOnly},
			want: "15/01/2024 CET",
		},
		{
			name: "long en-US Paris converted",
			date: paris,
			app:  Appearance{Locale: "en-US", DateStyle: DateStyleLong},
			want: "January 15, 2024, 3:30 PM CET",
		},
		{
			name: "long fr-FR Paris",
			date: paris,
			app:  Appearance{Locale: "fr-FR", DateStyle: DateStyleLong},
			want: "15 janvier 2024, 15:30 CET",
		},
		{
			name: "human fr-FR Paris",
			date: paris,
			app:  Appearance{Locale: "fr-FR", DateStyle: DateStyleHuman},
			want: "15 janvier 2024 à 15:30 CET",
		},
		{
			name: "human en-US Paris",
			date: paris,
			app:  Appearance{Locale: "en-US", DateStyle: DateStyleHuman},
			want: "January 15, 2024 at 3:30 PM CET",
		},
		{
			name: "human fr-FR Paris date only",
			date: paris,
			app:  Appearance{Locale: "fr-FR", DateStyle: DateStyleHuman, DateOmitTime: true},
			want: "15 janvier 2024 CET",
		},
		{
			name: "long en-US Paris date only",
			date: paris,
			app:  Appearance{Locale: "en-US", DateStyle: DateStyleLong, DateOmitTime: true},
			want: "January 15, 2024 CET",
		},
		{
			name: "human en-US Paris date only",
			date: paris,
			app:  Appearance{Locale: "en-US", DateStyle: DateStyleHuman, DateOmitTime: true},
			want: "January 15, 2024 CET",
		},
		{
			name: "DateOmitTime ignored for numeric",
			date: paris,
			app:  Appearance{Locale: "fr-FR", DateStyle: DateStyleNumeric, DateOmitTime: true},
			want: "15/01/2024 15:30 CET",
		},
		{
			name: "UTC to New York numeric",
			date: ny,
			app:  Appearance{Locale: "en-US", DateStyle: DateStyleNumeric},
			want: "01/15/2024 09:30 EST",
		},
		{
			name: "DateFormat overrides DateStyle",
			date: paris,
			app: Appearance{
				DateFormat: "02.01.2006 15:04",
				DateStyle:  DateStyleHuman,
				Locale:     "fr-FR",
			},
			want: "15.01.2024 15:30 CET",
		},
		{
			name: "empty DateStyle uses numeric default",
			date: time.Date(2024, 1, 15, 14, 30, 0, 0, time.UTC),
			app:  Appearance{},
			want: "01/15/2024 14:30 UTC",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := formatFillableDate(tt.date, tt.app)
			if err != nil {
				t.Fatalf("formatFillableDate error: %v", err)
			}
			if got != tt.want {
				t.Errorf("formatFillableDate() = %q, want %q", got, tt.want)
			}
		})
	}
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

func TestResolveDateOnlyLayout(t *testing.T) {
	tests := []struct {
		locale string
		want   string
	}{
		{"en-US", "01/02/2006"},
		{"fr-FR", "02/01/2006"},
		{"de-DE", "02.01.2006"},
		{"", "01/02/2006"},
	}
	for _, tt := range tests {
		t.Run(tt.locale, func(t *testing.T) {
			got := resolveDateOnlyLayout(tt.locale)
			if got != tt.want {
				t.Errorf("resolveDateOnlyLayout(%q) = %q, want %q", tt.locale, got, tt.want)
			}
		})
	}
}
