package sign

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf16"

	"github.com/digitorus/pdf"
)

// normalizeDA attempts to convert a possibly multi-line DA string like:
//
//	"0 0 0 rg\n/Helvetica 10 Tf"
//
// into a single-line, font-first form:
//
//	"/Helvetica 10 Tf 0 0 0 rg"
//
// normalizeDA ensures DA is a single-line string and forces black color.
// It preserves an existing font size if present, otherwise defaults to 10.
func normalizeDA(raw string) string {
	// collapse newlines and extra spaces
	s := strings.ReplaceAll(raw, "\r", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	parts := strings.Fields(s)
	// default size
	size := "10"
	if len(parts) > 0 {
		// try to find a size token before Tf, e.g. '/Helvetica 10 Tf' or '10 Tf'
		re := regexp.MustCompile(`([0-9]+(?:\.[0-9]+)?)\s*Tf`)
		if m := re.FindStringSubmatch(s); len(m) >= 2 {
			size = m[1]
		}
	}
	// Use resource font /F1 (must be present in Resources) and force black color
	return fmt.Sprintf("/F1 %s Tf 0 0 0 rg", size)
}

// createTextFieldAppearance creates an appearance stream for a text field.
// fontScale is an optional multiplier for fontSize (e.g. 1.2 for date fields); 0 means no scaling.
func (context *SignContext) createTextFieldAppearance(text string, rect [4]float64, da string, fontScale float64) ([]byte, error) {
	width := rect[2] - rect[0]
	height := rect[3] - rect[1]

	if width <= 0 || height <= 0 {
		return nil, fmt.Errorf("invalid rectangle dimensions")
	}

	// Extract font size from DA string
	fontSize := 10.0
	re := regexp.MustCompile(`([0-9]+(?:\.[0-9]+)?)\s*Tf`)
	if m := re.FindStringSubmatch(da); len(m) >= 2 {
		if size, err := strconv.ParseFloat(m[1], 64); err == nil {
			fontSize = size
		}
	}

	// Adjust font size to fit the field height with some padding
	maxFontSize := height * 0.7
	if fontSize > maxFontSize {
		fontSize = maxFontSize
	}
	if fontScale > 0 {
		fontSize *= fontScale
		if fontSize > maxFontSize {
			fontSize = maxFontSize
		}
	}

	// Better text width calculation (rough approximation for Helvetica)
	textWidth := float64(len(text)) * fontSize * 0.6

	// Center text horizontally and vertically
	textX := (width - textWidth) / 2
	if textX < 1 {
		textX = 1 // small left margin
	}

	// Center vertically: baseline should be positioned so text appears centered
	// For Helvetica, descender is about 0.2 * fontSize, ascender is about 0.7 * fontSize
	textY := (height-fontSize)/2 + fontSize*0.2

	// Create appearance stream
	var stream bytes.Buffer
	stream.WriteString("q\n") // Save graphics state

	// Draw white background
	stream.WriteString("1 1 1 rg\n")                                     // White fill color
	stream.WriteString(fmt.Sprintf("0 0 %.1f %.1f re\n", width, height)) // Rectangle covering entire field
	stream.WriteString("f\n")                                            // Fill rectangle

	// Draw text
	stream.WriteString("BT\n")                                      // Begin text
	stream.WriteString("/F1 ")                                      // Use font F1 (must be in Resources)
	stream.WriteString(fmt.Sprintf("%.1f", fontSize))               // Font size
	stream.WriteString(" Tf\n")                                     // Set font
	stream.WriteString("0 0 0 rg\n")                                // Black text color
	stream.WriteString(fmt.Sprintf("%.1f %.1f Td\n", textX, textY)) // Position
	stream.WriteString(pdfString(text))                             // Text content
	stream.WriteString(" Tj\n")                                     // Show text
	stream.WriteString("ET\n")                                      // End text
	stream.WriteString("Q\n")                                       // Restore graphics state

	// Create XObject dictionary
	var xobj bytes.Buffer
	xobj.WriteString("<<\n")
	xobj.WriteString("  /Type /XObject\n")
	xobj.WriteString("  /Subtype /Form\n")
	xobj.WriteString(fmt.Sprintf("  /BBox [0 0 %.1f %.1f]\n", width, height))
	xobj.WriteString("  /Resources <<\n")
	xobj.WriteString("    /Font <<\n")
	xobj.WriteString("      /F1 <<\n")
	xobj.WriteString("        /Type /Font\n")
	xobj.WriteString("        /Subtype /Type1\n")
	xobj.WriteString("        /BaseFont /Helvetica\n")
	xobj.WriteString("      >>\n")
	xobj.WriteString("    >>\n")
	xobj.WriteString("  >>\n")
	xobj.WriteString(fmt.Sprintf("  /Length %d\n", stream.Len()))
	xobj.WriteString(">>\n")
	xobj.WriteString("stream\n")
	xobj.Write(stream.Bytes())
	xobj.WriteString("\nendstream\n")

	return xobj.Bytes(), nil
}

// decodeFieldName decodes a field name that may be UTF-16 encoded with a BOM
func decodeFieldName(fieldName string) string {
	decodedFieldName := fieldName
	b := []byte(fieldName)
	if len(b) >= 2 {
		// BOM 0xFEFF = big endian, 0xFFFE = little endian
		if b[0] == 0xfe && b[1] == 0xff {
			// UTF-16 BE
			var u16s []uint16
			for i := 2; i+1 < len(b); i += 2 {
				u16s = append(u16s, uint16(b[i])<<8|uint16(b[i+1]))
			}
			decodedFieldName = string(utf16.Decode(u16s))
		} else if b[0] == 0xff && b[1] == 0xfe {
			// UTF-16 LE
			var u16s []uint16
			for i := 2; i+1 < len(b); i += 2 {
				u16s = append(u16s, uint16(b[i])|uint16(b[i+1])<<8)
			}
			decodedFieldName = string(utf16.Decode(u16s))
		}
	}
	return decodedFieldName
}

// matchFieldSigner matches a decoded field name against a pattern and compares the signer UID
func matchFieldSigner(decodedFieldName, pattern, uid string) (bool, string) {
	re := regexp.MustCompile(pattern)
	matches := re.FindStringSubmatch(decodedFieldName)
	var fieldSigner string
	if len(matches) >= 3 {
		fieldSigner = matches[2]
	} else {
		// Only use fallback if the pattern prefix matches (e.g., "initials_page_" or "date_id_")
		// Extract the prefix from the pattern (everything before the first capture group)
		prefixRe := regexp.MustCompile(`^([^\(]+)\(`)
		prefixMatches := prefixRe.FindStringSubmatch(pattern)
		if len(prefixMatches) >= 2 {
			patternPrefix := prefixMatches[1]
			// Check if field name starts with the pattern prefix
			if !strings.HasPrefix(decodedFieldName, patternPrefix) {
				return false, ""
			}
		}

		// Fallback: try to find 'signer_' in the field name and extract a hex-like tail.
		// Only use this if the pattern prefix matched above
		if idx := strings.Index(decodedFieldName, "signer_"); idx >= 0 {
			tail := decodedFieldName[idx+len("signer_"):]
			// Extract hex substring from tail
			hexRe := regexp.MustCompile(`[0-9a-fA-F]+`)
			hs := hexRe.FindString(tail)
			if hs == "" {
				return false, ""
			}
			fieldSigner = hs
		} else {
			return false, ""
		}
	}

	// Compare configured uid with the field signer. The fieldSigner may
	// be hex-encoded; accept either exact match, hex(uid), or hex-decoded match.
	if fieldSigner == uid {
		return true, fieldSigner
	}
	if hex.EncodeToString([]byte(uid)) == fieldSigner {
		return true, fieldSigner
	}
	// try decoding fieldSigner as hex
	if bs, err := hex.DecodeString(fieldSigner); err == nil {
		if string(bs) == uid {
			return true, fieldSigner
		}
	}
	return false, fieldSigner
}

// getFieldRect extracts rectangle coordinates from a field or its first kid widget
func getFieldRect(field pdf.Value) [4]float64 {
	rect := [4]float64{0, 0, 100, 20} // default size
	kids := field.Key("Kids")
	if !kids.IsNull() && kids.Len() > 0 {
		firstKid := kids.Index(0)
		if rectVal := firstKid.Key("Rect"); !rectVal.IsNull() && rectVal.Kind() == pdf.Array && rectVal.Len() >= 4 {
			rect[0] = rectVal.Index(0).Float64()
			rect[1] = rectVal.Index(1).Float64()
			rect[2] = rectVal.Index(2).Float64()
			rect[3] = rectVal.Index(3).Float64()
		}
	}
	return rect
}

// updateFieldObject updates a field object with a new value and optionally makes it read-only.
// appearanceFontScale is applied when building the appearance stream (0 = no scaling).
func (context *SignContext) updateFieldObject(field pdf.Value, value string, makeReadOnly bool, appearanceFontScale float64) error {
	ptr := field.GetPtr()
	if ptr.GetID() == 0 {
		// Direct object, skip parent update
	} else {
		// Build a new dictionary preserving existing keys except /V which we replace
		var buf bytes.Buffer
		buf.WriteString("<<\n")

		existingFf := int64(0)
		hasFf := false

		for _, key := range field.Keys() {
			if key == "V" {
				continue
			}
			if key == "Ff" {
				// Preserve existing field flags
				existingFf = field.Key("Ff").Int64()
				hasFf = true
				continue // Will add it back later with read-only flag if needed
			}
			// skip appearance streams to force viewers to regenerate them
			if key == "AP" {
				// Generate new appearance stream for this field
				rect := getFieldRect(field)
				da := normalizeDA(field.Key("DA").RawString())
				appearance, err := context.createTextFieldAppearance(value, rect, da, appearanceFontScale)
				if err == nil {
					apObjectId, err := context.addObject(appearance)
					if err == nil {
						buf.WriteString(fmt.Sprintf("/AP << /N %d 0 R >>\n", apObjectId))
					}
				}
				continue
			}
			buf.WriteString(" /")
			buf.WriteString(key)
			buf.WriteString(" ")

			// If this is the field name (/T) and it's encoded as UTF-16 with a BOM,
			// decode it and write as a normal PDF string so Acrobat can use it.
			switch key {
			case "T":
				tVal := field.Key("T").RawString()
				asciiT := decodeFieldName(tVal)
				buf.WriteString(pdfString(asciiT))
			case "DA":
				// normalize appearance default string
				daVal := field.Key("DA").RawString()
				buf.WriteString(pdfString(normalizeDA(daVal)))
			default:
				context.serializeCatalogEntry(&buf, ptr.GetID(), field.Key(key))
			}
			buf.WriteString("\n")
		}

		// Set field flags with read-only if requested
		if makeReadOnly {
			newFf := existingFf | 2 // Set bit 1 (ReadOnly)
			buf.WriteString(fmt.Sprintf(" /Ff %d\n", newFf))
		} else if hasFf {
			// Preserve existing Ff if not making read-only
			buf.WriteString(fmt.Sprintf(" /Ff %d\n", existingFf))
		}

		// Set new value
		buf.WriteString(" /V ")
		buf.WriteString(pdfString(value))
		buf.WriteString("\n")
		// Set appearance state to match value for proper rendering
		buf.WriteString(" /AS ")
		buf.WriteString(pdfString(value))
		buf.WriteString("\n")
		buf.WriteString(">>\n")

		if err := context.updateObject(uint32(ptr.GetID()), buf.Bytes()); err != nil {
			return fmt.Errorf("failed to update field object %d: %w", ptr.GetID(), err)
		}
	}

	// Also try to update any Kids (widget annotations) so visible widget values
	// reflect the new value. Kids can be indirect references and should be
	// updated even when the parent field is a direct object.
	kids := field.Key("Kids")
	if !kids.IsNull() {
		for k := 0; k < kids.Len(); k++ {
			kid := kids.Index(k)
			kptr := kid.GetPtr()
			if kptr.GetID() == 0 {
				continue
			}

			var kbuf bytes.Buffer
			kbuf.WriteString("<<\n")

			existingKidFf := int64(0)
			hasKidFf := false

			for _, kkey := range kid.Keys() {
				if kkey == "V" {
					continue
				}
				if kkey == "Ff" {
					existingKidFf = kid.Key("Ff").Int64()
					hasKidFf = true
					continue
				}
				if kkey == "AP" {
					// Generate new appearance stream for this widget
					var rect [4]float64
					if rectVal := kid.Key("Rect"); !rectVal.IsNull() && rectVal.Kind() == pdf.Array && rectVal.Len() >= 4 {
						rect[0] = rectVal.Index(0).Float64()
						rect[1] = rectVal.Index(1).Float64()
						rect[2] = rectVal.Index(2).Float64()
						rect[3] = rectVal.Index(3).Float64()
					} else {
						rect = [4]float64{0, 0, 100, 20} // default size
					}
					da := normalizeDA(kid.Key("DA").RawString())
					appearance, err := context.createTextFieldAppearance(value, rect, da, appearanceFontScale)
					if err == nil {
						apObjectId, err := context.addObject(appearance)
						if err == nil {
							kbuf.WriteString(fmt.Sprintf(" /AP << /N %d 0 R >>\n", apObjectId))
						}
					}
					continue
				}
				kbuf.WriteString(" /")
				kbuf.WriteString(kkey)
				kbuf.WriteString(" ")

				// Handle widget /T similarly: decode UTF-16 BOM if present
				switch kkey {
				case "T":
					tVal := kid.Key("T").RawString()
					asciiT := decodeFieldName(tVal)
					kbuf.WriteString(pdfString(asciiT))
				case "DA":
					daVal := kid.Key("DA").RawString()
					kbuf.WriteString(pdfString(normalizeDA(daVal)))
				default:
					context.serializeCatalogEntry(&kbuf, kptr.GetID(), kid.Key(kkey))
				}
				kbuf.WriteString("\n")
			}

			// Set field flags with read-only if requested
			if makeReadOnly {
				newKidFf := existingKidFf | 2 // Set bit 1 (ReadOnly)
				kbuf.WriteString(fmt.Sprintf(" /Ff %d\n", newKidFf))
			} else if hasKidFf {
				kbuf.WriteString(fmt.Sprintf(" /Ff %d\n", existingKidFf))
			}

			kbuf.WriteString(" /V ")
			kbuf.WriteString(pdfString(value))
			kbuf.WriteString("\n")
			// Set appearance state to match value for proper rendering
			kbuf.WriteString(" /AS ")
			kbuf.WriteString(pdfString(value))
			kbuf.WriteString("\n")
			kbuf.WriteString(">>\n")

			if err := context.updateObject(uint32(kptr.GetID()), kbuf.Bytes()); err != nil {
				return fmt.Errorf("failed to update kid object %d: %w", kptr.GetID(), err)
			}
		}
	}

	return nil
}

// fillFormFields is a generic function that fills form fields matching a pattern with a value.
// appearanceFontScale is applied when building field appearance (0 = no scaling, e.g. 1.2 for date fields).
func (context *SignContext) fillFormFields(pattern string, getValue func() (string, error), makeReadOnly bool, appearanceFontScale float64) error {
	uid := context.SignData.Appearance.SignerUID
	if uid == "" {
		return nil
	}

	value, err := getValue()
	if err != nil {
		return err
	}
	if value == "" {
		return nil
	}

	acroForm := context.PDFReader.Trailer().Key("Root").Key("AcroForm")
	if acroForm.IsNull() {
		return nil
	}

	fields := acroForm.Key("Fields")
	if fields.IsNull() {
		return nil
	}

	for i := 0; i < fields.Len(); i++ {
		field := fields.Index(i)
		t := field.Key("T")
		if t.IsNull() {
			continue
		}

		fieldName := t.RawString()
		decodedFieldName := decodeFieldName(fieldName)

		matched, _ := matchFieldSigner(decodedFieldName, pattern, uid)
		if !matched {
			continue
		}

		if err := context.updateFieldObject(field, value, makeReadOnly, appearanceFontScale); err != nil {
			return err
		}
	}

	return nil
}
