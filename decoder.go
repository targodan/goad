package goad

import (
	"fmt"
	"math"

	"gopkg.in/targodan/ffgopeg.v1/avcodec"
	"gopkg.in/targodan/ffgopeg.v1/avformat"
	"gopkg.in/targodan/ffgopeg.v1/avutil"
)

type Decoder struct {
	formatCtxt *avformat.Context
	streams    []streamInfo
	read       bool
	readErrors chan error
}

type streamInfo struct {
	buffer      chan []float32
	streamIndex int
	codecCtxt   *avcodec.CodecContext
}

func NewDecoder(filename string) (*Decoder, error) {
	d = &Decoder{
		readErrors: make(chan error, 1),
	}

	d.formatCtxt, code = avformat.OpenInput(filename, nil, nil)

	if !code.Ok() {
		return nil, code
	}

	d.formatCtxt.FindStreamInfo(nil)

	return d, nil
}

func (d *Decoder) EnableFirstAudioStream(bufferSize int) (<-chan []float32, error) {
	for i, s := range d.formatCtxt.Streams() {
		if s.CodecPar().CodecType() == avutil.AVMEDIA_TYPE_AUDIO {
			return d.EnableStream(i, bufferSize)
		}
	}
	return nil, fmt.ErErrorf("No audio stream found.")
}

func (d *Decoder) EnableStream(streamIndex int, bufferSize int) (<-chan []float32, error) {
	codec := avcodec.FindDecoder(d.formatCtxt.Streams()[streamIndex].CodecPar().CodecID())
	if codec == nil {
		return nil, fmt.Errorf("Could not find decoder for stream nr. %d.", streamIndex)
	}

	s := streamInfo{
		buffer:      make(chan []float32, bufferSize),
		streamIndex: streamIndex,
		codecCtxt:   avcodec.NewCodecContext(codec),
	}
	if s.codecCtxt == nil {
		return nil, fmt.Errorf("Could not create codec context for stream nr. %d.", streamIndex)
	}
	code := s.codecCtxt.FromParameters(f.formatCtx.Streams()[streamIndex].CodecPar())
	if !code.Ok() {
		return nil, code
	}
	d.streams = append(d.streams, s)
	// s.codecCtxt.SetRequestSampleFmt(s.codecCtxt.SampleFmt().Packed())
	code = codecCtxt.Open(codec, nil)
	if !code.Ok() {
		return nil, code
	}
	return s.buffer, nil
}

func (d *Decoder) Close() {
	for _, s := range d.streams {
		s.codecCtxt.Close()
		s.codecCtxt.Free()
		close(s.buffer)
	}
	d.formatCtxt.Close()
}

func (d *Decoder) Streams() []avformat.Stream {
	return d.formatCtxt.Streams()
}

func (d *Decoder) isStreamEnabled(i int) bool {
	for _, s := range d.streams {
		if s.streamIndex == i {
			return true
		}
	}
	return false
}

func (d *Decoder) findStream(i int) streamInfo {
	for _, s := range d.streams {
		if s.streamIndex == i {
			return s
		}
	}
	return nil
}

func getSample(sampleFmt avcodec.SampleFmt, buffer []byte, sampleIndex int) (float32, error) {
	sampleSize := sampleFmt.BytesPerSample()
	byteIndex := sampleSize * sampleIndex
	var val int64
	switch sampleSize {
	case 1:
		// 8bit samples are always unsigned
		val = int64(buffer[byteIndex]) - 127

	case 2:
		val = int64(int16(native.ByteOrder.Uint16(buffer[byteIndex : byteIndex+sampleSize])))

	case 4:
		val = int64(int32(native.ByteOrder.Uint32(buffer[byteIndex : byteIndex+sampleSize])))

	case 8:
		val = int64(native.ByteOrder.Uint64(buffer[byteIndex : byteIndex+sampleSize]))

	default:
		panic(fmt.Sprintf("Invalid sample size %d.", sampleSize))
	}

	var ret float32
	switch sampleFmt {
	case avutil.AV_SAMPLE_FMT_U8,
		avutil.AV_SAMPLE_FMT_S16,
		avutil.AV_SAMPLE_FMT_S32,
		avutil.AV_SAMPLE_FMT_U8P,
		avutil.AV_SAMPLE_FMT_S16P,
		avutil.AV_SAMPLE_FMT_S32P:
		// integer => Scale to [-1, 1] and convert to float.
		div := ((1 << (uint(sampleSize)*8 - 1)) - 1)
		ret = float32(val) / float32(div)
		break

	case avutil.AV_SAMPLE_FMT_FLT,
		avutil.AV_SAMPLE_FMT_FLTP:
		// float => reinterpret
		ret = math.Float32frombits(uint32(val))
		break

	case avutil.AV_SAMPLE_FMT_DBL,
		avutil.AV_SAMPLE_FMT_DBLP:
		// double => reinterpret and then static cast down
		ret = float32(math.Float64frombits(uint64(val)))
		break

	default:
		return 0, fmt.Errorf("Invalid sample format %s.", sampleFmt.Name())
	}

	return ret, nil
}

func (d *Decoder) Start() {
	d.read = true
	// start the reader
	go func(d *Decoder) {
		frame := avutil.NewFrame()
		if frame == nil {
			d.readErrors <- fmt.Errorf("Could not allocate frame.")
			d.read = false
			return
		}
		defer frame.Free()

		var packet avcodec.Packet
		packet.Init()

		for d.read {
			// Read next frame
			code := formatCtx.ReadFrame(&packet)
			if code.IsOneOf(avutil.AVERROR_EOF()) {
				break
			} else if !code.Ok() {
				d.readErrors <- code
				d.read = false
				return
			}

			if !d.isStreamEnabled(packet.StreamIndex()) {
				packet.Unref()
				continue
			}

			for {
				code = codecCtxt.SendPacket(&packet)
				if code.Ok() {
					packet.Unref()
					break
				} else if code.IsOneOf(avutil.AVERROR_EAGAIN()) {
					// wait and try again
					// TODO: synchronise with consumer
				} else {
					// Something went wrong.
					d.readErrors <- code
					d.read = false
					return
				}
			}
		}
		code := codecCtxt.SendPacket(nil)
		if !code.Ok() {
			d.readErrors <- code
			d.read = false
		}
	}(d)

	// start consumer
	go func(d *Decoder) {
		d.read = false
		frame := avutil.NewFrame()
		if frame == nil {
			d.readErrors <- fmt.Errorf("Could not allocate frame.")
			d.read = false
			return
		}
		defer frame.Free()

		for d.read {
			for _, stream := range d.streams {
				code := stream.codecCtxt.ReceiveFrame(frame)
				if code.IsOneOf(avutil.AVERROR_EAGAIN()) {
					// wait and try again
					// TODO: synchronise
				} else if !code.Ok() {
					d.readErrors <- code
					return
				}

				for s := 0; s < frame.NbSamples(); s++ {
					sample := make([]float32, stream.codecCtxt.Channels())
					for c := 0; c < stream.codecCtxt.Channels(); c++ {
						if stream.codecCtxt.SampleFmt().IsPlanar() {
							sample[c] = getSample(stream.codecCtxt, frame.ExtendedData(c, frame.Linesize(0)), s)
						} else {
							sample[c] = getSample(stream.codecCtxt, frame.ExtendedData(0, frame.Linesize(0)), s*stream.codecCtxt.Channels()+c)
						}
					}
					stream.buffer <- sample
				}

				frame.Unref()
			}
		}
	}(d)
}
