package tamperdemo

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

func TestAdditionalSyntheticFixtureRestrictions(t *testing.T) {
	tests := []struct {
		name  string
		setup func(*fakeRepository)
		want  error
	}{
		{"wrong event identity", func(repo *fakeRepository) { repo.assertion.ID = "OTHER-EVENT" }, ErrSyntheticFixtureMismatch},
		{"wrong series identity", func(repo *fakeRepository) { repo.series.ID = "OTHER-SERIES" }, ErrSyntheticFixtureMismatch},
		{"missing Hospital A response", func(repo *fakeRepository) { repo.history = nil }, ErrSyntheticFixtureMissing},
		{"missing Hospital A policy", func(repo *fakeRepository) { repo.history[0].PolicyVersionID = nil }, ErrSyntheticFixtureMismatch},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repo := validRepository()
			test.setup(repo)
			executor := &fakeExecutor{tag: pgconn.NewCommandTag("UPDATE 1")}
			_, err := newService(true, repo, executor, successfulVerifier()).Demonstrate(context.Background(), TargetHospitalAPolicy)
			if !errors.Is(err, test.want) || executor.calls != 0 {
				t.Fatalf("error=%v want=%v writes=%d", err, test.want, executor.calls)
			}
		})
	}
}
