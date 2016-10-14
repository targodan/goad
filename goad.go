package goad

import "gopkg.in/targodan/ffgopeg.v1/avformat"

func init() {
	avformat.RegisterAll()
}
