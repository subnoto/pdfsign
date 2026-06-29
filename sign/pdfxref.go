package sign

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"io"
	"strconv"

	"github.com/digitorus/pdf"
)

type xrefEntry struct {
	ID         uint32
	Offset     int64
	Generation int
	Free       bool
}

const (
	objectFooter = "\nendobj\n"
)

func (context *SignContext) getNextObjectID() uint32 {
	if context.lastXrefID == 0 {
		lastXrefID, err := context.getLastObjectIDFromXref()
		if err != nil {
			return 0
		}
		context.lastXrefID = lastXrefID
	}

	objectID := context.lastXrefID + uint32(len(context.newXrefEntries)) + 1
	return objectID
}

func (context *SignContext) addObject(object []byte) (uint32, error) {
	if context.lastXrefID == 0 {
		lastXrefID, err := context.getLastObjectIDFromXref()
		if err != nil {
			return 0, fmt.Errorf("failed to get last object ID: %w", err)
		}
		context.lastXrefID = lastXrefID
	}

	objectID := context.lastXrefID + uint32(len(context.newXrefEntries)) + 1
	context.newXrefEntries = append(context.newXrefEntries, xrefEntry{
		ID:     objectID,
		Offset: int64(context.OutputBuffer.Buff.Len()) + 1,
	})

	err := context.writeObject(objectID, 0, object)
	if err != nil {
		return 0, fmt.Errorf("failed to write object: %w", err)
	}

	return objectID, nil
}

func (context *SignContext) updateObject(id uint32, object []byte) error {
	gen := context.latestObjectGeneration(id) + 1
	context.updatedXrefEntries = append(context.updatedXrefEntries, xrefEntry{
		ID:         id,
		Offset:     int64(context.OutputBuffer.Buff.Len()) + 1,
		Generation: gen,
	})

	err := context.writeObject(id, gen, object)
	if err != nil {
		return fmt.Errorf("failed to write object: %w", err)
	}

	return nil
}

func (context *SignContext) latestObjectGeneration(id uint32) int {
	maxGen := 0
	if context.PDFReader != nil {
		for _, entry := range context.PDFReader.Xref() {
			ptr := entry.Ptr()
			if ptr.GetID() == id && int(ptr.GetGen()) > maxGen {
				maxGen = int(ptr.GetGen())
			}
		}
	}

	// digitorus/pdf does not always merge generation numbers from incremental
	// xref updates for objects that already exist; scan the PDF bytes for the
	// highest "id gen obj" header.
	if context.OutputBuffer != nil && context.OutputBuffer.Buff.Len() > 0 {
		maxGen = max(maxGen, highestObjectGenerationInPDF(context.OutputBuffer.Buff.Bytes(), id))
	} else if context.InputFile != nil {
		cur, err := context.InputFile.Seek(0, io.SeekCurrent)
		if err == nil {
			if _, err := context.InputFile.Seek(0, io.SeekStart); err == nil {
				if data, err := io.ReadAll(context.InputFile); err == nil {
					maxGen = max(maxGen, highestObjectGenerationInPDF(data, id))
				}
				_, _ = context.InputFile.Seek(cur, io.SeekStart)
			}
		}
	}

	for _, entry := range context.updatedXrefEntries {
		if entry.ID == id && entry.Generation > maxGen {
			maxGen = entry.Generation
		}
	}

	return maxGen
}

func highestObjectGenerationInPDF(data []byte, id uint32) int {
	maxGen := 0
	prefix := []byte(fmt.Sprintf("\n%d ", id))
	searchFrom := 0
	for {
		idx := bytes.Index(data[searchFrom:], prefix)
		if idx < 0 {
			break
		}
		pos := searchFrom + idx + len(prefix)
		end := pos
		for end < len(data) && data[end] >= '0' && data[end] <= '9' {
			end++
		}
		if end > pos && end+4 <= len(data) && bytes.Equal(data[end:end+4], []byte(" obj")) {
			if gen, err := strconv.Atoi(string(data[pos:end])); err == nil && gen > maxGen {
				maxGen = gen
			}
		}
		searchFrom = pos + 1
	}
	return maxGen
}

func (context *SignContext) writeObject(id uint32, generation int, object []byte) error {
	// Write the object header
	if _, err := fmt.Fprintf(context.OutputBuffer, "\n%d %d obj\n", id, generation); err != nil {
		return fmt.Errorf("failed to write object header: %w", err)
	}

	// Write the object content
	object = bytes.TrimSpace(object)
	if _, err := context.OutputBuffer.Write(object); err != nil {
		return fmt.Errorf("failed to write object content: %w", err)
	}

	// Write the object footer
	if _, err := context.OutputBuffer.Write([]byte(objectFooter)); err != nil {
		return fmt.Errorf("failed to write object footer: %w", err)
	}

	return nil
}

// writeXref writes the cross-reference table or stream based on the PDF type.
func (context *SignContext) writeXref() error {
	if _, err := context.OutputBuffer.Write([]byte("\n")); err != nil {
		return fmt.Errorf("failed to write newline before xref: %w", err)
	}

	context.NewXrefStart = int64(context.OutputBuffer.Buff.Len())

	switch context.PDFReader.XrefInformation.Type {
	case "table":
		return context.writeIncrXrefTable()
	case "stream":
		// NewXrefStart is updated inside writeXrefStream after addObject,
		// which records the exact byte offset of the xref stream object.
		return context.writeXrefStream()
	default:
		return fmt.Errorf("unknown xref type: %s", context.PDFReader.XrefInformation.Type)
	}
}

// encryptPdfString encrypts a text string for the given object ID and returns it
// in PDF syntax. Without encryption: (escaped text). With encryption: <hex of encrypted>.
func (context *SignContext) encryptPdfString(objID uint32, text string) (string, error) {
	if context.encryption == nil {
		return pdfString(text), nil
	}
	encrypted, err := context.encryptStreamData(objID, []byte(text))
	if err != nil {
		return "", fmt.Errorf("encrypt pdf string for object %d: %w", objID, err)
	}
	return "<" + hex.EncodeToString(encrypted) + ">", nil
}

// encryptStreamData encrypts stream data for the given object ID if the PDF is encrypted.
// Returns plaintext unchanged if encryption is not active.
func (context *SignContext) encryptStreamData(objID uint32, data []byte) ([]byte, error) {
	if context.encryption == nil {
		return data, nil
	}
	return pdf.EncryptStream(
		context.encryption.Key,
		context.encryption.UseAES,
		context.encryption.EncVersion,
		objID, 0, data,
	)
}

// encryptStreamForNextObject encrypts stream data for the object that will be
// assigned by the next addObject call.
func (context *SignContext) encryptStreamForNextObject(data []byte) ([]byte, error) {
	if context.encryption == nil {
		return data, nil
	}
	nextID := context.getNextObjectID()
	return context.encryptStreamData(nextID, data)
}

func (context *SignContext) getLastObjectIDFromXref() (uint32, error) {
	xref := context.PDFReader.Xref()
	if len(xref) == 0 {
		return 0, fmt.Errorf("no xref entries found")
	}

	// Find highest used object ID
	var maxID uint32
	for _, entry := range xref {
		ptr := entry.Ptr()

		// TODO: Check if in use (&& entry.offset != 0)
		if ptr.GetID() > maxID {
			maxID = ptr.GetID()
		}
	}

	return maxID + 1, nil
}
