package main

import (
	"C"
	"fmt"
	"io"
	"math"
	"os"
	"unsafe"

	"github.com/koropets/goav/avcodec"
	"github.com/koropets/goav/avutil"
)

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

func checkSampleFmt(codec *avcodec.Codec, sampleFmt avcodec.AvSampleFormat) bool {
	for _, v := range codec.SampleFmts() {
		if v == sampleFmt {
			return true
		}
	}
	return false
}

func selectSampleRate(codec *avcodec.Codec) int {
	var bestSamplerate int
	supportedSemplerates := codec.SupportedSamplerates()
	if len(supportedSemplerates) == 0 {
		return 44100
	}
	for _, v := range supportedSemplerates {
		if bestSamplerate == 0 || abs(44100-v) < abs(44100-bestSamplerate) {
			bestSamplerate = v
		}
	}
	return bestSamplerate
}

func selectChanelLayout(codec *avcodec.Codec) uint64 {
	var bestChLayout uint64
	var bestNbChannels int
	chanelLayouts := codec.ChannelLayouts()
	if len(chanelLayouts) == 0 {
		return uint64(avutil.AV_CH_LAYOUT_STEREO)
	}
	for _, v := range chanelLayouts {
		nbChannels := avutil.AvGetChannelLayoutNbChannels(v)

		if nbChannels > bestNbChannels {
			bestChLayout = v
			bestNbChannels = nbChannels
		}
	}
	return bestChLayout
}

func encode(encCtx *avcodec.Context, frame *avutil.Frame, pkt *avcodec.Packet, output io.Writer) {
	var ret int

	ret = encCtx.SendFrame(frame)
	if ret < 0 {
		fmt.Fprintf(os.Stderr, "Error sending the frame to the encoder\n")
		os.Exit(1)
	}
	for ret >= 0 {
		ret = encCtx.ReceivePacket(pkt)
		if ret == avutil.AVERROR_EAGAIN || ret == avutil.AVERROR_EOF {
			return
		} else if ret < 0 {
			fmt.Fprintf(os.Stderr, "Error encoding audio frame\n")
			os.Exit(1)
		}
		// TODO Write without extra memory allocations
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

	if len(os.Args) <= 1 {
		fmt.Printf("Usage: %s <output file>\n", os.Args[0])
		os.Exit(1)
	}
	filename := os.Args[1]

	avcodec.AvcodecRegisterAll()

	codec := avcodec.AvcodecFindEncoder(avcodec.CodecId(avcodec.AV_CODEC_ID_MP2))
	if codec == nil {
		fmt.Fprintf(os.Stderr, "Codec not found\n")
		os.Exit(1)
	}

	c := codec.AvcodecAllocContext3()
	if c == nil {
		fmt.Fprintf(os.Stderr, "Could not allocate audio codec context\n")
		os.Exit(1)
	}

	c.SetBitRate(64000)

	sampleFmt := avcodec.AvSampleFormat(avutil.AV_SAMPLE_FMT_S16)
	if !checkSampleFmt(codec, sampleFmt) {
		fmt.Fprintf(os.Stderr, "Encoder does not support sample format %s\n", avutil.AvGetSampleFmtName(int(sampleFmt)))
		os.Exit(1)
	}
	c.SetSampleFmt(sampleFmt)

	c.SetSampleRate(selectSampleRate(codec))
	channelLayout := selectChanelLayout(codec)
	c.SetChannelLayout(channelLayout)
	c.SetChannels(avutil.AvGetChannelLayoutNbChannels(channelLayout))

	ret = c.AvcodecOpen2(codec, nil)
	if ret < 0 {
		fmt.Fprintf(os.Stderr, "Could not open codec\n")
		os.Exit(1)
	}

	file, err = os.OpenFile(filename, os.O_WRONLY|os.O_CREATE, 0755)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Could not open file: %s\n", err)
		os.Exit(1)
	}
	defer file.Close()

	pkt := avcodec.AvPacketAlloc()
	if pkt == nil {
		fmt.Fprintf(os.Stderr, "Could not allocate packet\n")
		os.Exit(1)
	}

	frame := avutil.AvFrameAlloc()
	if frame == nil {
		fmt.Fprintf(os.Stderr, "Could not allocate audio frame\n")
		os.Exit(1)
	}

	frame.Nb_samples = int32(c.FrameSize())
	frame.SetFormat(int(c.SampleFmt()))
	frame.Channel_layout = c.ChannelLayout()

	ret = avutil.AvFrameGetBuffer(frame, 0)
	if ret < 0 {
		fmt.Fprintf(os.Stderr, "Could not allocate audio data buffers\n")
		os.Exit(1)
	}

	var t, tincr float64
	t = 0
	tincr = 2.0 * math.Pi * 440.0 / float64(c.SampleRate())
	for i := 0; i < 200; i++ {
		ret = avutil.AvFrameMakeWritable(frame)
		if ret < 0 {
			fmt.Fprintf(os.Stderr, "Cant make frame writable\n")
			os.Exit(1)
		}
		samples := unsafe.Pointer(frame.Data[0])
		v := uint16(math.Sin(t) * 10000)
		for j := 0; j < c.FrameSize(); j++ {
			*(*uint16)(unsafe.Pointer(uintptr(samples) + uintptr(j*2*2 /* sizeof(uint16) */))) = v
			for k := 1; k < c.Channels(); k++ {
				*(*uint16)(unsafe.Pointer(uintptr(samples) + uintptr((j*2+k)*2 /* sizeof(uint16) */))) = v
			}
			t += tincr
		}

		encode(c, frame, pkt, file)
	}

	encode(c, nil, pkt, file)

	avcodec.AvcodecFreeContext(c)
	avutil.AvFrameFree(frame)
	avcodec.AvPacketFree(pkt)
}
