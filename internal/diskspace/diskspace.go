// Package diskspace reports filesystem capacity in a cross-platform way.
package diskspace

// Get returns the total and free (available to the caller) bytes for the
// filesystem containing path.
func Get(path string) (total, free uint64, err error) {
	return get(path)
}
