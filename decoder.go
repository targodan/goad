package goad

import (
	"fmt"
	"math"
	"sync"

	"github.com/targodan/native"

	"gopkg.in/targodan/ffgopeg.v1/avcodec"
	"gopkg.in/targodan/ffgopeg.v1/avformat"
	"gopkg.in/targodan/ffgopeg.v1/avutil"
)

type Decoder struct {
	formatCtxt *avformat.FormatContext
	streams    []*streamInfo
	read       bool
}

type streamInfo struct {
	buffer        chan []float32
	streamIndex   int
	codecCtxt     *avcodec.CodecContext
	sendRecvMutex sync.Mutex
	eagainSend    *eagainSychronizer
	eagainRecv    *eagainSychronizer
}

func NewDecoder(filename string) (*Decoder, error) {
	d := &Decoder{}

	var code avutil.ReturnCode
	d.formatCtxt, code = avformat.OpenInput(filename, nil, nil)

	if !code.Ok() {
		return nil, code
	}

	d.formatCtxt.FindStreamInfo(nil)

	return d, nil
}

func (d *Decoder) EnableFirstAudioStream(bufferSize int, sampleRate int) (<-chan []float32, error, int) {
	for i, s := range d.formatCtxt.Streams() {
		if s.CodecPar().CodecType() == avutil.AVMEDIA_TYPE_AUDIO {
			return d.EnableStream(i, bufferSize, sampleRate)
		}
	}
	return nil, fmt.Errorf("No audio stream found."), 0
}

func (d *Decoder) EnableAllAudioStreams(bufferSize int, sampleRate int) ([]<-chan []float32, []error, []int) {
	var ret []<-chan []float32
	var errors []error
	var sampleRates []int
	for i, s := range d.formatCtxt.Streams() {
		if s.CodecPar().CodecType() == avutil.AVMEDIA_TYPE_AUDIO {
			ch, err, sr := d.EnableStream(i, bufferSize, sampleRate)
			ret = append(ret, ch)
			errors = append(errors, err)
			sampleRates = append(sampleRates, sr)
		}
	}
	return ret, errors, sampleRates
}

func (d *Decoder) EnableStream(streamIndex int, bufferSize int, sampleRate int) (<-chan []float32, error, int) {
	if d.formatCtxt.Streams()[streamIndex].CodecPar().CodecType() != avutil.AVMEDIA_TYPE_AUDIO {
		return nil, fmt.Errorf("Stream %d is not an audio stream!", streamIndex), 0
	}

	codec := avcodec.FindDecoder(d.formatCtxt.Streams()[streamIndex].CodecPar().CodecID())
	if codec == nil {
		return nil, fmt.Errorf("Could not find decoder for stream nr. %d.", streamIndex), 0
	}

	s := &streamInfo{
		buffer:      make(chan []float32, bufferSize),
		streamIndex: streamIndex,
		codecCtxt:   avcodec.NewCodecContext(codec),
		eagainSend:  newEagainSynchronizer(),
		eagainRecv:  newEagainSynchronizer(),
	}
	if s.codecCtxt == nil {
		return nil, fmt.Errorf("Could not create codec context for stream nr. %d.", streamIndex), 0
	}

	code := s.codecCtxt.FromParameters(d.formatCtxt.Streams()[streamIndex].CodecPar())
	if !code.Ok() {
		return nil, code, 0
	}

	d.streams = append(d.streams, s)

	s.codecCtxt.SetSampleRate(sampleRate)

	code = s.codecCtxt.Open(codec, nil)
	if !code.Ok() {
		return nil, code, 0
	}
	return s.buffer, nil, s.codecCtxt.SampleRate()
}

func (d *Decoder) Close() {
	for _, s := range d.streams {
		s.codecCtxt.Close()
		s.codecCtxt.Free()
	}
	d.formatCtxt.Close()
}

func (d *Decoder) Streams() []*avformat.Stream {
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

func (d *Decoder) findStream(i int) *streamInfo {
	for _, s := range d.streams {
		if s.streamIndex == i {
			return s
		}
	}
	panic("Stream does not exist.")
}

func getSample(sampleFmt avutil.SampleFormat, sampleSize int, buffer []byte, sampleIndex int) float32 {
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
		panic(fmt.Sprintf("Invalid sample format %s.", sampleFmt.Name()))
	}

	return ret
}

func (d *Decoder) Start() <-chan error {
	readErrors := make(chan error)
	d.read = true
	// start the reader
	go func(d *Decoder) {
		frame := avutil.NewFrame()
		if frame == nil {
			readErrors <- fmt.Errorf("Could not allocate frame.")
			d.read = false
			return
		}
		defer frame.Free()

		var packet avcodec.Packet
		packet.Init()

		for d.read {
			// Read next frame
			code := d.formatCtxt.ReadFrame(&packet)
			if code.IsOneOf(avutil.AVERROR_EOF()) {
				break
			} else if !code.Ok() {
				readErrors <- code
				d.read = false
				return
			}

			if !d.isStreamEnabled(packet.StreamIndex()) {
				packet.Unref()
				continue
			}

			stream := d.findStream(packet.StreamIndex())

			for {
				stream.sendRecvMutex.Lock()
				code = stream.codecCtxt.SendPacket(&packet)
				stream.sendRecvMutex.Unlock()
				if code.Ok() {
					packet.Unref()
					stream.eagainRecv.Signal()
					break
				} else if code.IsOneOf(avutil.AVERROR_EAGAIN()) {
					stream.eagainRecv.Signal()
					stream.eagainSend.Wait()
				} else {
					// Something went wrong.
					readErrors <- code
					d.read = false
					return
				}
			}
		}
		for _, stream := range d.streams {
			code := stream.codecCtxt.SendPacket(nil)
			if !code.Ok() {
				readErrors <- code
				d.read = false
			}
		}
	}(d)

	var wg sync.WaitGroup

	// start consumer
	for _, stream := range d.streams {
		wg.Add(1)
		go func(stream *streamInfo) {
			frame := avutil.NewFrame()
			if frame == nil {
				readErrors <- fmt.Errorf("Could not allocate frame.")
				d.read = false
				return
			}
			defer frame.Free()

			// Apparently retreiving the sample size and isPlanar takes rather long => do it just once.
			sampleSize := stream.codecCtxt.SampleFmt().BytesPerSample()
			isPlanar := stream.codecCtxt.SampleFmt().IsPlanar()

			for d.read {
				stream.sendRecvMutex.Lock()
				code := stream.codecCtxt.ReceiveFrame(frame)
				stream.sendRecvMutex.Unlock()
				if code.IsOneOf(avutil.AVERROR_EAGAIN()) {
					stream.eagainSend.Signal()
					stream.eagainRecv.Wait()
					continue
				} else if code.IsOneOf(avutil.AVERROR_EOF()) {
					break
				} else if !code.Ok() {
					readErrors <- code
					break
				}
				stream.eagainSend.Signal()

				for s := 0; s < frame.NbSamples(); s++ {
					sample := make([]float32, stream.codecCtxt.Channels())
					for c := 0; c < stream.codecCtxt.Channels(); c++ {
						if isPlanar {
							sample[c] = getSample(stream.codecCtxt.SampleFmt(), sampleSize, frame.ExtendedData(c, frame.Linesize(0)), s)
						} else {
							sample[c] = getSample(stream.codecCtxt.SampleFmt(), sampleSize, frame.ExtendedData(0, frame.Linesize(0)), s*stream.codecCtxt.Channels()+c)
						}
					}
					stream.buffer <- sample
				}

				frame.Unref()
			}
			close(stream.buffer)
			wg.Done()
		}(stream)
	}

	go func() {
		wg.Wait()
		close(readErrors)
	}()

	return readErrors
}
