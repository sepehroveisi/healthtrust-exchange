package cryptography

import "testing"

func TestSignAndVerify(t *testing.T) {
	publicKey, privateKey, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair() error = %v", err)
	}

	message := []byte("healthtrust audit event")
	signature := Sign(privateKey, message)
	if !Verify(publicKey, message, signature) {
		t.Fatal("Verify() rejected a valid signature")
	}
	if Verify(publicKey, []byte("modified event"), signature) {
		t.Fatal("Verify() accepted a signature for modified data")
	}
}
