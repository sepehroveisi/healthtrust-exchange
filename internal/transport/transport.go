package transport

import (
	"context"

	"github.com/healthtrust-exchange/healthtrust-exchange/internal/blockchain"
	"github.com/healthtrust-exchange/healthtrust-exchange/internal/node"
)

type Transport interface {
	BroadcastBlock(ctx context.Context, peers []node.Peer, block blockchain.Block) error
	FetchBlocks(ctx context.Context, peer node.Peer, fromHeight uint64) ([]blockchain.Block, error)
}
