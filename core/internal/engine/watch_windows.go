package engine

import (
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	kernel    = windows.NewLazySystemDLL("kernel32.dll")
	counter   = kernel.NewProc("QueryPerformanceCounter")
	frequency = func() int64 {
		var f int64
		kernel.NewProc("QueryPerformanceFrequency").Call(uintptr(unsafe.Pointer(&f)))
		return max(f, 1)
	}()
)

type watch int64

func ticks() int64 {
	var c int64
	counter.Call(uintptr(unsafe.Pointer(&c)))
	return c
}

func started() watch { return watch(ticks()) }

func (w watch) elapsed() time.Duration {
	d := ticks() - int64(w)
	return time.Duration(d/frequency)*time.Second + time.Duration(d%frequency*int64(time.Second)/frequency)
}
