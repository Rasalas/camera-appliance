//go:build !unix

package state

import "context"

var accessPolicyLock = make(chan struct{}, 1)

func LockAccessPolicy(ctx context.Context, _ string) (func(), error) {
	select {
	case accessPolicyLock <- struct{}{}:
		return func() { <-accessPolicyLock }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}
