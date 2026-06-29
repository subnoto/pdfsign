package sign

import (
	"fmt"
	"strings"
	"time"
)

// localeToDateLayout maps BCP 47-style locale tags to Go time layouts for date+time (24h).
// Used when Appearance.DateFormat is empty and numeric style is selected.
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

var monthNames = map[string][12]string{
	"en": {"January", "February", "March", "April", "May", "June", "July", "August", "September", "October", "November", "December"},
	"fr": {"janvier", "février", "mars", "avril", "mai", "juin", "juillet", "août", "septembre", "octobre", "novembre", "décembre"},
}

func applyTimezone(t time.Time, tz string) (time.Time, error) {
	if strings.TrimSpace(tz) == "" {
		return t, nil
	}
	loc, err := time.LoadLocation(tz)
	if err != nil {
		return t, fmt.Errorf("invalid timezone %q: %w", tz, err)
	}
	return t.In(loc), nil
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
		if layout, ok := localeToDateLayout[locale]; ok {
			return layout
		}
	}
	return "01/02/2006 15:04"
}

func resolveDateOnlyLayout(locale string) string {
	layout := resolveDateLayout("", locale)
	if idx := strings.LastIndex(layout, " "); idx >= 0 {
		return layout[:idx]
	}
	return layout
}

func normalizeDateStyle(style string) string {
	s := strings.TrimSpace(strings.ToLower(style))
	if s == "" {
		return DateStyleNumeric
	}
	return s
}

func extractLanguageCode(locale string) string {
	locale = strings.TrimSpace(locale)
	if locale == "" {
		return "en"
	}
	norm := strings.ReplaceAll(locale, "_", "-")
	if idx := strings.Index(norm, "-"); idx >= 0 {
		return strings.ToLower(norm[:idx])
	}
	return strings.ToLower(norm)
}

func localizedMonthNames(locale string) [12]string {
	lang := extractLanguageCode(locale)
	if names, ok := monthNames[lang]; ok {
		return names
	}
	return monthNames["en"]
}

func formatTime12h(hour, minute int) string {
	ampm := "AM"
	h := hour
	switch {
	case h == 0:
		h = 12
	case h == 12:
		ampm = "PM"
	case h > 12:
		h -= 12
		ampm = "PM"
	}
	return fmt.Sprintf("%d:%02d %s", h, minute, ampm)
}

func formatLongDate(date time.Time, locale string, includeTime bool) string {
	names := localizedMonthNames(locale)
	lang := extractLanguageCode(locale)
	month := names[int(date.Month())-1]

	switch lang {
	case "fr":
		if !includeTime {
			return fmt.Sprintf("%d %s %d", date.Day(), month, date.Year())
		}
		return fmt.Sprintf("%d %s %d, %02d:%02d", date.Day(), month, date.Year(), date.Hour(), date.Minute())
	default:
		if !includeTime {
			return fmt.Sprintf("%s %d, %d", month, date.Day(), date.Year())
		}
		return fmt.Sprintf("%s %d, %d, %s", month, date.Day(), date.Year(), formatTime12h(date.Hour(), date.Minute()))
	}
}

func formatHumanDate(date time.Time, locale string, includeTime bool) string {
	names := localizedMonthNames(locale)
	lang := extractLanguageCode(locale)
	month := names[int(date.Month())-1]

	switch lang {
	case "fr":
		if !includeTime {
			return fmt.Sprintf("%d %s %d", date.Day(), month, date.Year())
		}
		return fmt.Sprintf("%d %s %d à %02d:%02d", date.Day(), month, date.Year(), date.Hour(), date.Minute())
	default:
		if !includeTime {
			return fmt.Sprintf("%s %d, %d", month, date.Day(), date.Year())
		}
		return fmt.Sprintf("%s %d, %d at %s", month, date.Day(), date.Year(), formatTime12h(date.Hour(), date.Minute()))
	}
}

func formatDatePart(date time.Time, app Appearance) (string, error) {
	if strings.TrimSpace(app.DateFormat) != "" {
		return date.Format(app.DateFormat), nil
	}

	includeTime := !app.DateOmitTime

	switch normalizeDateStyle(app.DateStyle) {
	case DateStyleDateOnly:
		return date.Format(resolveDateOnlyLayout(app.Locale)), nil
	case DateStyleLong:
		return formatLongDate(date, app.Locale, includeTime), nil
	case DateStyleHuman:
		return formatHumanDate(date, app.Locale, includeTime), nil
	default:
		return date.Format(resolveDateLayout("", app.Locale)), nil
	}
}

func formatNumericOffset(offset int) string {
	if offset == 0 {
		return "UTC"
	}
	if offset < 0 {
		offsetHours := offset / 3600
		offsetMinutes := (offset % 3600) / 60
		if offsetMinutes < 0 {
			offsetMinutes = -offsetMinutes
		}
		return fmt.Sprintf("-%02d:%02d", -offsetHours, offsetMinutes)
	}
	offsetHours := offset / 3600
	offsetMinutes := (offset % 3600) / 60
	return fmt.Sprintf("+%02d:%02d", offsetHours, offsetMinutes)
}

func isUsableTimezoneAbbrev(name string) bool {
	if name == "" {
		return false
	}
	if name[0] == '+' || name[0] == '-' {
		return false
	}
	for _, c := range name {
		if c >= '0' && c <= '9' {
			return false
		}
	}
	return true
}

func formatTimezoneSuffix(date time.Time) string {
	name, offset := date.Zone()
	if offset == 0 {
		return "UTC"
	}
	if isUsableTimezoneAbbrev(name) {
		return name
	}
	return formatNumericOffset(offset)
}

func formatFillableDate(date time.Time, app Appearance) (string, error) {
	datePart, err := formatDatePart(date, app)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s %s", datePart, formatTimezoneSuffix(date)), nil
}

// formatDateString formats a time.Time using the given Go layout for date+time and appends timezone.
// Layout uses reference time Mon Jan 2 15:04:05 MST 2006 (e.g. "01/02/2006 15:04" or "02.01.2006 15:04").
func formatDateString(date time.Time, layout string) string {
	return fmt.Sprintf("%s %s", date.Format(layout), formatTimezoneSuffix(date))
}

// dateFieldFontScale is the multiplier for font size when rendering date fields (slightly larger).
const dateFieldFontScale = 1.2

// fillDateFields will search the AcroForm Fields array for fields with names
// matching the pattern `date_id_${id}_signer_${signer_uid}` and,
// when the signer_uid matches the configured Appearance.SignerUID, replace the
// field value (/V) with the signature time formatted as a display date string.
// The fields are made read-only after filling. Format is taken from
// Appearance.DateFormat, DateStyle, Locale, and Timezone when set.
// Using date_id allows multiple date fields per page.
func (context *SignContext) fillDateFields() error {
	sigTime := context.SignData.Signature.Info.Date
	if sigTime.IsZero() {
		return nil
	}

	app := &context.SignData.Appearance
	localized, err := applyTimezone(sigTime, app.Timezone)
	if err != nil {
		return err
	}

	pattern := `date_id_(\d+)_signer_(.+)`
	return context.fillFormFields(pattern, func() (string, error) {
		return formatFillableDate(localized, *app)
	}, true, dateFieldFontScale)
}
