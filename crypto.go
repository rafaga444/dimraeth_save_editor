package main

import (
	"bytes"
	"compress/gzip"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"io"
)

const maxSave = 32 << 20

func derive(salt []byte, info string) []byte {
	master := sha256.Sum256([]byte("IPutMyTrustInYouPushedAsFarAsICouldGo"))
	h := hmac.New(sha256.New, salt)
	h.Write(master[:])
	prk := h.Sum(nil)
	h = hmac.New(sha256.New, prk)
	h.Write([]byte(info))
	h.Write([]byte{1})
	return h.Sum(nil)
}
func decrypt(data []byte) ([]byte, error) {
	bad := errors.New("Corrupted or unsupported JRSF file")
	if len(data) < 102 || string(data[:6]) != "JRSF\x01\x02" {
		return nil, bad
	}
	n := int(binary.BigEndian.Uint32(data[34:38]))
	if n == 0 || n%16 != 0 || n > maxSave || n+86 != len(data) || !bytes.Equal(data[22:34], data[38:50]) {
		return nil, bad
	}
	h := hmac.New(sha256.New, derive(data[6:22], "mac"))
	h.Write(data[6 : 54+n])
	if !hmac.Equal(h.Sum(nil), data[54+n:]) {
		return nil, errors.New("HMAC verification failed: the save file is corrupted")
	}
	block, _ := aes.NewCipher(derive(data[6:22], "enc"))
	plain := make([]byte, n)
	cipher.NewCBCDecrypter(block, data[38:54]).CryptBlocks(plain, data[54:54+n])
	pad := int(plain[n-1])
	if pad < 1 || pad > 16 {
		return nil, bad
	}
	for _, v := range plain[n-pad:] {
		if int(v) != pad {
			return nil, bad
		}
	}
	z, err := gzip.NewReader(bytes.NewReader(plain[:n-pad]))
	if err != nil {
		return nil, err
	}
	defer z.Close()
	out, err := io.ReadAll(io.LimitReader(z, maxSave+1))
	if len(out) > maxSave {
		return nil, bad
	}
	return out, err
}
func encrypt(plain []byte) ([]byte, error) {
	if len(plain) > maxSave {
		return nil, errors.New("Save file exceeds 32 MiB")
	}
	var compressed bytes.Buffer
	z := gzip.NewWriter(&compressed)
	if _, err := z.Write(plain); err != nil {
		return nil, err
	}
	if err := z.Close(); err != nil {
		return nil, err
	}
	p := compressed.Bytes()
	pad := 16 - len(p)%16
	p = append(p, bytes.Repeat([]byte{byte(pad)}, pad)...)
	data := make([]byte, 54+len(p))
	copy(data, []byte("JRSF\x01\x02"))
	if _, err := rand.Read(data[6:22]); err != nil {
		return nil, err
	}
	if _, err := rand.Read(data[38:54]); err != nil {
		return nil, err
	}
	copy(data[22:34], data[38:50])
	binary.BigEndian.PutUint32(data[34:38], uint32(len(p)))
	block, _ := aes.NewCipher(derive(data[6:22], "enc"))
	cipher.NewCBCEncrypter(block, data[38:54]).CryptBlocks(data[54:], p)
	h := hmac.New(sha256.New, derive(data[6:22], "mac"))
	h.Write(data[6:])
	return append(data, h.Sum(nil)...), nil
}
