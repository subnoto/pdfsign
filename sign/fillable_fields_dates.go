package sign

import (
	"fmt"
	"time"
)

// formatDateString formats a time.Time as MM/DD/YYYY hh:mm + timezone
func formatDateString(date time.Time) string {
	// Format date as MM/DD/YYYY
	datePart := date.Format("01/02/2006")

	// Format time as hh:mm (24-hour format)
	timePart := date.Format("15:04")

	// Calculate timezone offset
	_, offset := date.Zone()

	// Handle timezone offset calculation properly for both positive and negative offsets
	var timezonePart string
	if offset < 0 {
		offsetHours := offset / 3600
		offsetMinutes := (offset % 3600) / 60
		// For negative offsets, modulo can be negative, so we need to handle it
		if offsetMinutes < 0 {
			offsetMinutes = -offsetMinutes
		}
		timezonePart = fmt.Sprintf("-%02d:%02d", -offsetHours, offsetMinutes)
	} else {
		offsetHours := offset / 3600
		offsetMinutes := (offset % 3600) / 60
		timezonePart = fmt.Sprintf("+%02d:%02d", offsetHours, offsetMinutes)
	}

	return fmt.Sprintf("%s %s %s", datePart, timePart, timezonePart)

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
