package sign

import (
	"crypto"
	"crypto/x509"
	"io"
	"time"

	"github.com/digitorus/pdf"
	"github.com/digitorus/timestamp"
	"github.com/mattetti/filebuffer"
	"github.com/subnoto/pdfsign/revocation"
)

type CatalogData struct {
	ObjectId   uint32
	RootString string
}

type TSA struct {
	URL      string
	Username string
	Password string
}

type RevocationFunction func(cert, issuer *x509.Certificate, i *revocation.InfoArchival) error

type SignData struct {
	Signature          SignDataSignature
	Signer             crypto.Signer
	DigestAlgorithm    crypto.Hash
	Certificate        *x509.Certificate
	CertificateChains  [][]*x509.Certificate
	TSA                TSA
	RevocationData     revocation.InfoArchival
	RevocationFunction RevocationFunction
	Appearance         Appearance

	objectId uint32
}

// Appearance represents the appearance of the signature
type Appearance struct {
	Visible bool

	Page        uint32
	LowerLeftX  float64
	LowerLeftY  float64
	UpperRightX float64
	UpperRightY float64

	Image            []byte // Image data to use as signature appearance
	ImageAsWatermark bool   // If true, the text will be drawn over the image
	// SignerUID, when set, will cause the signer initials to be filled into
	// AcroForm fields matching the pattern:
	//   initials_page_${pageIndex}_signer_${signer_uid}
	// The initials are derived from SignData.Signature.Info.Name.
	SignerUID string

	// Timezone is an IANA location name (e.g. "Europe/Paris", "America/New_York").
	// When set, the signature date is converted to this zone before formatting.
	// Invalid values cause fillDateFields to return an error.
	Timezone string
	// DateFormat is the Go time layout for the date+time part of filled date fields
	// (reference time: Mon Jan 2 15:04:05 MST 2006). When non-empty, used for date_id_* fields;
	// timezone is still appended. When empty, DateStyle and Locale are used.
	DateFormat string
	// DateStyle selects a preset format when DateFormat is empty.
	// Supported: DateStyleNumeric (default), DateStyleDateOnly, DateStyleLong, DateStyleHuman.
	DateStyle string
	// DateOmitTime, when true, causes long and human styles to omit the time portion
	// (e.g. "15 janvier 2024" instead of "15 janvier 2024 à 14:30"). Has no effect on
	// numeric or date-only styles.
	DateOmitTime bool
	// Locale is a BCP 47-style tag (e.g. "en-US", "fr-FR", "de-DE"). Used when DateFormat
	// is empty to pick locale-specific layouts and localized month names for long/human styles.
	Locale string
}

const (
	DateStyleNumeric  = "numeric"
	DateStyleDateOnly = "date-only"
	DateStyleLong     = "long"
	DateStyleHuman    = "human"
)

type VisualSignData struct {
	pageObjectId uint32
	objectId     uint32
}

type InfoData struct {
	ObjectId uint32
}

//go:generate stringer -type=CertType
type CertType uint

const (
	CertificationSignature CertType = iota + 1
	ApprovalSignature
	UsageRightsSignature
	TimeStampSignature
)

//go:generate stringer -type=DocMDPPerm
type DocMDPPerm uint

const (
	DoNotAllowAnyChangesPerms DocMDPPerm = iota + 1
	AllowFillingExistingFormFieldsAndSignaturesPerms
	AllowFillingExistingFormFieldsAndSignaturesAndCRUDAnnotationsPerms
)

type SignDataSignature struct {
	CertType   CertType
	DocMDPPerm DocMDPPerm
	Info       SignDataSignatureInfo
}

type SignDataSignatureInfo struct {
	Name        string
	Location    string
	Reason      string
	ContactInfo string
	Date        time.Time
}

// Remove the duplicated SignatureInfo and SignatureValidation types
// They are now available in the common package

type SignContext struct {
	InputFile              io.ReadSeeker
	OutputFile             io.Writer
	OutputBuffer           *filebuffer.Buffer
	SignData               SignData
	CatalogData            CatalogData
	VisualSignData         VisualSignData
	InfoData               InfoData
	PDFReader              *pdf.Reader
	NewXrefStart           int64
	ByteRangeValues        []int64
	SignatureMaxLength     uint32
	SignatureMaxLengthBase uint32

	existingSignatures []SignData
	lastXrefID         uint32
	newXrefEntries     []xrefEntry
	updatedXrefEntries []xrefEntry

	// Computed signature information
	computedDocumentHash  string
	computedSignatureHash string
	computedTimeStamp     *timestamp.Timestamp
}
