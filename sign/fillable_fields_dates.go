package sign

import (
	"fmt"
	"time"
)

// formatDateString formats a time.Time as a PDF date string without PDF string wrapping
func formatDateString(date time.Time) string {
	// Calculate timezone offset from GMT.
	_, original_offset := date.Zone()
	offset := original_offset
	if offset < 0 {
		offset = -offset
	}

	offset_duration := time.Duration(offset) * time.Second
	offset_hours := int(offset_duration.Hours())
	offset_minutes := int(offset_duration.Minutes())
	offset_minutes = offset_minutes - (offset_hours * 60)

	dateString := "D:" + date.Format("20060102150405")

	// Do some special formatting as the PDF timezone format isn't supported by Go.
	if original_offset < 0 {
		dateString += "-"
	} else {
		dateString += "+"
	}

	offset_hours_formatted := fmt.Sprintf("%d", offset_hours)
	offset_minutes_formatted := fmt.Sprintf("%d", offset_minutes)
	// Left pad to ensure 2 digits
	if len(offset_hours_formatted) < 2 {
		offset_hours_formatted = "0" + offset_hours_formatted
	}
	if len(offset_minutes_formatted) < 2 {
		offset_minutes_formatted = "0" + offset_minutes_formatted
	}
	dateString += offset_hours_formatted + "'" + offset_minutes_formatted + "'"

	return dateString
}

// fillDateFields will search the AcroForm Fields array for fields with names
// matching the pattern `date_id_${id}_signer_${signer_uid}` and,
// when the signer_uid matches the configured Appearance.SignerUID, replace the
// field value (/V) with the signature time formatted as a PDF date string.
// The fields are made read-only after filling.
// Using date_id allows multiple date fields per page.
func (context *SignContext) fillDateFields() error {
	sigTime := context.SignData.Signature.Info.Date
	if sigTime.IsZero() {
		return nil
	}

	pattern := `date_id_(\d+)_signer_(.+)`
	return context.fillFormFields(pattern, func() (string, error) {
		return formatDateString(sigTime), nil
	}, true) // makeReadOnly = true
}

