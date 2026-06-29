package sign

import (
	"bytes"
	"crypto/sha1"
	"crypto/x509"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/digitorus/pdf"
	"github.com/subnoto/pdfsign/common"
	"github.com/subnoto/pdfsign/revocation"
	"golang.org/x/crypto/ocsp"
)

// defaultHTTPTimeout limits how long signing waits on TSA/OCSP/CRL HTTP calls.
const defaultHTTPTimeout = 30 * time.Second

// RevocationHTTPClient uses a clone of DefaultTransport. On the js/wasm runtime
// this routes requests through the host's Fetch API (global.fetch), unlike the
// package-level http.Get/http.Post helpers which fall back to a (non-functional)
// native dialer.
var RevocationHTTPClient = &http.Client{
	Timeout:   defaultHTTPTimeout,
	Transport: &http.Transport{Proxy: http.ProxyFromEnvironment},
}

func embedOCSPRevocationStatus(cert, issuer *x509.Certificate, i *revocation.InfoArchival) error {
	req, err := ocsp.CreateRequest(cert, issuer, nil)
	if err != nil {
		return err
	}

	// Try each OCSP responder in turn. CAs (e.g. CertEurope) often list several
	// responders and the first is not always reachable. POST the DER request
	// (more robust than GET). Use the first responder that returns a valid one.
	var lastErr error
	for _, server := range cert.OCSPServer {
		resp, err := RevocationHTTPClient.Post(strings.TrimRight(server, "/"), "application/ocsp-request", bytes.NewReader(req))
		if err != nil {
			lastErr = err
			continue
		}
		body, err := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if err != nil {
			lastErr = err
			continue
		}
		if resp.StatusCode != http.StatusOK {
			lastErr = fmt.Errorf("OCSP request failed: %s", resp.Status)
			continue
		}
		if _, err := ocsp.ParseResponseForCert(body, cert, issuer); err != nil {
			lastErr = err
			continue
		}
		return i.AddOCSP(body)
	}
	if lastErr != nil {
		return lastErr
	}
	return fmt.Errorf("no OCSP responder available for certificate")
}

// embedCRLRevocationStatus requires an issuer as it needs to implement the
// the interface, a nil argment might be given if the issuer is not known.
func embedCRLRevocationStatus(cert, issuer *x509.Certificate, i *revocation.InfoArchival) error {
	if len(cert.CRLDistributionPoints) == 0 {
		return fmt.Errorf("no CRL distribution points on certificate")
	}
	resp, err := RevocationHTTPClient.Get(cert.CRLDistributionPoints[0])
	if err != nil {
		return err
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("CRL fetch failed: %s", resp.Status)
	}

	crl, err := x509.ParseRevocationList(body)
	if err != nil {
		return fmt.Errorf("parse CRL: %w", err)
	}
	if issuer != nil {
		if err := crl.CheckSignatureFrom(issuer); err != nil {
			return fmt.Errorf("CRL signature invalid: %w", err)
		}
	}

	return i.AddCRL(body)
}

func DefaultEmbedRevocationStatusFunction(cert, issuer *x509.Certificate, i *revocation.InfoArchival) error {
	// For each certificate a revoction status needs to be included, this can be done
	// by embedding a CRL or OCSP response. In most cases an OCSP response is smaller
	// to embed in the document but and empty CRL (often seen of dediced high volume
	// hirachies) can be smaller.
	//
	// There have been some reports that the usage of a CRL would result in a better
	// compatibility.
	//
	// TODO: Find and embed link about compatibility
	// TODO: Implement revocation status caching (required for higher volume signing)

	// using an OCSP server
	// OCSP requires issuer certificate.
	if issuer != nil && len(cert.OCSPServer) > 0 {
		err := embedOCSPRevocationStatus(cert, issuer, i)
		if err != nil {
			return err
		}
	}

	// using a crl
	if len(cert.CRLDistributionPoints) > 0 {
		err := embedCRLRevocationStatus(cert, issuer, i)
		if err != nil {
			return err
		}
	}

	return nil
}

// BestEffortEmbedRevocationStatusFunction wraps DefaultEmbedRevocationStatusFunction
// but never returns an error, so a missing or unreachable revocation responder
// does not fail signing (the signature is simply not LTV-complete).
func BestEffortEmbedRevocationStatusFunction(cert, issuer *x509.Certificate, i *revocation.InfoArchival) error {
	_ = DefaultEmbedRevocationStatusFunction(cert, issuer, i)
	return nil
}

// SignLTV signs the PDF like Sign and then appends a PDF-level DSS (Document
// Security Store) dictionary built from the revocation data gathered during
// signing, producing an LTV-enabled signature. When SignData.RevocationFunction
// is nil it defaults to BestEffortEmbedRevocationStatusFunction so a missing or
// unreachable responder never fails signing.
func SignLTV(input io.ReadSeeker, output io.Writer, rdr *pdf.Reader, size int64, signData SignData) (*common.SignatureInfo, error) {
	var ocsps, crls [][]byte
	inner := signData.RevocationFunction
	if inner == nil {
		inner = BestEffortEmbedRevocationStatusFunction
	}
	signData.RevocationFunction = func(cert, issuer *x509.Certificate, ia *revocation.InfoArchival) error {
		prevOCSP, prevCRL := len(ia.OCSP), len(ia.CRL)
		err := inner(cert, issuer, ia)
		for _, o := range ia.OCSP[prevOCSP:] {
			ocsps = append(ocsps, o.FullBytes)
		}
		for _, c := range ia.CRL[prevCRL:] {
			crls = append(crls, c.FullBytes)
		}
		return err
	}

	var buf bytes.Buffer
	info, err := Sign(input, &buf, rdr, size, signData)
	if err != nil {
		return nil, err
	}
	signed := buf.Bytes()

	var certs []*x509.Certificate
	if len(signData.CertificateChains) > 0 {
		certs = signData.CertificateChains[0]
	}
	var enc *EncryptionContext
	if key := rdr.EncryptionKey(); key != nil {
		enc = &EncryptionContext{Key: key, UseAES: rdr.UseAES(), EncVersion: rdr.EncVersion()}
	}
	if (len(ocsps) > 0 || len(crls) > 0) && len(certs) > 0 {
		augmented, dssErr := AddValidationData(signed, certs, ocsps, crls, enc)
		if dssErr != nil {
			return nil, fmt.Errorf("add validation data: %w", dssErr)
		}
		signed = augmented
	}

	if _, err := output.Write(signed); err != nil {
		return nil, err
	}
	return info, nil
}

// SignLTA signs the PDF, embeds long-term validation data (DSS dictionary),
// and appends a PAdES-BASELINE-LTA archive document timestamp (/DocTimeStamp)
// covering the entire LT revision. The approval signature does not embed its
// own signature timestamp; the archive timestamp covers the whole document and
// serves as the sole proof-of-existence, which validators accept as LTA.
func SignLTA(input io.ReadSeeker, output io.Writer, rdr *pdf.Reader, size int64, signData SignData) (*common.SignatureInfo, error) {
	// Clear TSA for the approval-signature step: only the archive timestamp
	// below calls the TSA. Passing TSA here would embed a signature timestamp
	// in the approval signature and produce two TSA calls for the same document.
	ltSignData := signData
	ltSignData.TSA = TSA{}
	var ltBuf bytes.Buffer
	info, err := SignLTV(input, &ltBuf, rdr, size, ltSignData)
	if err != nil {
		return nil, err
	}
	ltBytes := ltBuf.Bytes()
	ltReader, err := pdf.NewReader(bytes.NewReader(ltBytes), int64(len(ltBytes)))
	if err != nil {
		return nil, fmt.Errorf("lta: re-open LT pdf: %w", err)
	}
	if _, err = Sign(bytes.NewReader(ltBytes), output, ltReader, int64(len(ltBytes)), SignData{
		Signature:       SignDataSignature{CertType: TimeStampSignature},
		DigestAlgorithm: signData.DigestAlgorithm,
		TSA:             signData.TSA,
	}); err != nil {
		return nil, fmt.Errorf("lta: archive timestamp: %w", err)
	}
	return info, nil
}

func dssLastSubmatch(re *regexp.Regexp, b []byte) [][]byte {
	ms := re.FindAllSubmatch(b, -1)
	if len(ms) == 0 {
		return nil
	}
	return ms[len(ms)-1]
}

func dssRefArray(nums []int) string {
	var sb strings.Builder
	sb.WriteByte('[')
	for i, n := range nums {
		if i > 0 {
			sb.WriteByte(' ')
		}
		fmt.Fprintf(&sb, "%d 0 R", n)
	}
	sb.WriteByte(']')
	return sb.String()
}

// dssContiguousRuns groups sorted object numbers into [start, count] runs.
func dssContiguousRuns(nums []int) [][2]int {
	var runs [][2]int
	for i := 0; i < len(nums); {
		j := i
		for j+1 < len(nums) && nums[j+1] == nums[j]+1 {
			j++
		}
		runs = append(runs, [2]int{nums[i], j - i + 1})
		i = j + 1
	}
	return runs
}

// dssVRIKey returns the uppercase hex SHA-1 of the last signature's /Contents
// value, used as the key of the /VRI dictionary (ISO 32000-2 / PAdES).
func dssVRIKey(pdf []byte) string {
	idx := bytes.LastIndex(pdf, []byte("/Contents"))
	if idx < 0 {
		return ""
	}
	rel := bytes.IndexByte(pdf[idx:], '<')
	if rel < 0 {
		return ""
	}
	lt := idx + rel
	rel = bytes.IndexByte(pdf[lt:], '>')
	if rel < 0 {
		return ""
	}
	gt := lt + rel
	hexStr := bytes.Map(func(r rune) rune {
		switch {
		case r >= '0' && r <= '9', r >= 'a' && r <= 'f', r >= 'A' && r <= 'F':
			return r
		}
		return -1
	}, pdf[lt+1:gt])
	der := make([]byte, hex.DecodedLen(len(hexStr)))
	n, err := hex.Decode(der, hexStr)
	if err != nil || n == 0 {
		return ""
	}
	sum := sha1.Sum(der[:n])
	return strings.ToUpper(hex.EncodeToString(sum[:]))
}

// dssEncryptStream encrypts a new stream object's payload with the document's
// encryption key when the source PDF is encrypted. It is defined at file scope
// (not inside AddValidationData) so that the pdf package identifier is not
// shadowed by AddValidationData's pdf []byte parameter.
func dssEncryptStream(enc *EncryptionContext, objID int, data []byte) ([]byte, error) {
	if enc == nil {
		return data, nil
	}
	return pdf.EncryptStream(enc.Key, enc.UseAES, enc.EncVersion, uint32(objID), 0, data)
}

// AddValidationData appends an incremental update adding an ISO 32000 DSS
// (Document Security Store) dictionary that references the given chain
// certificates and OCSP/CRL responses, so the signature is recognised as
// LTV-enabled by viewers such as Adobe Acrobat. The certificates and
// revocation responses must be DER-encoded.
//
// When enc is non-nil the source PDF is encrypted, so each new cert/OCSP/CRL
// stream payload is encrypted with the document key; the DSS dictionary itself
// holds only references and names, which are never encrypted.
//
// The cross-reference of the incremental update matches the document's existing
// type (classic table or cross-reference stream).
func AddValidationData(pdf []byte, certs []*x509.Certificate, ocsps, crls [][]byte, enc *EncryptionContext) ([]byte, error) {
	rootM := dssLastSubmatch(regexp.MustCompile(`/Root\s+(\d+)\s+(\d+)\s+R`), pdf)
	if rootM == nil {
		return nil, fmt.Errorf("/Root not found")
	}
	rootNum, _ := strconv.Atoi(string(rootM[1]))
	rootGen, _ := strconv.Atoi(string(rootM[2]))

	sizeM := dssLastSubmatch(regexp.MustCompile(`/Size\s+(\d+)`), pdf)
	if sizeM == nil {
		return nil, fmt.Errorf("/Size not found")
	}
	size, _ := strconv.Atoi(string(sizeM[1]))

	sxM := dssLastSubmatch(regexp.MustCompile(`startxref\s+(\d+)`), pdf)
	if sxM == nil {
		return nil, fmt.Errorf("startxref not found")
	}
	prevStartxref := string(sxM[1])
	prevStartxrefOff, _ := strconv.Atoi(prevStartxref)

	infoRef := ""
	if m := dssLastSubmatch(regexp.MustCompile(`/Info\s+(\d+)\s+(\d+)\s+R`), pdf); m != nil {
		infoRef = fmt.Sprintf(" /Info %s %s R", m[1], m[2])
	}

	// An encrypted document requires /Encrypt and /ID in every trailer of an
	// incremental update. Without them a reader that starts from this latest
	// trailer does not know the file is encrypted and reads the (still
	// encrypted) streams and strings as plaintext, producing garbage.
	encRef := ""
	if m := dssLastSubmatch(regexp.MustCompile(`/Encrypt\s+(\d+)\s+(\d+)\s+R`), pdf); m != nil {
		encRef = fmt.Sprintf(" /Encrypt %s %s R", m[1], m[2])
	}
	idRef := ""
	if m := dssLastSubmatch(regexp.MustCompile(`(?s)/ID\s*(\[[^\]]*\])`), pdf); m != nil {
		idRef = fmt.Sprintf(" /ID %s", m[1])
	}

	catRe := regexp.MustCompile(fmt.Sprintf(`(?s)\b%d\s+%d\s+obj\b.*?endobj`, rootNum, rootGen))
	catLoc := catRe.FindIndex(pdf)
	if catLoc == nil {
		return nil, fmt.Errorf("catalog object %d not found", rootNum)
	}
	catText := append([]byte(nil), pdf[catLoc[0]:catLoc[1]]...)
	dictAt := bytes.Index(catText, []byte("<<"))
	if dictAt < 0 {
		return nil, fmt.Errorf("catalog dictionary not found")
	}

	num := size
	next := func(k int) []int {
		out := make([]int, k)
		for i := 0; i < k; i++ {
			out[i] = num
			num++
		}
		return out
	}
	certNums := next(len(certs))
	ocspNums := next(len(ocsps))
	crlNums := next(len(crls))
	dssNum := num
	num++

	var buf bytes.Buffer
	buf.Write(pdf)
	if len(pdf) > 0 && pdf[len(pdf)-1] != '\n' {
		buf.WriteByte('\n')
	}

	offsets := map[int]int{}
	stream := func(n int, der []byte) error {
		payload, err := dssEncryptStream(enc, n, der)
		if err != nil {
			return err
		}
		offsets[n] = buf.Len()
		fmt.Fprintf(&buf, "%d 0 obj\n<< /Length %d >>\nstream\n", n, len(payload))
		buf.Write(payload)
		buf.WriteString("\nendstream\nendobj\n")
		return nil
	}
	for i, c := range certs {
		if err := stream(certNums[i], c.Raw); err != nil {
			return nil, err
		}
	}
	for i, d := range ocsps {
		if err := stream(ocspNums[i], d); err != nil {
			return nil, err
		}
	}
	for i, d := range crls {
		if err := stream(crlNums[i], d); err != nil {
			return nil, err
		}
	}

	offsets[dssNum] = buf.Len()
	fmt.Fprintf(&buf, "%d 0 obj\n<< /Type /DSS", dssNum)
	if len(certNums) > 0 {
		fmt.Fprintf(&buf, " /Certs %s", dssRefArray(certNums))
	}
	if len(ocspNums) > 0 {
		fmt.Fprintf(&buf, " /OCSPs %s", dssRefArray(ocspNums))
	}
	if len(crlNums) > 0 {
		fmt.Fprintf(&buf, " /CRLs %s", dssRefArray(crlNums))
	}
	if key := dssVRIKey(pdf); key != "" {
		fmt.Fprintf(&buf, " /VRI << /%s <<", key)
		if len(certNums) > 0 {
			fmt.Fprintf(&buf, " /Cert %s", dssRefArray(certNums))
		}
		if len(ocspNums) > 0 {
			fmt.Fprintf(&buf, " /OCSP %s", dssRefArray(ocspNums))
		}
		if len(crlNums) > 0 {
			fmt.Fprintf(&buf, " /CRL %s", dssRefArray(crlNums))
		}
		buf.WriteString(" >> >>")
	}
	buf.WriteString(" >>\nendobj\n")

	// Rewrite the catalog with a /DSS reference (same object number, new offset).
	offsets[rootNum] = buf.Len()
	buf.Write(catText[:dictAt+2])
	fmt.Fprintf(&buf, " /DSS %d 0 R", dssNum)
	buf.Write(catText[dictAt+2:])
	buf.WriteByte('\n')

	// Match the existing cross-reference type. A classic table starts with the
	// "xref" keyword at the startxref offset; otherwise it is a stream object.
	usesXrefStream := prevStartxrefOff <= 0 ||
		prevStartxrefOff+4 > len(pdf) ||
		!bytes.Equal(pdf[prevStartxrefOff:prevStartxrefOff+4], []byte("xref"))

	if !usesXrefStream {
		xrefOff := buf.Len()
		buf.WriteString("xref\n")
		nums := make([]int, 0, len(offsets))
		for n := range offsets {
			nums = append(nums, n)
		}
		sort.Ints(nums)
		buf.WriteString("0 1\n0000000000 65535 f\r\n")
		for _, run := range dssContiguousRuns(nums) {
			fmt.Fprintf(&buf, "%d %d\n", run[0], run[1])
			for k := 0; k < run[1]; k++ {
				fmt.Fprintf(&buf, "%010d 00000 n\r\n", offsets[run[0]+k])
			}
		}
		fmt.Fprintf(&buf, "trailer\n<< /Size %d /Root %d %d R /Prev %s%s%s%s >>\nstartxref\n%d\n%%%%EOF\n",
			num, rootNum, rootGen, prevStartxref, infoRef, encRef, idRef, xrefOff)
		return buf.Bytes(), nil
	}

	// Cross-reference stream: the new xref stream is itself an object, so it
	// must appear in its own xref entries.
	xrefNum := num
	num++
	xrefOff := buf.Len()
	offsets[xrefNum] = xrefOff

	nums := make([]int, 0, len(offsets))
	for n := range offsets {
		nums = append(nums, n)
	}
	sort.Ints(nums)

	// Entries: W = [1 4 2] (type, 4-byte offset, 2-byte generation).
	var bin bytes.Buffer
	for _, n := range nums {
		off := offsets[n]
		bin.WriteByte(1)
		bin.WriteByte(byte(off >> 24))
		bin.WriteByte(byte(off >> 16))
		bin.WriteByte(byte(off >> 8))
		bin.WriteByte(byte(off))
		bin.WriteByte(0)
		bin.WriteByte(0)
	}
	var index strings.Builder
	for i, run := range dssContiguousRuns(nums) {
		if i > 0 {
			index.WriteByte(' ')
		}
		fmt.Fprintf(&index, "%d %d", run[0], run[1])
	}

	fmt.Fprintf(&buf, "%d 0 obj\n<< /Type /XRef /Size %d /Root %d %d R /Prev %s%s%s%s /W [1 4 2] /Index [%s] /Length %d >>\nstream\n",
		xrefNum, num, rootNum, rootGen, prevStartxref, infoRef, encRef, idRef, index.String(), bin.Len())
	buf.Write(bin.Bytes())
	buf.WriteString("\nendstream\nendobj\n")
	fmt.Fprintf(&buf, "startxref\n%d\n%%%%EOF\n", xrefOff)

	return buf.Bytes(), nil
}
