package storage

import (
	"context"
	"time"
)

// SweepTempObjects deletes temporary audio older than retention and returns the number of
// objects deleted and bytes freed.
//
// This is a pure data operation on the Store — it does not run on a schedule, does not hold a
// goroutine or ticker. The process lifecycle belongs to the cron package, which decides how
// often to run and handles the result.
//
// Only scans the temp branch: saved user voices live in a different branch and must not be
// deleted. That constraint is enforced by List (non-recursive), not by the caller remembering.
func SweepTempObjects(ctx context.Context, store Store, retention time.Duration) (int, int64, error) {
	objects, err := store.List(ctx, TempPrefix)
	if err != nil {
		return 0, 0, err
	}

	cutoff := time.Now().Add(-retention).Unix()
	var removed int
	var freed int64

	for _, o := range objects {
		if o.Modified > cutoff {
			continue
		}
		if err := store.Delete(ctx, o.Key); err != nil {
			// The file may be currently being read to serve a client; the next sweep will clean it.
			continue
		}
		removed++
		freed += o.Size
	}

	return removed, freed, nil
}
