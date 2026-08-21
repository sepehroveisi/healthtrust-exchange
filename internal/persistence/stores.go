package persistence

import (
	"context"

	"github.com/healthtrust-exchange/healthtrust-exchange/internal/blockchain"
	"github.com/healthtrust-exchange/healthtrust-exchange/internal/identity"
)

type BlockStore interface {
	SaveBlock(context.Context, blockchain.Block) error
	GetBlock(context.Context, uint64) (blockchain.Block, error)
	GetBlocksFrom(context.Context, uint64) ([]blockchain.Block, error)
	GetLatestBlock(context.Context) (blockchain.Block, error)
}

type IdentityStore interface {
	SaveOrganization(context.Context, identity.Organization) error
	GetOrganization(context.Context, string) (identity.Organization, error)
	ListOrganizations(context.Context) ([]identity.Organization, error)
	SaveActor(context.Context, identity.Actor) error
	GetActor(context.Context, string) (identity.Actor, error)
	ListActors(context.Context) ([]identity.Actor, error)
}

type Store interface {
	BlockStore
	IdentityStore
}
