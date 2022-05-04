package av

/*
#cgo pkg-config: libavformat
#include <libavformat/avformat.h>
#include "demux.h"
*/
import "C"
import (
	"errors"
	"unsafe"
)

type DeviceContext struct {
	Sinks       []*IndexedSink
	avformatctx *C.AVFormatContext
	packet      *AVPacket
}

// v4l2, /dev/video0 for example
func NewDevice(format, device string) (*DeviceContext, error) {
	cformat := C.CString(format)
	defer C.free(unsafe.Pointer(cformat))

	inputformat := C.av_find_input_format(cformat)
	if inputformat == nil {
		return nil, errors.New("could not find sdp format")
	}

	avformatctx := C.avformat_alloc_context()
	if avformatctx == nil {
		return nil, errors.New("failed to create format context")
	}

	cdevice := C.CString(device)
	defer C.free(unsafe.Pointer(cdevice))

	if averr := C.avformat_open_input(&avformatctx, cdevice, inputformat, nil); averr < 0 {
		return nil, av_err("avformat_open_input", averr)
	}

	c := &DeviceContext{
		avformatctx: avformatctx,
		packet:      NewAVPacket(),
	}

	if averr := C.avformat_find_stream_info(avformatctx, nil); averr < 0 {
		return nil, av_err("avformat_find_stream_info", averr)
	}

	return c, nil
}

func (c *DeviceContext) AVFormatContext() *C.AVFormatContext {
	return c.avformatctx
}

func (c *DeviceContext) Streams() []*AVStream {
	streams := make([]*AVStream, c.avformatctx.nb_streams)
	for i, stream := range (*[1 << 30]*C.AVStream)(unsafe.Pointer(c.avformatctx.streams))[:c.avformatctx.nb_streams] {
		streams[i] = &AVStream{stream}
	}
	return streams
}

func (c *DeviceContext) Run() error {
	streams := c.Streams()
	if len(c.Sinks) != len(streams) {
		return errors.New("number of streams does not match number of sinks")
	}
	for {
		if averr := C.av_read_frame(c.avformatctx, c.packet.packet); averr < 0 {
			return av_err("av_read_frame", averr)
		}
		if sink := c.Sinks[c.packet.packet.stream_index]; sink != nil {
			c.packet.packet.stream_index = C.int(sink.Index)
			c.packet.timebase = streams[sink.Index].stream.time_base
			if err := sink.WriteAVPacket(c.packet); err != nil {
				return err
			}
		}
		C.av_packet_unref(c.packet.packet)
	}
}

func (c *DeviceContext) Close() error {
	// close all the sinks
	for _, sink := range c.Sinks {
		if err := sink.Close(); err != nil {
			return err
		}
	}

	// close the packet
	if err := c.packet.Close(); err != nil {
		return err
	}

	// free the context
	C.avformat_free_context(c.avformatctx)

	return nil
}

var _ AVFormatContext = (*DeviceContext)(nil)