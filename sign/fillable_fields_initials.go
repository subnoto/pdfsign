package sign

import (
	"strings"
	"unicode"
)

// fillInitialsFields will search the AcroForm Fields array for fields with names
// matching the pattern `initials_page_${pageIndex}_signer_${signer_uid}` and,
// when the signer_uid matches the configured Appearance.SignerUID, replace the
// field value (/V) with the initials computed from SignData.Signature.Info.Name.
// The fields are made read-only after filling.
func (context *SignContext) fillInitialsFields() error {
	name := context.SignData.Signature.Info.Name
	if name == "" {
		return nil
	}

	// compute initials (first rune of each name part, uppercased)
	parts := strings.Fields(name)
	var initialsRunes []rune
	for _, p := range parts {
		r := []rune(p)
		if len(r) > 0 {
			initialsRunes = append(initialsRunes, unicode.ToUpper(r[0]))
		}
	}
	initials := string(initialsRunes)

	pattern := `initials_page_(\d+)_signer_(.+)`
	return context.fillFormFields(pattern, func() (string, error) {
		return initials, nil
	}, true) // makeReadOnly = true
}

