package pdf

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"encoding/hex"
	"io"
	"testing"
)

func TestCryptKey(t *testing.T) {
	key := []byte("secret")
	ptr := objptr{id: 10, gen: 0}

	ck1 := cryptKey(key, false, ptr)
	ck2 := cryptKey(key, false, ptr)
	if string(ck1) != string(ck2) {
		t.Error("cryptKey not deterministic")
	}

	ckAES := cryptKey(key, true, ptr)
	if string(ck1) == string(ckAES) {
		t.Error("cryptKey should differ for AES (salt)")
	}
}

func TestDecryptStringRC4(t *testing.T) {
	key := []byte("testkey")
	ptr := objptr{id: 5, gen: 0}
	data := "Hello PDF"

	// Encrypt manually using rc4 logic from read.go
	encrypted, _ := decryptString(key, false, 2, ptr, data)
	// Decrypting again with same key/ptr should recover original because RC4 is XOR
	decrypted, _ := decryptString(key, false, 2, ptr, encrypted)

	if decrypted != data {
		t.Errorf("RC4 Decryption failed: got %q, want %q", decrypted, data)
	}
}

func TestDecryptStringAES(t *testing.T) {
	key := make([]byte, 16) // 128-bit key
	ptr := objptr{id: 1, gen: 0}

	// Create valid AES-CBC encrypted block with padding
	// 16 bytes IV + data
	plaintext := "SecretMessage!!!" // 16 bytes
	iv := make([]byte, 16)
	for i := range iv {
		iv[i] = byte(i)
	}

	block, _ := aes.NewCipher(key) // This is not the derived key, but for simple test it's fine
	mode := cipher.NewCBCEncrypter(block, iv)

	ciphertext := make([]byte, 16)
	mode.CryptBlocks(ciphertext, []byte(plaintext))

	// Add padding block (16 bytes of 0x10)
	padding := make([]byte, 16)
	for i := range padding {
		padding[i] = 16
	}
	ciphertextPadded := make([]byte, 16)
	mode.CryptBlocks(ciphertextPadded, padding)

	full := append(iv, ciphertext...)
	full = append(full, ciphertextPadded...)

	// We need to bypass cryptKey for this unit test or use a pre-calculated derived key.
	// decryptString calls cryptKey(key, true, ptr) if encVersion < 5.
	// Let's use V5 logic which uses the key directly.

	decrypted, err := decryptString(key, true, 5, ptr, string(full))
	if err != nil {
		t.Fatalf("AES Decryption failed: %v", err)
	}

	if decrypted != plaintext {
		t.Errorf("AES Decryption mismatch: got %q, want %q", decrypted, plaintext)
	}
}
func TestAuthenticateV5(t *testing.T) {
	// Vectors from gen_v5.go
	pwd := "user"
	uHex := "8a35e0ef6b995a3af7a084c7b39f3f9aa96f4ce6b961d27d5ee084a779b93ec331323334353637383837363534333231"
	ueHex := "fdf2ebcf67bd7c6f527008513dd4c01c4d5a3db53b16f3713ab07e58e67026e9"

	u, _ := hex.DecodeString(uHex)
	ue, _ := hex.DecodeString(ueHex)

	fek, ok := authenticateV5Password(pwd, u, ue)
	if !ok {
		t.Fatal("Authentication failed")
	}

	expectedFEK := []byte("32-byte-fek-must-be-exactly-32-b")
	if !bytes.Equal(fek, expectedFEK) {
		t.Errorf("FEK mismatch: %q, want %q", string(fek), string(expectedFEK))
	}
}

func TestDecryptStream(t *testing.T) {
	key := make([]byte, 16)
	ptr := objptr{id: 1, gen: 0}

	data := []byte("0123456789ABCDEF") // 16 bytes, exactly one block
	// For simplicity, test with V5 logic (no crpytKey)
	// DecryptStream expects a derived key. If version < 5 it calls cryptKey.
	// We'll test version 5 to skip cryptKey derivation.

	// Create ciphertext
	block, _ := aes.NewCipher(key)
	iv := make([]byte, aes.BlockSize)
	// PKCS#7 adds a full block of 16 bytes if original data is 16 bytes
	padding := bytes.Repeat([]byte{16}, 16)
	padded := append(data, padding...)
	ciphertext := make([]byte, len(padded))
	mode := cipher.NewCBCEncrypter(block, iv)
	mode.CryptBlocks(ciphertext, padded)

	full := append(iv, ciphertext...)

	resultRd, err := decryptStream(key, true, 5, ptr, bytes.NewReader(full))
	if err != nil {
		t.Fatalf("decryptStream failed: %v", err)
	}

	got, _ := io.ReadAll(resultRd)
	// The cbcReader does NOT strip padding, so we expect data + padding
	if !bytes.HasPrefix(got, data) {
		t.Errorf("Decrypted data mismatch: %q, want prefix %q", string(got), string(data))
	}
}

func TestCryptKeyTruncation(t *testing.T) {
	// 40-bit RC4 document key (5 bytes) → object key must be 10 bytes, not 16.
	key := []byte{0x01, 0x02, 0x03, 0x04, 0x05}
	ptr := objptr{id: 42, gen: 0}

	derived := cryptKey(key, false, ptr)
	if len(derived) != 10 {
		t.Errorf("cryptKey with 5-byte doc key: got %d bytes, want 10", len(derived))
	}

	// 128-bit key (16 bytes) → object key stays 16 bytes.
	key128 := make([]byte, 16)
	derived128 := cryptKey(key128, false, ptr)
	if len(derived128) != 16 {
		t.Errorf("cryptKey with 16-byte doc key: got %d bytes, want 16", len(derived128))
	}
}

func TestEncryptStreamRoundTripRC4(t *testing.T) {
	key := []byte{0x01, 0x02, 0x03, 0x04, 0x05}
	ptr := objptr{id: 7, gen: 0}
	plaintext := []byte("stream payload")

	encrypted, err := EncryptStream(key, false, 2, ptr.id, ptr.gen, plaintext)
	if err != nil {
		t.Fatalf("EncryptStream: %v", err)
	}

	rd, err := decryptStream(key, false, 2, ptr, bytes.NewReader(encrypted))
	if err != nil {
		t.Fatalf("decryptStream: %v", err)
	}
	got, err := io.ReadAll(rd)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if !bytes.Equal(got, plaintext) {
		t.Errorf("round-trip mismatch: got %q, want %q", got, plaintext)
	}
}

func TestOkayV4(t *testing.T) {
	// Simple test for okayV4 helper
	encrypt := map[string]Object{
		"CF": {Kind: Dict, DictVal: map[string]Object{}},
	}
	_ = okayV4(encrypt)

	// Negative case
	delete(encrypt, "CF")
	if okayV4(encrypt) {
		t.Error("expected false for missing CF")
	}
}
