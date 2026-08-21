package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/healthtrust-exchange/healthtrust-exchange/internal/blockchain"
)

var ErrBlockNotFound = errors.New("block not found")

func (s *Store) SaveBlock(ctx context.Context, block blockchain.Block) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin block transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err = tx.Exec(ctx, `INSERT INTO blocks (height,hash,previous_hash,timestamp_unix_nano) VALUES ($1,$2,$3,$4)`, block.Height, block.Hash[:], block.PreviousHash[:], block.Timestamp.UTC().UnixNano()); err != nil {
		return fmt.Errorf("insert block: %w", err)
	}
	for position, value := range block.Transactions {
		if _, err = tx.Exec(ctx, `INSERT INTO transactions (id,block_height,position,type,actor_id,organization_id,resource_id,payload_hash,timestamp_unix_nano,public_key,signature) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`, value.ID, block.Height, position, value.Type, value.ActorID, value.OrganizationID, value.ResourceID, value.PayloadHash, value.Timestamp.UTC().UnixNano(), value.PublicKey, value.Signature); err != nil {
			return fmt.Errorf("insert transaction %d: %w", position, err)
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit block transaction: %w", err)
	}
	return nil
}
func (s *Store) GetBlock(ctx context.Context, height uint64) (blockchain.Block, error) {
	blocks, err := s.getBlocks(ctx, `WHERE height=$1`, height)
	if err != nil {
		return blockchain.Block{}, err
	}
	if len(blocks) == 0 {
		return blockchain.Block{}, ErrBlockNotFound
	}
	return blocks[0], nil
}
func (s *Store) GetBlocksFrom(ctx context.Context, height uint64) ([]blockchain.Block, error) {
	return s.getBlocks(ctx, `WHERE height >= $1`, height)
}
func (s *Store) GetLatestBlock(ctx context.Context) (blockchain.Block, error) {
	blocks, err := s.getBlocks(ctx, `ORDER BY height DESC LIMIT 1`)
	if err != nil {
		return blockchain.Block{}, err
	}
	if len(blocks) == 0 {
		return blockchain.Block{}, ErrBlockNotFound
	}
	return blocks[0], nil
}
func (s *Store) getBlocks(ctx context.Context, clause string, args ...any) ([]blockchain.Block, error) {
	query := `SELECT height,hash,previous_hash,timestamp_unix_nano FROM blocks ` + clause
	if clause[:5] == "WHERE" {
		query += ` ORDER BY height`
	}
	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query blocks: %w", err)
	}
	defer rows.Close()
	var result []blockchain.Block
	for rows.Next() {
		var block blockchain.Block
		var hash, previous []byte
		var timestampUnixNano int64
		if err := rows.Scan(&block.Height, &hash, &previous, &timestampUnixNano); err != nil {
			return nil, err
		}
		if len(hash) != 32 || len(previous) != 32 {
			return nil, errors.New("persisted block hash has invalid length")
		}
		copy(block.Hash[:], hash)
		copy(block.PreviousHash[:], previous)
		block.Timestamp = time.Unix(0, timestampUnixNano).UTC()
		transactions, err := s.getTransactions(ctx, block.Height)
		if err != nil {
			return nil, err
		}
		block.Transactions = transactions
		result = append(result, block)
	}
	return result, rows.Err()
}
func (s *Store) getTransactions(ctx context.Context, height uint64) ([]blockchain.Transaction, error) {
	rows, err := s.pool.Query(ctx, `SELECT id,type,actor_id,organization_id,resource_id,payload_hash,timestamp_unix_nano,public_key,signature FROM transactions WHERE block_height=$1 ORDER BY position`, height)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []blockchain.Transaction
	for rows.Next() {
		var value blockchain.Transaction
		var timestampUnixNano int64
		if err := rows.Scan(&value.ID, &value.Type, &value.ActorID, &value.OrganizationID, &value.ResourceID, &value.PayloadHash, &timestampUnixNano, &value.PublicKey, &value.Signature); err != nil {
			return nil, err
		}
		value.Timestamp = time.Unix(0, timestampUnixNano).UTC()
		result = append(result, value)
	}
	return result, rows.Err()
}
