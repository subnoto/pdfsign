package sign

import (
	"fmt"
	"strings"
	"time"
)

// localeToDateLayout maps BCP 47-style locale tags to Go time layouts for date+time (24h).
// Used when Appearance.DateFormat is empty and Appearance.Locale is set.
var localeToDateLayout = map[string]string{
	"en-US": "01/02/2006 15:04",
	"en_US": "01/02/2006 15:04",
	"fr-FR": "02/01/2006 15:04",
	"fr_FR": "02/01/2006 15:04",
	"de-DE": "02.01.2006 15:04",
	"de_DE": "02.01.2006 15:04",
	"en-GB": "02/01/2006 15:04",
	"en_GB": "02/01/2006 15:04",
	"es-ES": "02/01/2006 15:04",
	"es_ES": "02/01/2006 15:04",
	"it-IT": "02/01/2006 15:04",
	"it_IT": "02/01/2006 15:04",
}

// resolveDateLayout returns the effective Go time layout for the date+time part.
// If DateFormat is non-empty it is used; else if Locale is set a predefined layout is used;
// otherwise default US layout is returned.
func resolveDateLayout(dateFormat, locale string) string {
	if strings.TrimSpace(dateFormat) != "" {
		return dateFormat
	}
	if strings.TrimSpace(locale) != "" {
		norm := strings.ReplaceAll(locale, "_", "-")
		if layout, ok := localeToDateLayout[norm]; ok {
			return layout
		}
		// try as-is for locale map (with underscore)
		if layout, ok := localeToDateLayout[locale]; ok {
			return layout
		}
	}
	return "01/02/2006 15:04"
}

// formatDateString formats a time.Time using the given Go layout for date+time and appends timezone.
// Layout uses reference time Mon Jan 2 15:04:05 MST 2006 (e.g. "01/02/2006 15:04" or "02.01.2006 15:04").
func formatDateString(date time.Time, layout string) string {
	dateTimePart := date.Format(layout)

	_, offset := date.Zone()
	var timezonePart string
	if offset == 0 {
		timezonePart = "GMT"
	} else if offset < 0 {
		offsetHours := offset / 3600
		offsetMinutes := (offset % 3600) / 60
		if offsetMinutes < 0 {
			offsetMinutes = -offsetMinutes
		}
		timezonePart = fmt.Sprintf("-%02d:%02d", -offsetHours, offsetMinutes)
	} else {
		offsetHours := offset / 3600
		offsetMinutes := (offset % 3600) / 60
		timezonePart = fmt.Sprintf("+%02d:%02d", offsetHours, offsetMinutes)
	}

	return fmt.Sprintf("%s %s", dateTimePart, timezonePart)
}

// dateFieldFontScale is the multiplier for font size when rendering date fields (slightly larger).
const dateFieldFontScale = 1.2

// fillDateFields will search the AcroForm Fields array for fields with names
// matching the pattern `date_id_${id}_signer_${signer_uid}` and,
// when the signer_uid matches the configured Appearance.SignerUID, replace the
// field value (/V) with the signature time formatted as a PDF date string.
// The fields are made read-only after filling. Date layout is taken from
// Appearance.DateFormat or Appearance.Locale when set.
// Using date_id allows multiple date fields per page.
func (context *SignContext) fillDateFields() error {
	sigTime := context.SignData.Signature.Info.Date
	if sigTime.IsZero() {
		return nil
	}

	app := &context.SignData.Appearance
	layout := resolveDateLayout(app.DateFormat, app.Locale)
	pattern := `date_id_(\d+)_signer_(.+)`
	return context.fillFormFields(pattern, func() (string, error) {
		return formatDateString(sigTime, layout), nil
	}, true, dateFieldFontScale)
}
