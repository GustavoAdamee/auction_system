package main

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/pem"
	"log"
	"os"
)

type AssimetricKeys struct {
	PrivateKey *rsa.PrivateKey
	PublicKey  *rsa.PublicKey
}

func (ass_keys *AssimetricKeys) Init() {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		log.Fatalf("Error generating RSA key pair: %s", err)
	}
	ass_keys.PrivateKey = privateKey
	ass_keys.PublicKey = &privateKey.PublicKey
}

func (ass_keys *AssimetricKeys) Generate_private_key_pem_file(outpout_path string) error {
	priv_key_bytes := x509.MarshalPKCS1PrivateKey(ass_keys.PrivateKey)
	priv_key_PEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: priv_key_bytes,
	})
	err := os.WriteFile(outpout_path+".pem", priv_key_PEM, 0600)
	return err
}

func (ass_keys *AssimetricKeys) Generate_public_key_pem_file(outpout_path string) error {
	pub_key_bytes, err := x509.MarshalPKIXPublicKey(ass_keys.PublicKey)
	if err != nil {
		return err
	}
	pub_key_PEM := pem.EncodeToMemory(&pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: pub_key_bytes,
	})
	err = os.WriteFile(outpout_path+".pem", pub_key_PEM, 0644)
	return err
}

func (ass_keys *AssimetricKeys) Get_public_key_pem() ([]byte, error) {
	pub_key_bytes, err := x509.MarshalPKIXPublicKey(ass_keys.PublicKey)
	if err != nil {
		return nil, err
	}
	pub_key_PEM := pem.EncodeToMemory(&pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: pub_key_bytes,
	})
	return pub_key_PEM, nil
}

func (ass_keys *AssimetricKeys) Sign_data(message string) ([]byte, error) {
	message_in_bytes := []byte(message) //Entender isso dps
	hash := sha256.New()
	_, err := hash.Write(message_in_bytes)
	if err != nil {
		log.Fatalf("Error hashing message: %v", err)
	}
	hashedMessage := hash.Sum(nil)
	signature, err := rsa.SignPKCS1v15(rand.Reader, ass_keys.PrivateKey, crypto.SHA256, hashedMessage)
	if err != nil {
		log.Fatalf("Error signing message: %v", err)
	}
	return signature, nil
}
