//go:build !windows

package engine

import "time"

type watch time.Time

func started() watch { return watch(time.Now()) }

func (w watch) elapsed() time.Duration { return time.Since(time.Time(w)) }
