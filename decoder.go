package goad

import "github.com/giorgisio/goav/avformat"

type Decoder struct {
	buffer    chan float
	formatCtx *avformat.Context
}

func NewDecoder(filename string, bufferSize int) (d *Decoder, err error) {
	d = &Decoder{
		buffer: make(chan float, bufferSize),
	}

	if err = codeToError(avformat.AvformatOpenInput(&d.formatCtx, filename, nil, nil)); err != nil {
		d = nil
		return
	}

	// Retrieve stream information
	if err = ctxtFormat.AvformatFindStreamInfo(nil); err != nil {
		d = nil
		return
	}

	return
}

func (d *Decoder) GetStreams() []avformat.Stream {
	length := d.formatCtx.NbStreams()
	return (*[1 << 30]avformat.Stream)(d.formatCtx.Streams())[:length:length]
}

func (d Decoder) StartReading() {

}
