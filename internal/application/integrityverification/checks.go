package integrityverification

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/healthtrust-exchange/healthtrust-exchange/internal/ledger"
	"github.com/healthtrust-exchange/healthtrust-exchange/internal/persistence/authorityledger"
	"github.com/healthtrust-exchange/healthtrust-exchange/internal/submission"
	"github.com/jackc/pgx/v5"
)

func hashString(value []byte) string { return "0x" + hex.EncodeToString(value) }

func compare(t CheckType, id, expected, observed, class string) Check {
	if expected == observed {
		return verified(t, id, expected, observed)
	}
	return failed(t, id, expected, observed, class)
}

func transactionEvidence(domainID string, sub authorityledger.Submission, receipt ledger.Receipt) (Check, bool) {
	if receipt.Status == ledger.ReceiptUnknown {
		return indeterminate(CheckTransaction, domainID, ClassReceiptUnavailable), true
	}
	if receipt.Status != ledger.ReceiptSuccessful {
		return failed(CheckTransaction, domainID, "SUCCESSFUL", fmt.Sprint(receipt.Status), ClassReceiptFailed), false
	}
	var hash ledger.TransactionHash
	copy(hash[:], sub.TransactionHash)
	if receipt.Hash != hash {
		return failed(CheckTransaction, domainID, hash.String(), receipt.Hash.String(), ClassMismatch), false
	}
	if int64(receipt.BlockNumber) != *sub.BlockNumber {
		return failed(CheckTransaction, domainID, fmt.Sprint(*sub.BlockNumber), fmt.Sprint(receipt.BlockNumber), ClassMismatch), false
	}
	if hashString(sub.BlockHash) != receipt.BlockHash.String() {
		return failed(CheckTransaction, domainID, hashString(sub.BlockHash), receipt.BlockHash.String(), ClassMismatch), false
	}
	return verified(CheckTransaction, domainID, hash.String(), receipt.Hash.String()), false
}

func (s *Service) transactionCheck(ctx context.Context, kind, domainID string) (Check, bool, error) {
	sub, err := s.repo.GetSubmissionByDomain(ctx, kind, domainID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return indeterminate(CheckTransaction, domainID, ClassUnconfirmed), true, nil
		}
		return Check{}, false, missing(err)
	}
	if sub.State != "CONFIRMED" || len(sub.TransactionHash) != 32 || sub.BlockNumber == nil || len(sub.BlockHash) != 32 {
		return indeterminate(CheckTransaction, domainID, ClassUnconfirmed), true, nil
	}
	var hash ledger.TransactionHash
	copy(hash[:], sub.TransactionHash)
	receipt, err := s.ledger.TransactionReceipt(ctx, hash)
	if err != nil {
		return indeterminate(CheckTransaction, domainID, ClassReceiptUnavailable), true, nil
	}
	check, unavailable := transactionEvidence(domainID, sub, receipt)
	return check, unavailable, nil
}

func (s *Service) intendedCommitmentCheck(ctx context.Context, kind, domainID, current string) (Check, bool, error) {
	sub, err := s.repo.GetSubmissionByDomain(ctx, kind, domainID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return indeterminate(CheckCanonicalCommitment, domainID+":submission", ClassUnconfirmed), true, nil
		}
		return Check{}, false, err
	}
	expected := hashString(sub.IntendedCommitment[:])
	return compare(CheckCanonicalCommitment, domainID+":submission", expected, current, ClassMismatch), false, nil
}

func operationAuthority() string { return submission.OperationAuthorityAssertion }
func operationResponse() string  { return submission.OperationResponseVersion }
