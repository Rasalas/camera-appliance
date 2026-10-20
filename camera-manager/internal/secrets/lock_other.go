//go:build !unix

package secrets

func lockDir(string) (func(), error) {
	return func() {}, nil
}
