package branch

import (
	"github.com/pachyderm/pachyderm/src/pfs"
	"github.com/pachyderm/pachyderm/src/pkg/timing"
	"github.com/pachyderm/pachyderm/src/pps/store"
)

type Brancher interface {
	GetOutputCommitID(
		inputRepositoryName string,
		inputCommitID string,
		outputRepositoryName string,
	) (string, error)
	CommitOutstanding() error
	// TODO(pedge)
	//DeleteOutstanding() error
}

func NewBrancher(
	pfsAPIClient pfs.ApiClient,
	storeClient store.Client,
	timer timing.Timer,
) Brancher {
	return newBrancher(
		pfsAPIClient,
		storeClient,
		timer,
	)
}
