package permissions

import "context"

type Request struct {
	Tool    string
	Details string
}

type Manager interface {
	RequestApproval(ctx context.Context, req Request) (bool, error)
}
