package main

import (
	"crypto/sha256"
	"fmt"
	"log"
	"time"

	"github.com/healthtrust-exchange/healthtrust-exchange/internal/authorization"
	"github.com/healthtrust-exchange/healthtrust-exchange/internal/blockchain"
	trustcrypto "github.com/healthtrust-exchange/healthtrust-exchange/internal/cryptography"
	"github.com/healthtrust-exchange/healthtrust-exchange/internal/identity"
)

func main() {
	doctorPublicKey, doctorPrivateKey, err := trustcrypto.GenerateKeyPair()
	if err != nil {
		log.Fatal(err)
	}

	registry := identity.NewMemoryRegistry()
	if err := registry.RegisterOrganization(identity.Organization{ID: "hospital-a", Name: "Hospital A", Status: identity.OrganizationActive}); err != nil {
		log.Fatal(err)
	}
	if err := registry.RegisterActor(identity.Actor{ID: "doctor-a", OrganizationID: "hospital-a", Role: identity.RoleDoctor, PublicKey: doctorPublicKey, Status: identity.ActorActive}); err != nil {
		log.Fatal(err)
	}

	baseTime := time.Now().UTC()
	recordHash := sha256.Sum256([]byte("fictional clinical record stored off-chain"))
	record := blockchain.Transaction{
		ID: "event-demo-001", Type: blockchain.RecordCommitted,
		ActorID: "doctor-a", OrganizationID: "hospital-a",
		ResourceID: "record-fictional-001", PayloadHash: recordHash[:], Timestamp: baseTime,
	}
	if err := record.Sign(doctorPrivateKey); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Transaction signature valid: %t\n", record.VerifySignature())

	chain := blockchain.New(baseTime.Add(-time.Minute), registry, authorization.HealthcarePolicy{})
	if _, err := chain.AddBlock(baseTime, []blockchain.Transaction{record}); err != nil {
		log.Fatal(err)
	}

	_, attackerPrivateKey, err := trustcrypto.GenerateKeyPair()
	if err != nil {
		log.Fatal(err)
	}
	forged := record
	forged.ID = "event-demo-forged"
	if err := forged.Sign(attackerPrivateKey); err != nil {
		log.Fatal(err)
	}
	_, rejectedErr := chain.AddBlock(baseTime.Add(time.Second), []blockchain.Transaction{forged})
	if err := chain.Validate(); err != nil {
		log.Fatal(err)
	}

	fmt.Println("Identity registered: doctor-a at hospital-a (Doctor, Active)")
	fmt.Println("Authorized RecordCommitted accepted: true")
	fmt.Printf("Forged doctor identity correctly rejected: %t (%v)\n", rejectedErr != nil, rejectedErr)
	fmt.Println("Blockchain valid: true")
	for _, block := range chain.Blocks() {
		fmt.Printf("Block %d | transactions: %d | hash: %x | previous: %x\n",
			block.Height, len(block.Transactions), block.Hash[:6], block.PreviousHash[:6])
	}
}
