package sign

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode"
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

// createTextFieldAppearance creates an appearance stream for a text field
func (context *SignContext) createTextFieldAppearance(text string, rect [4]float64, da string) ([]byte, error) {
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

// fillInitialsFields will search the AcroForm Fields array for fields with names
// matching the pattern `initials_page_${pageIndex}_signer_${signer_uid}` and,
// when the signer_uid matches the configured Appearance.SignerUID, replace the
// field value (/V) with the initials computed from SignData.Signature.Info.Name.
func (context *SignContext) fillInitialsFields() error {
	uid := context.SignData.Appearance.SignerUID
	if uid == "" {
		return nil
	}

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

	acroForm := context.PDFReader.Trailer().Key("Root").Key("AcroForm")
	if acroForm.IsNull() {
		return nil
	}

	fields := acroForm.Key("Fields")
	if fields.IsNull() {
		return nil
	}

	// Match the pattern anywhere in the field name to tolerate BOM or encoding prefixes
	pattern := `initials_page_(\d+)_signer_(.+)`
	re := regexp.MustCompile(pattern)

	for i := 0; i < fields.Len(); i++ {
		field := fields.Index(i)
		t := field.Key("T")
		if t.IsNull() {
			continue
		}

		fieldName := t.RawString()
		// If the field name is UTF-16 with a BOM, decode it to UTF-8 for regex matching.
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
		matches := re.FindStringSubmatch(decodedFieldName)
		var fieldSigner string
		if len(matches) >= 3 {
			fieldSigner = matches[2]
		} else {
			// Fallback: try to find 'signer_' in the field name and extract a hex-like tail.
			if idx := strings.Index(decodedFieldName, "signer_"); idx >= 0 {
				tail := decodedFieldName[idx+len("signer_"):]
				// Extract hex substring from tail
				hexRe := regexp.MustCompile(`[0-9a-fA-F]+`)
				hs := hexRe.FindString(tail)
				if hs == "" {
					continue
				}
				fieldSigner = hs
			} else {
				continue
			}
		}

		// Compare configured uid with the field signer. The fieldSigner may
		// be hex-encoded; accept either exact match, hex(uid), or hex-decoded match.
		matched := false
		if fieldSigner == uid {
			matched = true
		} else if hex.EncodeToString([]byte(uid)) == fieldSigner {
			matched = true
		} else {
			// try decoding fieldSigner as hex
			if bs, err := hex.DecodeString(fieldSigner); err == nil {
				if string(bs) == uid {
					matched = true
				}
			}
		}

		if !matched {
			continue
		}

		// Attempt to update the parent field object if it's indirect.
		ptr := field.GetPtr()
		if ptr.GetID() == 0 {
		} else {

			// Build a new dictionary preserving existing keys except /V which we replace
			var buf bytes.Buffer
			buf.WriteString("<<\n")
			for _, key := range field.Keys() {
				if key == "V" {
					continue
				}
				// skip appearance streams to force viewers to regenerate them
				if key == "AP" {
					// Generate new appearance stream for this field
					// Try to get rect from first Kid widget
					rect := [4]float64{0, 0, 100, 20} // default size
					kids := field.Key("Kids")
					if !kids.IsNull() && kids.Len() > 0 {
						firstKid := kids.Index(0)
						rectVal := firstKid.Key("Rect")
						if !rectVal.IsNull() {
							// Safely check if it's an array
							var isArray bool
							func() {
								defer func() {
									if r := recover(); r != nil {
										isArray = false
									}
								}()
								isArray = rectVal.Kind() == pdf.Array && rectVal.Len() >= 4
							}()
							if isArray {
								rect[0] = rectVal.Index(0).Float64()
								rect[1] = rectVal.Index(1).Float64()
								rect[2] = rectVal.Index(2).Float64()
								rect[3] = rectVal.Index(3).Float64()
							}
						}
					}
					da := normalizeDA(field.Key("DA").RawString())
					appearance, err := context.createTextFieldAppearance(initials, rect, da)
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
					b := []byte(tVal)
					asciiT := tVal
					if len(b) >= 2 {
						if b[0] == 0xfe && b[1] == 0xff {
							// UTF-16 BE
							var u16s []uint16
							for i := 2; i+1 < len(b); i += 2 {
								u16s = append(u16s, uint16(b[i])<<8|uint16(b[i+1]))
							}
							asciiT = string(utf16.Decode(u16s))
						} else if b[0] == 0xff && b[1] == 0xfe {
							// UTF-16 LE
							var u16s []uint16
							for i := 2; i+1 < len(b); i += 2 {
								u16s = append(u16s, uint16(b[i])|uint16(b[i+1])<<8)
							}
							asciiT = string(utf16.Decode(u16s))
						}
					}
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

			// Set new value
			buf.WriteString(" /V ")
			buf.WriteString(pdfString(initials))
			buf.WriteString("\n")
			// Set appearance state to match value for proper rendering
			buf.WriteString(" /AS ")
			buf.WriteString(pdfString(initials))
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
				for _, kkey := range kid.Keys() {
					if kkey == "V" {
						continue
					}
					if kkey == "AP" {
						// Generate new appearance stream for this widget
						var rect [4]float64
						rectVal := kid.Key("Rect")
						if !rectVal.IsNull() {
							// Safely check if it's an array
							var isArray bool
							func() {
								defer func() {
									if r := recover(); r != nil {
										isArray = false
									}
								}()
								isArray = rectVal.Kind() == pdf.Array && rectVal.Len() >= 4
							}()
							if isArray {
								rect[0] = rectVal.Index(0).Float64()
								rect[1] = rectVal.Index(1).Float64()
								rect[2] = rectVal.Index(2).Float64()
								rect[3] = rectVal.Index(3).Float64()
							} else {
								rect = [4]float64{0, 0, 100, 20} // default size
							}
						} else {
							rect = [4]float64{0, 0, 100, 20} // default size
						}
						da := normalizeDA(kid.Key("DA").RawString())
						appearance, err := context.createTextFieldAppearance(initials, rect, da)
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
						b := []byte(tVal)
						asciiT := tVal
						if len(b) >= 2 {
							if b[0] == 0xfe && b[1] == 0xff {
								var u16s []uint16
								for i := 2; i+1 < len(b); i += 2 {
									u16s = append(u16s, uint16(b[i])<<8|uint16(b[i+1]))
								}
								asciiT = string(utf16.Decode(u16s))
							} else if b[0] == 0xff && b[1] == 0xfe {
								var u16s []uint16
								for i := 2; i+1 < len(b); i += 2 {
									u16s = append(u16s, uint16(b[i])|uint16(b[i+1])<<8)
								}
								asciiT = string(utf16.Decode(u16s))
							}
						}
						kbuf.WriteString(pdfString(asciiT))
					case "DA":
						daVal := kid.Key("DA").RawString()
						kbuf.WriteString(pdfString(normalizeDA(daVal)))
					default:
						context.serializeCatalogEntry(&kbuf, kptr.GetID(), kid.Key(kkey))
					}
					kbuf.WriteString("\n")
				}
				kbuf.WriteString(" /V ")
				kbuf.WriteString(pdfString(initials))
				kbuf.WriteString("\n")
				// Set appearance state to match value for proper rendering
				kbuf.WriteString(" /AS ")
				kbuf.WriteString(pdfString(initials))
				kbuf.WriteString("\n")
				kbuf.WriteString(">>\n")

				if err := context.updateObject(uint32(kptr.GetID()), kbuf.Bytes()); err != nil {
					return fmt.Errorf("failed to update kid object %d: %w", kptr.GetID(), err)
				}
			}
		}
	}

	return nil
}
