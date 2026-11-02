//go:build windows

package main

// Bắt âm thanh đang phát trên loa mặc định (WASAPI loopback).

import (
	"encoding/binary"
	"fmt"
	"math"
	"runtime"
	"time"
	"unsafe"

	ole "github.com/go-ole/go-ole"
	"github.com/moutend/go-wca/pkg/wca"
)

func captureLoop(stop chan struct{}, out func([]byte), errc chan error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	fail := func(step string, err error) { errc <- fmt.Errorf("%s: %v", step, err) }

	if err := ole.CoInitializeEx(0, ole.COINIT_MULTITHREADED); err != nil {
		if oe, ok := err.(*ole.OleError); !ok || oe.Code() != 1 { // S_FALSE = đã khởi tạo
			fail("CoInitializeEx", err)
			return
		}
	}
	defer ole.CoUninitialize()

	var mmde *wca.IMMDeviceEnumerator
	if err := wca.CoCreateInstance(wca.CLSID_MMDeviceEnumerator, 0, wca.CLSCTX_ALL, wca.IID_IMMDeviceEnumerator, &mmde); err != nil {
		fail("MMDeviceEnumerator", err)
		return
	}
	defer mmde.Release()
	var mmd *wca.IMMDevice
	if err := mmde.GetDefaultAudioEndpoint(wca.ERender, wca.EConsole, &mmd); err != nil {
		fail("không có loa mặc định", err)
		return
	}
	defer mmd.Release()
	var ac *wca.IAudioClient
	if err := mmd.Activate(wca.IID_IAudioClient, wca.CLSCTX_ALL, nil, &ac); err != nil {
		fail("IAudioClient", err)
		return
	}
	defer ac.Release()
	var wfx *wca.WAVEFORMATEX
	if err := ac.GetMixFormat(&wfx); err != nil {
		fail("GetMixFormat", err)
		return
	}
	defer ole.CoTaskMemFree(uintptr(unsafe.Pointer(wfx)))
	chans := int(wfx.NChannels)
	bits := int(wfx.WBitsPerSample)
	block := int(wfx.NBlockAlign)
	isFloat := bits == 32 && wfx.WFormatTag != wca.WAVE_FORMAT_PCM
	if err := ac.Initialize(wca.AUDCLNT_SHAREMODE_SHARED, wca.AUDCLNT_STREAMFLAGS_LOOPBACK, 2_000_000, 0, wfx, nil); err != nil {
		fail("Initialize", err)
		return
	}
	var acc *wca.IAudioCaptureClient
	if err := ac.GetService(wca.IID_IAudioCaptureClient, &acc); err != nil {
		fail("IAudioCaptureClient", err)
		return
	}
	defer acc.Release()
	if err := ac.Start(); err != nil {
		fail("Start", err)
		return
	}
	defer ac.Stop()
	errc <- nil

	rs := &resampler{inRate: float64(wfx.NSamplesPerSec)}
	var mono []float32
	tick := time.NewTicker(10 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-stop:
			return
		case <-tick.C:
		}
		for {
			var n uint32
			if acc.GetNextPacketSize(&n) != nil || n == 0 {
				break
			}
			var data *byte
			var frames, flags uint32
			var devPos, qpc uint64
			if acc.GetBuffer(&data, &frames, &flags, &devPos, &qpc) != nil {
				break
			}
			mono = mono[:0]
			silent := flags&0x2 != 0 // AUDCLNT_BUFFERFLAGS_SILENT
			raw := unsafe.Slice(data, int(frames)*block)
			for f := 0; f < int(frames); f++ {
				var sum float32
				if !silent {
					for c := 0; c < chans; c++ {
						off := f*block + c*bits/8
						switch {
						case isFloat:
							sum += math.Float32frombits(binary.LittleEndian.Uint32(raw[off:]))
						case bits == 16:
							sum += float32(int16(binary.LittleEndian.Uint16(raw[off:]))) / 32768
						case bits == 32:
							sum += float32(int32(binary.LittleEndian.Uint32(raw[off:]))) / 2147483648
						case bits == 24:
							v := int32(raw[off]) | int32(raw[off+1])<<8 | int32(int8(raw[off+2]))<<16
							sum += float32(v) / 8388608
						}
					}
				}
				mono = append(mono, sum/float32(chans))
			}
			acc.ReleaseBuffer(frames)
			rs.push(mono, out)
		}
	}
}
