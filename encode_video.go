/*
 * Ported from C version of
 * https://github.com/FFmpeg/FFmpeg/blob/master/doc/examples/encode_video.c
 */
package main

import "C"

import (
	"fmt"
	"github.com/koropets/goav/avcodec"
	"github.com/koropets/goav/avutil"
	"io"
	"os"
	"unsafe"
)

var endcode = []byte{0, 0, 1, 0xb7}

func encode(encCtx *avcodec.Context, frame *avutil.Frame, pkt *avcodec.Packet, output io.Writer) {
	var ret int

	/* Send the frame to the encoder */
	if frame != nil {
		fmt.Printf("Send frame %d\n", frame.Pts)
	}
	ret = encCtx.SendFrame(frame)
	if ret < 0 {
		fmt.Fprintf(os.Stderr, "Error sending a frame for decoding\n")
		os.Exit(1)
	}
	for ret >= 0 {
		ret = encCtx.ReceivePacket(pkt)
		if ret == avutil.AVERROR_EAGAIN || ret == avutil.AVERROR_EOF {
			return
		} else if ret < 0 {
			fmt.Fprintf(os.Stderr, "Error during encoding\n")
			os.Exit(1)
		}
		data := C.GoBytes(unsafe.Pointer(pkt.Data()), C.int(pkt.Size()))
		_, err := output.Write(data)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Could not write to file: %s\n", err)
			os.Exit(1)
		}
		pkt.AvPacketUnref()
	}
}

func main() {
	var ret int
	var file io.WriteCloser
	var err error

	if len(os.Args) <= 2 {
		fmt.Printf("Usage: %s <output file> <codec name>\n", os.Args[0])
		os.Exit(1)
	}
	filename := os.Args[1]
	codecName := os.Args[2]

	avcodec.AvcodecRegisterAll()

	file, err = os.OpenFile(filename, os.O_WRONLY|os.O_CREATE, 0755)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Could not open file: %s\n", err)
		os.Exit(1)
	}
	defer file.Close()

	codec := avcodec.AvcodecFindEncoderByName(codecName)
	if codec == nil {
		fmt.Fprintf(os.Stderr, "Codec '%s' not found\n", codecName)
		os.Exit(1)
	}

	c := codec.AvcodecAllocContext3()
	if c == nil {
		fmt.Fprintf(os.Stderr, "Could not allocate video codec context\n")
		os.Exit(1)
	}

	pkt := avcodec.AvPacketAlloc()
	if pkt == nil {
		fmt.Fprintf(os.Stderr, "Could not allocate packet\n")
		os.Exit(1)
	}

	c.SetBitRate(400000)
	/* Resolution must be a multiple of two */
	c.SetWidth(325)
	c.SetHeight(288)
	c.SetTimeBase(avutil.NewRational(1, 25))
	c.SetFramerate(avutil.NewRational(25, 1))

	/* Emit one intra frame every ten frames
	 * check frame pict_type before passing frame
	 * to encoder, if frame.pict_type is AV_PICTURE_TYPE_I
	 * then gop_size is ignored and the output of encoder
	 * will always be I frame irrespective to gop_size
	 */
	c.SetGopSize(10)

	c.SetMaxBFrames(1)
	c.SetPixFmt(avcodec.AV_PIX_FMT_YUV420P)

	ret = c.AvcodecOpen2(codec, nil)
	if ret < 0 {
		fmt.Fprintf(os.Stderr, "Could not open codec\n")
		os.Exit(1)
	}

	frame := avutil.AvFrameAlloc()
	if frame == nil {
		fmt.Fprintf(os.Stderr, "Could not allocate video frame\n")
		os.Exit(1)
	}
	frame.SetFormat(avutil.PixelFormat(c.PixFmt()))
	frame.SetWidth(c.Width())
	frame.SetHeight(c.Height())

	ret = avutil.AvFrameGetBuffer(frame, 32)
	if ret < 0 {
		fmt.Fprintf(os.Stderr, "Could not allocate the video frame data\n")
		os.Exit(1)
	}

	dataItemSize := unsafe.Sizeof(*frame.Data[0])

	var y, x, i, v uint8

	/* Encode 1 second of video */
	for i = 0; i < 25; i++ {
		ret = avutil.AvFrameMakeWritable(frame)
		if ret < 0 {
			fmt.Fprintf(os.Stderr, "Cant make frame writable\n")
			os.Exit(1)
		}

		/* Prepare a dummy image */
		/* Y */
		for y = 0; y < uint8(c.Height()); y++ {
			for x = 0; x < uint8(c.Width()); x++ {
				v = x + y + i*3
				*(*uint8)(unsafe.Pointer(uintptr(unsafe.Pointer(frame.Data[0])) + uintptr(uintptr(y)*uintptr(frame.Linesize[0])+uintptr(x)*dataItemSize))) = v
			}
		}

		for y = 0; y < uint8(c.Height())/2; y++ {
			for x = 0; x < uint8(c.Width())/2; x++ {
				v = 128 + y + i*2
				*(*uint8)(unsafe.Pointer(uintptr(unsafe.Pointer(frame.Data[1])) + uintptr(uintptr(y)*uintptr(frame.Linesize[1])+uintptr(x)*dataItemSize))) = v
				v = 64 + x + i*5
				*(*uint8)(unsafe.Pointer(uintptr(unsafe.Pointer(frame.Data[2])) + uintptr(uintptr(y)*uintptr(frame.Linesize[2])+uintptr(x)*dataItemSize))) = v
			}
		}
		frame.Pts = int64(i)

		/* Cb and Cr */
		encode(c, frame, pkt, file)
	}

	encode(c, nil, pkt, file)

	/* Add sequence end code to have a real MPEG file */
	_, err = file.Write(endcode)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Could not write to file: %s\n", err)
		os.Exit(1)
	}

	avcodec.AvcodecFreeContext(c)
	avutil.AvFrameFree(frame)
	avcodec.AvPacketFree(pkt)
}
