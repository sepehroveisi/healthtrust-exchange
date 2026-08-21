package recovery

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/healthtrust-exchange/healthtrust-exchange/internal/authorization"
	"github.com/healthtrust-exchange/healthtrust-exchange/internal/blockchain"
	"github.com/healthtrust-exchange/healthtrust-exchange/internal/demodata"
	"github.com/healthtrust-exchange/healthtrust-exchange/internal/identity"
	"github.com/healthtrust-exchange/healthtrust-exchange/internal/persistence"
)

type State struct {
	Chain    *blockchain.Blockchain
	Registry *identity.MemoryRegistry
}

func Load(ctx context.Context, store persistence.Store, policy authorization.Policy, genesisTimestamp time.Time, seedDemoIdentities bool) (State, error) {
	organizations, err := store.ListOrganizations(ctx)
	if err != nil {
		return State{}, fmt.Errorf("load organizations: %w", err)
	}
	actors, err := store.ListActors(ctx)
	if err != nil {
		return State{}, fmt.Errorf("load actors: %w", err)
	}
	if len(organizations) == 0 && len(actors) == 0 && seedDemoIdentities {
		if err := seed(ctx, store); err != nil {
			return State{}, err
		}
		organizations, err = store.ListOrganizations(ctx)
		if err != nil {
			return State{}, err
		}
		actors, err = store.ListActors(ctx)
		if err != nil {
			return State{}, err
		}
	}
	if seedDemoIdentities {
		if err := seedMissing(ctx, store, organizations, actors); err != nil {
			return State{}, err
		}
		organizations, err = store.ListOrganizations(ctx)
		if err != nil {
			return State{}, err
		}
		actors, err = store.ListActors(ctx)
		if err != nil {
			return State{}, err
		}
	}
	registry := identity.NewMemoryRegistry()
	for _, organization := range organizations {
		if err := registry.RegisterOrganization(organization); err != nil {
			return State{}, fmt.Errorf("register recovered organization: %w", err)
		}
	}
	for _, actor := range actors {
		if err := registry.RegisterActor(actor); err != nil {
			return State{}, fmt.Errorf("register recovered actor: %w", err)
		}
	}
	blocks, err := store.GetBlocksFrom(ctx, 0)
	if err != nil {
		return State{}, fmt.Errorf("load blocks: %w", err)
	}
	expectedGenesis := blockchain.CreateGenesisBlock(genesisTimestamp)
	if len(blocks) == 0 {
		if err := store.SaveBlock(ctx, expectedGenesis); err != nil {
			return State{}, fmt.Errorf("persist genesis: %w", err)
		}
		blocks = []blockchain.Block{expectedGenesis}
	}
	if blocks[0].Height != 0 || blocks[0].Hash != expectedGenesis.Hash {
		return State{}, errors.New("persisted genesis is incompatible with configured network genesis")
	}
	chain, err := blockchain.Recover(blocks, registry, policy)
	if err != nil {
		return State{}, err
	}
	return State{Chain: chain, Registry: registry}, nil
}

func seedMissing(ctx context.Context, store persistence.IdentityStore, organizations []identity.Organization, actors []identity.Actor) error {
	registry, _, err := demodata.Registry()
	if err != nil {
		return err
	}
	haveOrganizations := map[string]bool{}
	for _, value := range organizations {
		haveOrganizations[value.ID] = true
	}
	for _, id := range []string{"hospital-a", "hospital-b"} {
		if !haveOrganizations[id] {
			value, _ := registry.GetOrganization(id)
			if err := store.SaveOrganization(ctx, value); err != nil {
				return err
			}
		}
	}
	haveActors := map[string]bool{}
	for _, value := range actors {
		haveActors[value.ID] = true
	}
	for _, id := range []string{"doctor-a", "doctor-b", "patient-p", "patient-consent-hospital-a", "patient-consent-hospital-b"} {
		if !haveActors[id] {
			value, _ := registry.GetActor(id)
			if err := store.SaveActor(ctx, value); err != nil {
				return err
			}
		}
	}
	return nil
}

func seed(ctx context.Context, store persistence.IdentityStore) error {
	registry, _, err := demodata.Registry()
	if err != nil {
		return err
	}
	for _, organization := range []string{"hospital-a", "hospital-b"} {
		value, err := registry.GetOrganization(organization)
		if err != nil {
			return err
		}
		if err := store.SaveOrganization(ctx, value); err != nil {
			return err
		}
	}
	for _, actorID := range []string{"doctor-a", "doctor-b", "patient-p", "patient-consent-hospital-a", "patient-consent-hospital-b"} {
		value, err := registry.GetActor(actorID)
		if err != nil {
			return err
		}
		if err := store.SaveActor(ctx, value); err != nil {
			return err
		}
	}
	return nil
}
