//go:build !windows

package engine

func keep() func() { return func() {} }
